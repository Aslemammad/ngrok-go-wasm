.PHONY: wasm

# Include other targets...

wasm:
	mkdir -p dist
	./scripts/build_wasm.sh 