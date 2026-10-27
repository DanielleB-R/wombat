package decode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// localGroup encodes one locals entry: a count and a value type (spec 5.5.13).
func localGroup(count uint32) []byte { return bs(uleb(count), []byte{typeI32}) }

// funcBody encodes one code entry (size ++ locals vector ++ end) whose
// function declares one locals group per count.
func funcBody(counts ...uint32) []byte {
	locals := uleb(uint32(len(counts)))
	for _, c := range counts {
		locals = append(locals, localGroup(c)...)
	}
	entry := bs(locals, []byte{opEnd})
	return bs(uleb(uint32(len(entry))), entry)
}

// localsMod is a module with one () -> () function per entry in funcs, each
// declaring the given locals group counts.
func localsMod(funcs ...[]uint32) []byte {
	funcBodies := uleb(uint32(len(funcs)))
	typeIdxs := uleb(uint32(len(funcs)))
	for _, counts := range funcs {
		typeIdxs = append(typeIdxs, 0x00)
		funcBodies = append(funcBodies, funcBody(counts...)...)
	}
	return mod(typeSec(), sec(secFunc, typeIdxs...), sec(secCode, funcBodies...))
}

// Spec 5.5.13 / the reference interpreter: the locals declared by a single
// function may total at most 2^32-1. The limit is per function, not per
// module, and is not shared between functions.
func TestDecodeLocalsLimitIsPerFunction(t *testing.T) {
	const maxLocals = 0xFFFFFFFF
	tests := []struct {
		name  string
		funcs [][]uint32
		ok    bool
	}{
		{"one function at the limit", [][]uint32{{maxLocals}}, true},
		{"one function at the limit across groups", [][]uint32{{0x80000000, 0x7FFFFFFF}}, true},
		{"one function one past the limit", [][]uint32{{maxLocals, 1}}, false},
		{"one function at 2^32 across groups", [][]uint32{{0x80000000, 0x80000000}}, false},
		{"one function far past the limit", [][]uint32{{maxLocals, maxLocals}}, false},
		{"two functions each at the limit", [][]uint32{{maxLocals}, {maxLocals}}, true},
		{"two functions each 2^31 (module total is 2^32)", [][]uint32{{0x80000000}, {0x80000000}}, true},
		{"three functions each at the limit", [][]uint32{{maxLocals}, {maxLocals}, {maxLocals}}, true},
		{"second function past the limit", [][]uint32{{1}, {maxLocals, 1}}, false},
		{"first function past the limit", [][]uint32{{maxLocals, 1}, {1}}, false},
		{"many small functions", [][]uint32{{1}, {2}, {3}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := testDecode(localsMod(tc.funcs...))
			skipIfNotWired(t, err)
			if tc.ok {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrTooManyLocals)
			}
		})
	}
}
