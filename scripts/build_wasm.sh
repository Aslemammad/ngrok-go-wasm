#!/bin/bash

# Create static directory
mkdir -p examples/wasm/static

# Build wasm
GOOS=js GOARCH=wasm go build -o examples/wasm/static/ngrok.wasm wasm/main.go

# Copy wasm exec
cp "$(go env GOROOT)/misc/wasm/wasm_exec.js" examples/wasm/static/ 