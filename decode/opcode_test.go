package decode

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DanielleB-R/wombat/wasm"
)

// Every possible opcode byte, 0x00-0xFF, gets decoded in a function body with
// a range of immediates. The classification below is transcribed from the 1.0
// instruction encoding table (spec 5.4, "Instructions"), not from the decoder:
//
//	0x00-0x05 control   0x0B end          0x0C-0x11 branch/call
//	0x1A-0x1B parametric 0x20-0x24 variable
//	0x28-0x35 loads     0x36-0x3E stores  0x3F-0x40 memory.size/grow
//	0x41-0x44 constants 0x45-0xBF numeric (no immediates)
//
// and every other byte is reserved (or belongs to a post-1.0 proposal).

type opClass int

const (
	classInvalid opClass = iota
	classNone
	classBlockType // block, loop, if
	classIndex     // one u32: br, br_if, call, local.*, global.*
	classBrTable
	classCallIndirect
	classMemArg
	classMemReserved // memory.size, memory.grow: one 0x00 byte
	classI32Const
	classI64Const
	classF32Const
	classF64Const
	classElse
	classEnd
)

func classOf(op int) opClass {
	switch {
	case op == 0x00, op == 0x01, op == 0x0F, op == 0x1A, op == 0x1B:
		return classNone
	case op >= 0x02 && op <= 0x04:
		return classBlockType
	case op == 0x05:
		return classElse
	case op == 0x0B:
		return classEnd
	case op == 0x0C, op == 0x0D, op == 0x10, op >= 0x20 && op <= 0x24:
		return classIndex
	case op == 0x0E:
		return classBrTable
	case op == 0x11:
		return classCallIndirect
	case op >= 0x28 && op <= 0x3E:
		return classMemArg
	case op == 0x3F, op == 0x40:
		return classMemReserved
	case op == 0x41:
		return classI32Const
	case op == 0x42:
		return classI64Const
	case op == 0x43:
		return classF32Const
	case op == 0x44:
		return classF64Const
	case op >= 0x45 && op <= 0xBF:
		return classNone
	}
	return classInvalid
}

// immSample is one encoding of an opcode's immediates and what it decodes to.
type immSample struct {
	bytes []byte
	want  any
}

func repeat(b byte, n int) []byte { return slices.Repeat([]byte{b}, n) }

