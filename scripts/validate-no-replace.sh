#!/usr/bin/env bash
# validate-no-replace.sh — prove a module resolves dependencies from
# published versions, not local replace directives.
#
# Copies the module into a temporary directory outside the repository
# workspace, strips every local "replace" directive, and runs
# download/tidy/vet/test/build with GOWORK=off. The temporary copy is
# removed on exit.
#
# Usage: validate-no-replace.sh <module-dir>
#   module-dir is relative to the repository root (e.g. ".",
#   "instrumentation/adk").
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MODULE_DIR="${1:?usage: validate-no-replace.sh <module-dir>}"
SRC_DIR="${REPO_ROOT}/${MODULE_DIR}"

if [ ! -f "${SRC_DIR}/go.mod" ]; then
    echo "ERROR: no go.mod in ${SRC_DIR}" >&2
    exit 1
fi

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

# Copy the module (exclude .git, vendored caches, build artifacts).
rsync -a --exclude='.git' --exclude='vendor' "${SRC_DIR}/" "${TMP_DIR}/" 2>/dev/null \
    || { cp -r "${SRC_DIR}/." "${TMP_DIR}/"; rm -rf "${TMP_DIR}/vendor"; }

# Strip all replace directives from the copied go.mod.
# Go mod edit cannot remove multi-line replace blocks reliably in a
# single pass, so we rewrite the file with an awk pass that drops
# "replace" lines and skips the contents of replace blocks.
cd "${TMP_DIR}"
awk '
    /^replace[ \t]*\(/ { inblock=1; next }
    inblock && /^\)/    { inblock=0; next }
    inblock             { next }
    /^replace[ \t]/     { next }
    { print }
' go.mod > go.mod.sanitized && mv go.mod.sanitized go.mod

# Fail loudly if any replace survived sanitization.
if grep -n '^replace' go.mod; then
    echo "ERROR: sanitized go.mod still contains replace directives" >&2
    exit 1
fi

# Resolve from declared versions only — no workspace, no replaces.
export GOWORK=off
go mod tidy -diff
go mod download
go vet ./...
go test ./...
go build ./...

echo "validate-no-replace: ${MODULE_DIR} OK"
