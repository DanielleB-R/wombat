# wombat: a WebAssembly interpreter in Go

wombat is a WebAssembly 1.0 (MVP) interpreter written in Go. The goal is to refresh my Go skills, so **I write the implementation myself**.

- Module path: `github.com/DanielleB-R/wombat`
- CLI binary: `wombat` (e.g. `wombat run hello.wasm`)
- Mascot: a wombat, naturally.

## Working agreement

- **I write all implementation code.** Do not write or modify non-test code unless I explicitly ask. If you spot a problem, describe it and point to the file and line; don't fix it.
- **You review.** When I ask for a review, focus on: correctness against the spec, error handling, idiomatic Go, concurrency or aliasing hazards, and API design. Be direct about problems; don't pad with praise.
- **You write tests.** Derive tests from the WebAssembly spec's semantics, not from my implementation, so they don't bless my bugs. Prefer table-driven tests. Test files only (`*_test.go` and testdata/harness code).
- **Explain, don't just answer.** When I ask how something in the spec or in Go works, explain the reasoning, and point me to the relevant spec section.

## Scope

- Target: Wasm 1.0 (MVP) only. No SIMD, threads, multi-value, bulk memory, reference types, or GC until the MVP spec tests pass.
- Stretch goals, in order: WAT text-format parser, performance work (compact internal bytecode or closure-threaded code, profile with `pprof`), differential testing against wazero.
- Target size: roughly 4-5k lines of non-test code.

## Milestones

1. **Binary decoder** (~900 lines): LEB128, all sections (type, import, function, table, memory, global, export, start, element, code, data), typed module struct. Test by decoding real `.wasm` files.
2. **Validator** (~900 lines): the spec appendix's type-checking algorithm with a value-type stack and control-frame stack. Precompute branch targets (block end, else, loop start) during validation so the interpreter never scans.
3. **Interpreter** (~1500 lines): flat `[]uint64` value stack (floats via `math.Float32bits`/`Float64bits`), call-frame stack, opcode switch. Order: numeric ops, memory, control flow, calls. Trap semantics matter (div by zero, integer overflow, out-of-bounds, unreachable, indirect call type mismatch).
4. **Runtime/store** (~600 lines): module instances, linear memory with `memory.grow`, tables and `call_indirect`, globals, imports, host functions as Go closures.
5. **CLI + WASI subset** (~500 lines): `fd_write`, `args_get`, `proc_exit`; enough to run hello-world compiled from C or Rust.

Work milestone by milestone. Don't start the next one until the current one passes its tests and has been reviewed.

## Testing

- Conformance: the official spec tests (`WebAssembly/spec`, `test/core/*.wast`), converted with `wast2json` from WABT into JSON plus `.wasm` files. Write a Go harness that loads the JSON and runs each command (`module`, `assert_return`, `assert_trap`, `assert_invalid`, `assert_malformed`, `assert_exhaustion`, `register`, `invoke`).
- Before starting a milestone, write its tests first where practical. For the validator, build a set of should-pass and should-fail modules.
- Use Go's native fuzzing (`go test -fuzz`) on the decoder: it must never panic on arbitrary input and should return errors instead.
- Run `go vet` and `go test -race ./...` before considering anything done.

## Go conventions

- Dependencies: no libraries for the thing being learned, normal choices for everything around it.
  - **Hard lines:** no Wasm decoding, validation, or execution libraries in non-test code (wazero is allowed as a test-only dependency for differential testing). No dependency for what the stdlib does well (`encoding/binary`, `math`, `math/bits`, etc.). LEB128 is hand-written.
  - **Fine to use:** `testify` (`require`/`assert`) or `go-cmp` for tests; `cobra` or `urfave/cli` for the CLI; `log/slog` for logging; `prometheus/client_golang` and `pprof` for any metrics/profiling stretch work; `golangci-lint` and a GitHub Actions CI workflow.
  - Justify any other dependency before adding it.
- Errors: return errors, don't panic, except for traps inside the interpreter loop, which may use panic/recover internally but must surface as a typed `*Trap` error at the public API.
- Typed errors for decode and validation failures where the spec tests need to distinguish them.
- Keep packages small and acyclic. Suggested layout:
  - `wasm/` module types and constants
  - `decode/` binary decoder
  - `validate/` validator
  - `exec/` interpreter and runtime
  - `wasi/` WASI subset
  - `cmd/wombat/` CLI
  - `internal/spectest/` test harness
- Write doc comments on exported identifiers. Run `gofmt` and `go vet`.

## Reference

- Spec: https://webassembly.github.io/spec/core/ (binary format ch. 5, validation ch. 3 and appendix, execution ch. 4)
- Tools: WABT (`wat2wasm`, `wast2json`, `wasm-objdump`)
- Comparison implementation: wazero
