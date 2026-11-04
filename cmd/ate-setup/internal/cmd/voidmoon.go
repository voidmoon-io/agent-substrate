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
		Use:         "render",
		Annotations: map[string]string{clusterAnnotation: "no"},
		Short:       "Write the manifests deploy ate-system applies to --out instead of applying them",
		Long: `Write the manifests deploy ate-system applies to --out, split into crds/
and system/. Needs --image-repo and --image-tag so every image is pinned to a
published digest. It does not read kubeconfig or contact the cluster.`,
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
