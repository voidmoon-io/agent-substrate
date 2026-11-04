// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package steps

import (
	"go.ate.dev/substrate/cmd/ate-setup/internal/config"
	"go.ate.dev/substrate/cmd/ate-setup/internal/kube"
	"testing"
)

func TestRenderGitOpsSnapshotConfig(t *testing.T) {
	cfg := &config.Config{Kind: true, SnapshotLocation: "gs://ate-snapshots/", PostgresReadWriteRole: config.DefaultPostgresReadWriteRole, PostgresOwnerRole: config.DefaultPostgresOwnerRole}
	e := &Env{Cfg: cfg}
	raw, err := e.renderAPIServerEnvVars()
	if err != nil {
		t.Fatal(err)
	}
	objs, err := kube.DecodeManifestBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, obj := range objs {
		if obj.GetKind() == "ConfigMap" {
			found = true
			data := obj.Object["data"].(map[string]any)
			if data["ATE_API_SNAPSHOT_LOCATION"] != "gs://ate-snapshots/" {
				t.Fatalf("missing snapshot location: %v", data)
			}
			if value, ok := data["ATE_API_SNAPSHOT_TRANSFER_SERVICE_ACCOUNT"]; !ok || value != "" {
				t.Fatalf("kind transfer account: %v", data)
			}
		}
	}
	if !found {
		t.Fatal("missing API ConfigMap")
	}
	cfg.SnapshotLocation = ""
	if _, err := e.renderAPIServerEnvVars(); err == nil {
		t.Fatal("accepted missing snapshot location")
	}
	cfg.SnapshotLocation = "gs://ate-snapshots/"
	cfg.Kind = false
	if _, err := e.renderAPIServerEnvVars(); err == nil {
		t.Fatal("accepted missing transfer identity")
	}
}
