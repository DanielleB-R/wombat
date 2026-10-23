.PHONY: build test check spec-fetch spec-convert testdata-wasm

build:
	go build -o bin/wombat ./cmd/wombat

test:
	go test ./...

# What CLAUDE.md requires before anything is "done".
check:
	gofmt -l .
	go vet ./...
	go test -race ./...

# Clone the spec repo at the 1.0 (MVP) tag, shallow, into testdata/spec/repo.
# main is the 3.0 draft and its tests need post-MVP features.
spec-fetch:
	test -d testdata/spec/repo || git clone --depth 1 --branch wg-1.0 https://github.com/WebAssembly/spec testdata/spec/repo

# Convert test/core/*.wast to JSON + .wasm under testdata/spec/json.
spec-convert: spec-fetch
	mkdir -p testdata/spec/json
	for f in testdata/spec/repo/test/core/*.wast; do \
	  n=$$(basename $$f .wast); mkdir -p testdata/spec/json/$$n; \
	  wast2json $$f -o testdata/spec/json/$$n/$$n.json || echo "skip $$n"; \
	done

# Rebuild the hand-written fixtures in testdata/wasm with every post-1.0
# feature switched off, so they stay valid MVP modules.
WAT_MVP_FLAGS = --disable-mutable-globals --disable-saturating-float-to-int \
	--disable-sign-extension --disable-simd --disable-multi-value \
	--disable-bulk-memory --disable-reference-types --disable-tail-call \
	--disable-memory64 --disable-multi-memory --disable-extended-const \
	--disable-relaxed-simd --disable-exceptions

testdata-wasm:
	for f in testdata/wasm/*.wat; do wat2wasm $(WAT_MVP_FLAGS) $$f -o $${f%.wat}.wasm; done
