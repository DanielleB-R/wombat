package decode

import (
	"errors"
	"testing"
)

// This is the only test file that names the implementation. The other tests
// in this package are written against the WebAssembly 1.0 binary format
// (https://www.w3.org/TR/wasm-core-1/#binary-format%E2%91%A0) and talk to your
// code exclusively through the four adapters below.
//
// Wire them up as you implement each piece; until then they return
// errNotWired and the tests that need them skip instead of failing.

var errNotWired = errors.New("decode tests are not wired to the implementation yet")

// testReadU32 decodes one LEB128 u32 from the front of b (spec 5.2.2) and
// reports how many bytes it consumed.
func testReadU32(b []byte) (v uint32, n int, err error) { return ReadLEB128U32(b) }

// testReadS32 decodes one LEB128 s32 (the immediate of i32.const).
func testReadS32(b []byte) (v int32, n int, err error) { return ReadLEB128I32(b) }

// testReadS64 decodes one LEB128 s64 (the immediate of i64.const).
func testReadS64(b []byte) (v int64, n int, err error) { return ReadLEB128I64(b) }

// testDecode decodes a complete module. It must return nil exactly when the
// bytes are a well-formed WebAssembly 1.0 binary, and an error otherwise. It
// must never panic. Validation errors (type errors, bad indices, limits where
// min > max, ...) are NOT decode errors; see the validCases in decode_test.go.
func testDecode(_ []byte) error { return errNotWired }

// skipIfNotWired skips t when err says the adapter is a stub.
func skipIfNotWired(t testing.TB, err error) {
	t.Helper()
	if errors.Is(err, errNotWired) {
		t.Skip(err)
	}
}
