.PHONY: build test check spec-fetch spec-convert

build:
	go build -o bin/wombat ./cmd/wombat

test:
	go test ./...

# What CLAUDE.md requires before anything is "done".
check:
	gofmt -l .
	go vet ./...
	go test -race ./...

# Clone the spec repo (shallow) into testdata/spec/repo.
spec-fetch:
	test -d testdata/spec/repo || git clone --depth 1 https://github.com/WebAssembly/spec testdata/spec/repo

# Convert test/core/*.wast to JSON + .wasm under testdata/spec/json.
spec-convert: spec-fetch
	mkdir -p testdata/spec/json
	for f in testdata/spec/repo/test/core/*.wast; do \
	  n=$$(basename $$f .wast); mkdir -p testdata/spec/json/$$n; \
	  wast2json $$f -o testdata/spec/json/$$n/$$n.json || echo "skip $$n"; \
	done
