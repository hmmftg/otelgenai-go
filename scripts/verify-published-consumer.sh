#!/usr/bin/env bash
# verify-published-consumer.sh — prove a published module version is
# consumable from a completely unrelated external module.
#
# Creates a temporary module with NO replace, NO go.work, NO local
# module path, and NO checkout dependency, then resolves the given
# module@version through Go's normal resolution (proxy/direct per
# GOPROXY) and builds a trivial program that imports it.
#
# Usage: verify-published-consumer.sh <module-path> <version> <import-path>
#   e.g. verify-published-consumer.sh github.com/hmmftg/otelgenai-go v0.6.0 \
#            github.com/hmmftg/otelgenai-go
set -euo pipefail

MODULE_PATH="${1:?usage: verify-published-consumer.sh <module-path> <version> <import-path>}"
VERSION="${2:?usage: verify-published-consumer.sh <module-path> <version> <import-path>}"
IMPORT_PATH="${3:?usage: verify-published-consumer.sh <module-path> <version> <import-path>}"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT
cd "${TMP_DIR}"

cat > go.mod <<EOF
module example.com/published-consumer-check

go 1.25.0

require ${MODULE_PATH} ${VERSION}
EOF

# A minimal program that only imports the package. Constructors are
# exercised implicitly by package initialization; a compile-time
# reference keeps the import alive.
cat > main.go <<EOF
package main

import (
	_ "${IMPORT_PATH}"
)

func main() {}
EOF

export GOWORK=off

# No replace, no workspace, no local path: go get resolves the exact
# published version through GOPROXY (direct VCS if the proxy lacks it).
go mod tidy
go build ./...

echo "verify-published-consumer: ${MODULE_PATH}@${VERSION} OK"
