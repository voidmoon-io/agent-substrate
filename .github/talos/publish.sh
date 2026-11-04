#!/usr/bin/env bash

# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Runs every root-gated test package under sudo. A package is root-gated when
# its tests import internal/roottest (the tests self-skip unless run as root),
# so adding privileged tests anywhere picks them up here with no CI change.
# Extra arguments are passed through to `go test` (e.g. -v, -run).

set -o errexit -o nounset -o pipefail

set -o errexit -o nounset -o pipefail
V="$(git describe --tags --always --dirty)"
go build -o "${RUNNER_TEMP}/ate-setup" ./cmd/ate-setup

ate() { "${RUNNER_TEMP}/ate-setup" --no-dev-env --no-report-defaults "$@"; }

VERSION="${V}" ate publish release-images --ko-docker-repo "${REPO}" | tee "${RUNNER_TEMP}/published.txt"

# The kind profile: bundled PostgreSQL, in-cluster rustfs for
# snapshots and the in-namespace otel stack. Credential injection is
# off; AX tasks reach models through key-adding proxies instead.
mkdir -p out
ate --kind --credential-provider='{"enabled":false}' \
  --image-repo "${REPO}" --image-tag "${V}" \
  voidmoon render --out out

# This command emits references even when release-images skipped existing tags.
VERSION="${V}" ate publish worker-images --ko-docker-repo "${REPO}" > "${RUNNER_TEMP}/workers.txt"
awk '$1 == "ateworker-gvisor:" || $1 == "ateworker-microvm:" {print $2}' \
  "${RUNNER_TEMP}/workers.txt" > out/worker-images.txt
test "$(wc -l < out/worker-images.txt)" -eq 2

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o out/kubectl-ate ./cmd/kubectl-ate
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o out/ate-setup ./cmd/ate-setup
# bootstrap reads the checked-in
# manifests, so ship the tree the binaries were built from.
tar -czf out/manifests.tar.gz manifests

printf '%s\n' "${V}" > out/SUBSTRATE_VERSION
git rev-parse HEAD > out/SOURCE_COMMIT
cat out/worker-images.txt