func samplesFor(c opClass) []immSample {
	//nolint:exhaustive // only classes that carry immediates have samples
	switch c {
	case classNone:
		return []immSample{{nil, nil}}
	case classIndex:
		return []immSample{
			{[]byte{0x00}, uint32(0)},
			{[]byte{0x05}, uint32(5)},
			{[]byte{0x80, 0x01}, uint32(128)},
			{[]byte{0x85, 0x00}, uint32(5)}, // padded
			{[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, uint32(math.MaxUint32)},
		}
	case classBrTable:
		return []immSample{
			{[]byte{0x00, 0x00}, wasm.BranchTable{Default: 0}},
			{[]byte{0x00, 0x05}, wasm.BranchTable{Default: 5}},
			{[]byte{0x02, 0x00, 0x01, 0x02}, wasm.BranchTable{Indices: []uint32{0, 1}, Default: 2}},
			{[]byte{0x01, 0x80, 0x01, 0x81, 0x00}, wasm.BranchTable{Indices: []uint32{128}, Default: 1}},
		}
	case classCallIndirect:
		return []immSample{
			{[]byte{0x00, 0x00}, uint32(0)},
			{[]byte{0x03, 0x00}, uint32(3)},
			{[]byte{0x80, 0x01, 0x00}, uint32(128)},
		}
	case classMemArg:
		return []immSample{
			{[]byte{0x00, 0x00}, wasm.MemArg{}},
			{[]byte{0x02, 0x08}, wasm.MemArg{Align: 2, Offset: 8}},
			{[]byte{0x80, 0x01, 0x80, 0x80, 0x04}, wasm.MemArg{Align: 128, Offset: 65536}},
			{[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, wasm.MemArg{Align: math.MaxUint32, Offset: math.MaxUint32}},
		}
	case classMemReserved:
		return []immSample{{[]byte{0x00}, nil}}
	case classI32Const:
		return []immSample{
			{[]byte{0x00}, int32(0)},
			{[]byte{0x7F}, int32(-1)},
			{[]byte{0x80, 0x00}, int32(0)}, // padded
			{[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0x07}, int32(math.MaxInt32)},
			{[]byte{0x80, 0x80, 0x80, 0x80, 0x78}, int32(math.MinInt32)},
		}
	case classI64Const:
		return []immSample{
			{[]byte{0x00}, int64(0)},
			{[]byte{0x7F}, int64(-1)},
			{append(repeat(0xFF, 9), 0x00), int64(math.MaxInt64)},
			{append(repeat(0x80, 9), 0x7F), int64(math.MinInt64)},
		}
	case classF32Const:
		return []immSample{
			{[]byte{1, 2, 3, 4}, [4]byte{1, 2, 3, 4}},
			{repeat(0x0B, 4), [4]byte{0x0B, 0x0B, 0x0B, 0x0B}}, // looks like end
			{repeat(0xFF, 4), [4]byte{0xFF, 0xFF, 0xFF, 0xFF}},
		}
	case classF64Const:
		return []immSample{
			{[]byte{1, 2, 3, 4, 5, 6, 7, 8}, [8]byte{1, 2, 3, 4, 5, 6, 7, 8}},
			{repeat(0x0B, 8), [8]byte{0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B}},
			{repeat(0xFF, 8), [8]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}},
		}
	}
	return nil
}

func sameImmediate(t *testing.T, want, got any, msg string) {
	t.Helper()
	if bt, ok := want.(wasm.BranchTable); ok {
		gotBT, isBT := got.(wasm.BranchTable)
		require.True(t, isBT, "%s: immediate is %T", msg, got)
		require.True(t, slices.Equal(bt.Indices, gotBT.Indices), "%s: labels want %v, got %v", msg, bt.Indices, gotBT.Indices)
		require.Equal(t, bt.Default, gotBT.Default, msg)
		return
	}
	require.Equal(t, want, got, msg)
}

func opName(op int) string { return fmt.Sprintf("0x%02X", op) }

// Every opcode that carries immediates (or none) is followed by a nop, so a
// decoder that reads too few or too many immediate bytes shows up as a wrong
// instruction sequence. Padded and maximum-width LEB128 values are included.
func TestOpcodeImmediates(t *testing.T) {
	for op := range 256 {
		c := classOf(op)
		if c == classInvalid || c == classBlockType || c == classElse || c == classEnd {
			continue
		}
		t.Run(opName(op), func(t *testing.T) {
			for i, s := range samplesFor(c) {
				body := bs([]byte{byte(op)}, s.bytes, []byte{0x01, opEnd})
				m := decodeOrSkip(t, bodyMod(body...))
				require.Len(t, m.Code.Codes, 1)
				got := m.Code.Codes[0].Body.Instructions
				require.Len(t, got, 3, "sample %d: opcode, nop, end", i)
				require.Equal(t, wasm.Opcode(op), got[0].Opcode, "sample %d", i)
				sameImmediate(t, s.want, got[0].Immediate, fmt.Sprintf("sample %d (% x)", i, s.bytes))
				require.Equal(t, wasm.Nop, got[1].Opcode, "sample %d: immediates must be consumed exactly", i)
				require.Equal(t, wasm.End, got[2].Opcode, "sample %d", i)
			}
		})
	}
}

// Cutting an immediate one byte short, so that the trailing end byte is
// swallowed as part of it, must leave a body with no end: malformed. The same
// goes for an opcode with its immediate missing entirely.
func TestOpcodeTruncatedImmediates(t *testing.T) {
	for op := range 256 {
		c := classOf(op)
		if c == classInvalid || c == classNone || c == classElse || c == classEnd {
			continue
		}
		t.Run(opName(op)+"/truncated", func(t *testing.T) {
			bodies := [][]byte{{byte(op)}, {byte(op), opEnd}}
			if c != classBlockType {
				for _, s := range samplesFor(c) {
					bodies = append(bodies, bs([]byte{byte(op)}, s.bytes[:len(s.bytes)-1], []byte{opEnd}))
				}
			}
			for _, body := range bodies {
				err := testDecode(bodyMod(body...))
				skipIfNotWired(t, err)
				require.Error(t, err, "body % x", body)
			}
		})
	}
}

// Reserved opcode bytes are rejected, whatever follows them.
func TestOpcodeReserved(t *testing.T) {
	for op := range 256 {
		if classOf(op) != classInvalid {
			continue
		}
		t.Run(opName(op), func(t *testing.T) {
			for _, body := range [][]byte{{byte(op), opEnd}, {byte(op), 0x00, opEnd}, {byte(op)}} {
				err := testDecode(bodyMod(body...))
				skipIfNotWired(t, err)
				require.Error(t, err, "body % x", body)
			}
		})
	}
}

// block, loop and if take a block type: 0x40 (empty) or a value type. Any
// other byte, including a type index (multi-value, not in 1.0), is malformed.
func TestOpcodeBlockTypes(t *testing.T) {
	valid := []byte{0x40, typeI32, typeI64, typeF32, typeF64}
	invalid := []byte{0x00, 0x01, 0x3F, 0x41, 0x6F, 0x70, 0x7B, 0x80, 0xFF}
	for _, op := range []wasm.Opcode{wasm.Block, wasm.Loop, wasm.If} {
		for _, bt := range valid {
			t.Run(fmt.Sprintf("%s/valid 0x%02X", opName(int(op)), bt), func(t *testing.T) {
				m := decodeOrSkip(t, bodyMod(byte(op), bt, opEnd, opEnd))
				got := opcodesOf(m.Code.Codes[0].Body)
				require.Equal(t, []wasm.Opcode{op, wasm.End, wasm.End}, got)
			})
		}
		for _, bt := range invalid {
			t.Run(fmt.Sprintf("%s/invalid 0x%02X", opName(int(op)), bt), func(t *testing.T) {
				err := testDecode(bodyMod(byte(op), bt, opEnd, opEnd))
				skipIfNotWired(t, err)
				require.Error(t, err)
			})
		}
	}
}

// The reserved byte after call_indirect's type index, and the only byte of
// memory.size and memory.grow, must be 0x00 (spec 5.4.1 and 5.4.6).
func TestOpcodeReservedBytes(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{"call_indirect reserved 0x01", []byte{0x11, 0x00, 0x01, opEnd}},
		{"call_indirect reserved 0x80 (padded LEB128 would be 0)", []byte{0x11, 0x00, 0x80, 0x00, opEnd}},
		{"memory.size reserved 0x01", []byte{0x3F, 0x01, opEnd}},
		{"memory.grow reserved 0x01", []byte{0x40, 0x01, opEnd}},
		{"memory.grow reserved 0xFF", []byte{0x40, 0xFF, opEnd}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := testDecode(bodyMod(tc.body...))
			skipIfNotWired(t, err)
			require.Error(t, err)
		})
	}
}

