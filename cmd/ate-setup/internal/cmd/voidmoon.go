// voidmoon-io fork only; see ../steps/voidmoon.go.

package cmd

import (
	"github.com/spf13/cobra"
)

var voidmoonCmd = &cobra.Command{
	Use:   "voidmoon",
	Short: "Render ate-system for a GitOps tree and bootstrap what it cannot hold (voidmoon-io fork)",
	// A render describes no cluster, and a bootstrap only completes one that
	// the GitOps tree installed, so neither replaces the install record.
	Annotations: map[string]string{recordAnnotation: "no"},
}

func init() {
	rootCmd.AddCommand(voidmoonCmd)

	var out string
	render := &cobra.Command{
		Use:   "render",
		Short: "Write the manifests deploy ate-system applies to --out instead of applying them",
		Long: `Write the manifests deploy ate-system applies to --out, split into crds/
and system/. Needs --image-repo and --image-tag so every image is pinned to a
published digest. It does not contact the cluster, but the kubeconfig is still
read, so CI needs a placeholder one.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return env.RenderGitOps(cmd.Context(), out)
		},
	}
	render.Flags().StringVar(&out, "out", "out", "Directory to write the rendered manifests to")
	voidmoonCmd.AddCommand(render)

	var postgres bool
	bootstrap := &cobra.Command{
		Use:   "bootstrap",
		Short: "Create the secrets and config deploy ate-system creates imperatively, leaving existing ones alone",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return env.BootstrapGitOps(cmd.Context(), postgres)
		},
	}
	bootstrap.Flags().BoolVar(&postgres, "postgres", false,
		"Also set up the bundled PostgreSQL identities; needs postgres-0 running")
	voidmoonCmd.AddCommand(bootstrap)
}
