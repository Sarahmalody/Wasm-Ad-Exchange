#!/usr/bin/env bash
# Compiles auction.go to a Wasm binary and copies the matching JS glue
# code (wasm_exec.js) that the Go runtime needs to bootstrap in-browser.
#
# Usage:
#   cd engine/
#   ./build.sh
#
# Requires Go 1.21+ on your PATH.

set -euo pipefail

echo "==> Compiling auction.go to WebAssembly..."
GOOS=js GOARCH=wasm go build -o auction.wasm auction.go

echo "==> Locating wasm_exec.js from the Go installation..."
# The location of wasm_exec.js moved in Go 1.24 (misc/wasm -> lib/wasm).
# We check both paths so this script works across Go versions instead of
# hardcoding one and silently failing on the other.
GOROOT_PATH="$(go env GOROOT)"
if [ -f "${GOROOT_PATH}/lib/wasm/wasm_exec.js" ]; then
    cp "${GOROOT_PATH}/lib/wasm/wasm_exec.js" .
elif [ -f "${GOROOT_PATH}/misc/wasm/wasm_exec.js" ]; then
    cp "${GOROOT_PATH}/misc/wasm/wasm_exec.js" .
else
    echo "ERROR: could not locate wasm_exec.js under ${GOROOT_PATH}"
    echo "       Check your Go installation, or copy it manually."
    exit 1
fi

echo "==> Done. Output files:"
ls -lh auction.wasm wasm_exec.js

echo ""
echo "Copy both auction.wasm and wasm_exec.js into the frontend/ directory"
echo "(or update the fetch paths in index.html) before opening the page."