// else is only legal inside an if, and each if has at most one; end closes the
// innermost open block, and the function body needs exactly one more.
func TestOpcodeElseAndEnd(t *testing.T) {
	valid := []struct {
		name string
		body []byte
		want []wasm.Opcode
	}{
		{"if/else", []byte{0x41, 0x00, 0x04, 0x40, 0x01, 0x05, 0x01, opEnd, opEnd}, []wasm.Opcode{wasm.I32Const, wasm.If, wasm.Nop, wasm.Else, wasm.Nop, wasm.End, wasm.End}},
		{"if/else with empty arms", []byte{0x41, 0x00, 0x04, 0x40, 0x05, opEnd, opEnd}, []wasm.Opcode{wasm.I32Const, wasm.If, wasm.Else, wasm.End, wasm.End}},
		{"nested if/else in the else arm", []byte{0x41, 0x00, 0x04, 0x40, 0x05, 0x41, 0x00, 0x04, 0x40, 0x05, opEnd, opEnd, opEnd},
			[]wasm.Opcode{wasm.I32Const, wasm.If, wasm.Else, wasm.I32Const, wasm.If, wasm.Else, wasm.End, wasm.End, wasm.End}},
		{"else of an inner if, then else of the outer", []byte{0x41, 0x00, 0x04, 0x40, 0x41, 0x00, 0x04, 0x40, 0x05, opEnd, 0x05, opEnd, opEnd},
			[]wasm.Opcode{wasm.I32Const, wasm.If, wasm.I32Const, wasm.If, wasm.Else, wasm.End, wasm.Else, wasm.End, wasm.End}},
		{"if inside a loop inside a block", []byte{0x02, 0x40, 0x03, 0x40, 0x41, 0x00, 0x04, 0x40, 0x05, opEnd, opEnd, opEnd, opEnd},
			[]wasm.Opcode{wasm.Block, wasm.Loop, wasm.I32Const, wasm.If, wasm.Else, wasm.End, wasm.End, wasm.End, wasm.End}},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, bodyMod(tc.body...))
			require.Equal(t, tc.want, opcodesOf(m.Code.Codes[0].Body))
		})
	}

	malformed := []struct {
		name string
		body []byte
	}{
		{"else at function level", []byte{0x05, opEnd}},
		{"else in a block", []byte{0x02, 0x40, 0x05, opEnd, opEnd}},
		{"else in a loop", []byte{0x03, 0x40, 0x05, opEnd, opEnd}},
		{"else after the if has closed", []byte{0x41, 0x00, 0x04, 0x40, opEnd, 0x05, opEnd}},
		{"second else in one if", []byte{0x41, 0x00, 0x04, 0x40, 0x05, 0x01, 0x05, opEnd, opEnd}},
		{"else inside a block inside an if's then-arm", []byte{0x41, 0x00, 0x04, 0x40, 0x02, 0x40, 0x05, opEnd, opEnd, opEnd}},
		{"function end only", []byte{opEnd, opEnd}},
		{"block never closed", []byte{0x02, 0x40, 0x01, opEnd}},
		{"if never closed", []byte{0x41, 0x00, 0x04, 0x40, 0x05, opEnd}},
		{"no end at all", []byte{}},
	}
	for _, tc := range malformed {
		t.Run("malformed/"+tc.name, func(t *testing.T) {
			err := testDecode(bodyMod(tc.body...))
			skipIfNotWired(t, err)
			require.Error(t, err)
		})
	}
}

