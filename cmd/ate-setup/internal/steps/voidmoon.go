// voidmoon-io fork only: lets a GitOps tree (Flux in shart-cloud/talos-gitops)
// own what `deploy ate-system` would apply, by rendering the same manifests to
// files instead of applying them, and splits off the imperative remainder
// (key material, ConfigMaps derived from cluster state, bundled PostgreSQL
// identities) into a bootstrap that runs once per install or upgrade.
//
// Kept in one file so upstream syncs only ever have to carry this file and
// its command in ../cmd/voidmoon.go.

package steps

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/agent-substrate/substrate/cmd/ate-setup/internal/kube"
	"github.com/agent-substrate/substrate/cmd/ate-setup/internal/log"
)

// RenderGitOps writes the manifests DeployAteSystem applies to outDir, split
// the way a GitOps tree has to apply them: crds/ (CRDs and the controller
// RBAC they ship with) must be established before system/ is applied.
//
// It never talks to the cluster. Images must be pre-built (--image-repo and
// --image-tag) so every reference is pinned to a published digest.
func (e *Env) RenderGitOps(ctx context.Context, outDir string) error {
	if !e.Cfg.Images.IsPrebuilt() {
		return fmt.Errorf("render needs --image-repo and --image-tag: a GitOps tree must reference published images")
	}
	if err := e.RequireCanonicalNamespace("voidmoon render"); err != nil {
		return err
	}
	provider, err := e.Cfg.CredentialProvider()
	if err != nil {
		return err
	}
	if provider.Kubernetes() {
		return fmt.Errorf("render does not support the bundled k8s.io credential provider; pass --credential-provider='{\"enabled\":false}'")
	}

	crds, err := e.ResolveManifest(ctx, e.Cfg.Manifest("generated"))
	if err != nil {
		return err
	}

	podcert, err := e.renderResolve(ctx, e.Cfg.Manifest("pod-certificate-controller.yaml"))
	if err != nil {
		return err
	}
	if e.Cfg.PodcertWorkersPerSigner > 0 {
		objs, err := kube.DecodeManifestBytes(podcert)
		if err != nil {
			return err
		}
		if err := setPodcertWorkersPerSigner(objs, e.Cfg.PodcertWorkersPerSigner); err != nil {
			return err
		}
		if podcert, err = encodeObjects(objs); err != nil {
			return err
		}
	}

	postgresObjs, err := e.postgresObjects()
	if err != nil {
		return err
	}
	postgres, err := encodeObjects(postgresObjs)
	if err != nil {
		return err
	}

	system, err := e.renderSystemManifests(ctx)
	if err != nil {
		return err
	}
	egress, err := e.renderAtenetEgressManifest(ctx, provider)
	if err != nil {
		return err
	}
	apiEnv, err := e.renderAPIServerEnvVars()
	if err != nil {
		return err
	}

	files := []struct {
		path string
		data []byte
	}{
		{path: "crds/generated.yaml", data: crds},
		{path: "system/pod-certificate-controller.yaml", data: podcert},
		{path: "system/sandboxconfig-validation.yaml", data: mustRead(e.Cfg.Manifest("sandboxconfig-validation.yaml"))},
		{path: "system/sandboxconfig-gvisor.yaml", data: mustRead(e.Cfg.Manifest("sandboxconfig-gvisor.yaml"))},
		{path: "system/ate-otel-config.yaml", data: mustRead(e.otelConfigPath())},
		{path: "system/postgres.yaml", data: postgres},
		{path: "system/ate-system.yaml", data: system},
		{path: "system/atenet-egress.yaml", data: egress},
		{path: "system/api-server-envvars.yaml", data: apiEnv},
	}
	for _, f := range files {
		if f.data == nil {
			return fmt.Errorf("render: %s is empty", f.path)
		}
		dst := filepath.Join(outDir, f.path)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dst, f.data, 0o644); err != nil {
			return err
		}
		log.Infof("wrote %s", dst)
	}
	return nil
}

// renderAPIServerEnvVars is what CreateAPIServerEnvVars applies for the
// bundled PostgreSQL, which needs no cluster state to compute. The Secret
// holds connection strings, not credentials: the bundled database
// authenticates each login by pod certificate.
func (e *Env) renderAPIServerEnvVars() ([]byte, error) {
	readWriteDSN, ownerDSN, err := e.postgresReadWriteConnectionStrings()
	if err != nil {
		return nil, err
	}
	configVars := map[string]string{
		"ATE_API_POSTGRES_READ_WRITE_ROLE": e.Cfg.PostgresReadWriteRole,
		"ATE_API_POSTGRES_OWNER_ROLE":      e.Cfg.PostgresOwnerRole,
		"ATE_API_ACTOR_JWT_ISSUER":         e.Cfg.ActorJWTIssuer,
	}
	if e.Cfg.PostgresPoolMaxConns != "" {
		configVars["ATE_API_POSTGRES_POOL_MAX_CONNS"] = e.Cfg.PostgresPoolMaxConns
	}
	objs := []*unstructured.Unstructured{
		{Object: map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]any{"name": ConfigMapAPIEnvVars, "namespace": e.Namespace()},
			"data":     toAny(configVars),
		}},
		{Object: map[string]any{
			"apiVersion": "v1", "kind": "Secret",
			"metadata":   map[string]any{"name": SecretAPIEnvVars, "namespace": e.Namespace()},
			"stringData": toAny(buildAPIServerEnvVars(readWriteDSN, ownerDSN, e.Cfg.PostgresSchemaName())),
		}},
	}
	return encodeObjects(objs)
}

func toAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// BootstrapGitOps creates the key material DeployAteSystem generates and a
// GitOps tree must not hold: the actor-identity JWT and CA pools, the
// podcertificate-controller CAs and the egress MITM CA pool. Existing
// Secrets are left alone, so it is safe to rerun on every upgrade. The
// ate-api-server env and authentication ConfigMaps are not created here;
// the GitOps tree owns them (see renderAPIServerEnvVars).
//
// With postgres set it also runs the bundled PostgreSQL identity setup,
// which needs postgres-0 up, so run it again after the GitOps tree has
// reconciled the StatefulSet.
func (e *Env) BootstrapGitOps(ctx context.Context, postgres bool) error {
	if err := e.EnsureAteSystemNamespace(ctx); err != nil {
		return err
	}
	if err := e.ensureSecret(ctx, e.Namespace(), SecretActorIDJWTPool, e.CreateJWTAuthorityPoolSecret); err != nil {
		return err
	}
	if err := e.ensureSecret(ctx, e.Namespace(), SecretActorIDCAPool, e.CreateActorIDCAPoolSecret); err != nil {
		return err
	}
	if err := e.ensureSecret(ctx, e.Namespace(), SecretActorIDCACerts, e.CreateActorIDCACertsSecret); err != nil {
		return err
	}
	if err := e.EnsurePodCertificateCAs(ctx); err != nil {
		return err
	}
	if err := e.EnsureEgressMITMCAPoolSecret(ctx); err != nil {
		return err
	}
	if !postgres {
		return nil
	}
	return e.waitAndSetupBundledPostgres(ctx)
}

func encodeObjects(objs []*unstructured.Unstructured) ([]byte, error) {
	var buf bytes.Buffer
	for _, obj := range objs {
		out, err := yaml.Marshal(obj.Object)
		if err != nil {
			return nil, err
		}
		buf.WriteString("---\n")
		buf.Write(out)
	}
	return buf.Bytes(), nil
}

func mustRead(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Infof("read %s: %v", path, err)
		return nil
	}
	return data
}
