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

case "$1" in
build-for-arm64)
  GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build ./...
  ;;
unit-tests)
  go -C tools/junittool run . register "${ARTIFACTS}/unit.xml"
  hack/run-tool.sh gotestsum --junitfile "${ARTIFACTS}/unit.xml" \
    --jsonfile "${ARTIFACTS}/unit.json" \
    --format pkgname-and-test-fails --format-hide-empty-pkg \
    -- -race ./...

  ;;
apitool-tests)
  go -C ../junittool run . register "${ARTIFACTS}/apitool.xml"
  ../../hack/run-tool.sh gotestsum --junitfile "${ARTIFACTS}/apitool.xml" \
    --jsonfile "${ARTIFACTS}/apitool.json" \
    --format pkgname-and-test-fails --format-hide-empty-pkg \
    -- -race ./...

  ;;
junittool-tests)
  go run . register "${ARTIFACTS}/junittool.xml"
  ../../hack/run-tool.sh gotestsum --junitfile "${ARTIFACTS}/junittool.xml" \
    --jsonfile "${ARTIFACTS}/junittool.json" \
    --format pkgname-and-test-fails --format-hide-empty-pkg \
    -- -race ./...

  ;;
dynamic-module-tests)
  hack/test-dynamic-modules.sh
  ;;
root-gated-tests)
  hack/run-root-tests.sh -race
  ;;
require-tests-to-have-run)
  go -C tools/junittool run . verify -manifest "${ARTIFACTS}/expected-junit.txt"
  ;;
verify)
  hack/verify-all.sh
  ;;
verify-immutable-postgresql-migrations)
  hack/verify/postgresql-migrations.sh 17fa8586
  ;;
create-cluster)
  if ! grep -qw nfsd /proc/filesystems; then
    echo "Talos must load the nfsd module before Substrate E2E can run." >&2
    exit 1
  fi
  hack/create-kind-cluster.sh
  ;;
install-agent-substrate--matrixdataplane-)
  go run ./cmd/ate-setup --kind deploy ate-system
  ;;
install-csi-nfs-driver)
  go run ./cmd/ate-setup --kind setup csi nfs
  ;;
deploy-micro-vm-counter-demo)
  go run ./cmd/ate-setup --kind deploy demo counter-microvm
  ;;
deploy-gvisor-counter-demo)
  go run ./cmd/ate-setup --kind deploy demo counter
  ;;
deploy-egress-demos)
  go run ./cmd/ate-setup --kind deploy demo egress
  go run ./cmd/ate-setup --kind deploy demo egress-microvm

  ;;
trust-local-tls-origins-in-egress-gateway)
  hack/setup-e2e-egress-tls-kind.sh
  ;;
focus-the-node-on-gvisor)
  hack/e2e-focus-sandbox-class.sh gvisor
  ;;
run-e2e-tests-gvisor)
  hack/run-e2e-kind.sh -args --no-color --preview="${ATE_PREVIEW}"
  ;;
focus-the-node-on-micro-vm)
  hack/e2e-focus-sandbox-class.sh microvm
  ;;
run-e2e-tests-micro-vm)
  hack/run-e2e-kind.sh -args --no-color --preview="${ATE_PREVIEW}"
  ;;
require-every-lane-to-have-run-tests)
  go -C tools/junittool run . verify -manifest "${ARTIFACTS}/expected-junit.txt"
  ;;
summarize-test-failures)
  go -C tools/junittool run . report "${ARTIFACTS}"
  ;;
dump-diagnostics-on-failure)
  kubectl --context kind-kind get workerpool,pods -A -o wide || true
  dump() {
    echo "=== logs: $1/$2 ==="
    kubectl --context kind-kind logs -n "$1" "$2" --all-containers --tail=300 2>/dev/null || true
  }
  for p in $(kubectl --context kind-kind get pods -n ate-system -o name 2>/dev/null); do
    dump ate-system "$p"
  done
  # Every worker pod in any namespace: the demo pools plus the e2e suites'
  # randomly-named per-test namespaces, which the suites keep on failure.
  # The failing actor runs in one of these, so this is where its worker logs
  # (and, for a micro-VM worker, the guest console tail) live.
  kubectl --context kind-kind get pods -A -l ate.dev/worker-pool \
    -o 'custom-columns=:.metadata.namespace,:.metadata.name' --no-headers 2>/dev/null \
    | while read -r ns name; do dump "$ns" "$name"; done

  ;;
*) echo "Unknown CI step: $1" >&2; exit 2 ;;
esac