// Nesting depth must not cost recursion depth or quadratic time in the
// decoder: a body of a million nested blocks is a few megabytes of input.
func TestOpcodeDeepNesting(t *testing.T) {
	const depth = 1_000_000
	body := make([]byte, 0, 4*depth+2)
	for range depth {
		body = append(body, 0x02, 0x40)
	}
	for range depth {
		body = append(body, opEnd)
	}
	body = append(body, opEnd)
	// A body this big needs a multi-byte code entry size, so frame it by hand.
	entry := bs([]byte{0x00}, body)
	code := bs([]byte{secCode}, uleb(uint32(len(uleb(1))+len(uleb(uint32(len(entry))))+len(entry))), uleb(1), uleb(uint32(len(entry))), entry)
	in := mod(typeSec(), funcSec(), code)

	m := decodeOrSkip(t, in)
	require.Len(t, m.Code.Codes, 1)
	require.Len(t, m.Code.Codes[0].Body.Instructions, 2*depth+1)

	// Same input with the last end missing is malformed, not a hang or a panic.
	short := slices.Clone(body[:len(body)-1])
	entry = bs([]byte{0x00}, short)
	code = bs([]byte{secCode}, uleb(uint32(len(uleb(1))+len(uleb(uint32(len(entry))))+len(entry))), uleb(1), uleb(uint32(len(entry))), entry)
	err := testDecode(mod(typeSec(), funcSec(), code))
	require.Error(t, err)
}
