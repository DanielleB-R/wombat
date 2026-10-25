package decode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The modules below are assembled byte by byte from the WebAssembly 1.0
// binary grammar (spec chapter 5, "Binary Format"), not by running any
// encoder, so they test the decoder against the spec rather than against
// itself.

var header = []byte{0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x00}

// Section ids, spec 5.5.2.
const (
	secCustom   = 0
	secType     = 1
	secImport   = 2
	secFunc     = 3
	secTable    = 4
	secMemory   = 5
	secGlobal   = 6
	secExport   = 7
	secStart    = 8
	secElement  = 9
	secCode     = 10
	secData     = 11
	opEnd       = 0x0B
	typeFunc    = 0x60
	typeI32     = 0x7F
	typeI64     = 0x7E
	typeF32     = 0x7D
	typeF64     = 0x7C
	elemFuncref = 0x70
)

// bs concatenates byte slices.
func bs(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// uleb is a minimal unsigned LEB128 encoder (spec 5.2.2), kept here so the
// fixtures do not depend on the decoder under test.
func uleb(n uint32) []byte {
	var out []byte
	for {
		b := byte(n & 0x7F)
		n >>= 7
		if n == 0 {
			return append(out, b)
		}
		out = append(out, b|0x80)
	}
}

// sec frames body as a section: id, LEB128 size, contents.
func sec(id byte, body ...byte) []byte {
	return bs([]byte{id}, uleb(uint32(len(body))), body)
}

// typeSecN is a type section declaring n copies of () -> (). Its vector count
// is the minimal LEB128 of n, so counts of 64 and above exercise the byte
// values (0x40-0x7F, then multi-byte) where a signed read goes wrong.
func typeSecN(n int) []byte {
	body := uleb(uint32(n))
	for range n {
		body = append(body, typeFunc, 0x00, 0x00)
	}
	return sec(secType, body...)
}

// mod is a module made of the header followed by raw sections.
func mod(sections ...[]byte) []byte { return bs(header, bs(sections...)) }

// typeSec is a type section declaring the single function type () -> ().
func typeSec() []byte { return sec(secType, 0x01, typeFunc, 0x00, 0x00) }

// funcSec declares one function of type 0.
func funcSec() []byte { return sec(secFunc, 0x01, 0x00) }

// codeSec has one entry whose function body is locals ++ expr.
func codeSec(locals []byte, expr ...byte) []byte {
	entry := bs(locals, expr)
	return sec(secCode, bs([]byte{0x01, byte(len(entry))}, entry)...)
}

// bodyMod is a module with one function () -> () and the given body
// expression (which must include its terminating 0x0B where intended).
func bodyMod(expr ...byte) []byte {
	return mod(typeSec(), funcSec(), codeSec([]byte{0x00}, expr...))
}

type decodeCase struct {
	name string
	in   []byte
}

// validCases are well-formed binaries. Several are semantically invalid on
// purpose: the decoder implements chapter 5 only, and rejecting these belongs
// to the validator (chapter 3).
func validCases() []decodeCase {
	return []decodeCase{
		{"header only", header},
		{"empty type section", mod(sec(secType, 0x00))},
		{"empty function section matches an absent code section", mod(sec(secFunc, 0x00))},
		{"function type () -> ()", mod(typeSec())},
		{"function type with params and result", mod(sec(secType, 0x01, typeFunc, 0x02, typeI32, typeI64, 0x01, typeF32))},
		{"63 function types (largest count below the s7 sign bit)", mod(typeSecN(63))},
		{"64 function types (count byte 0x40 is u32 64, not s32 -64)", mod(typeSecN(64))},
		{"127 function types (count byte 0x7F is u32 127, not s32 -1)", mod(typeSecN(127))},
		{"128 function types (two-byte count)", mod(typeSecN(128))},
		{"function type with f64", mod(sec(secType, 0x01, typeFunc, 0x01, typeF64, 0x00))},
		{"padded section size (0x84 0x00 == 4)", mod([]byte{secType, 0x84, 0x00, 0x01, typeFunc, 0x00, 0x00})},
		{"import func", mod(typeSec(), sec(secImport, 0x01, 0x01, 'a', 0x01, 'b', 0x00, 0x00))},
		{"import table", mod(sec(secImport, 0x01, 0x01, 'a', 0x01, 'b', 0x01, elemFuncref, 0x00, 0x01))},
		{"import memory with max", mod(sec(secImport, 0x01, 0x01, 'a', 0x01, 'b', 0x02, 0x01, 0x01, 0x02))},
		{"import immutable global", mod(sec(secImport, 0x01, 0x01, 'a', 0x01, 'b', 0x03, typeI32, 0x00))},
		{"import with empty names", mod(sec(secImport, 0x01, 0x00, 0x00, 0x03, typeI64, 0x00))},
		{"import name with 3-byte UTF-8 (euro sign)", mod(sec(secImport, 0x01, 0x03, 0xE2, 0x82, 0xAC, 0x01, 'b', 0x03, typeI32, 0x00))},
		{"import name with 4-byte UTF-8 (U+1F428)", mod(sec(secImport, 0x01, 0x04, 0xF0, 0x9F, 0x90, 0xA8, 0x01, 'b', 0x03, typeI32, 0x00))},
		{"function section", mod(typeSec(), funcSec(), codeSec([]byte{0x00}, opEnd))},
		{"table", mod(sec(secTable, 0x01, elemFuncref, 0x00, 0x01))},
		{"table with max", mod(sec(secTable, 0x01, elemFuncref, 0x01, 0x01, 0x02))},
		{"memory", mod(sec(secMemory, 0x01, 0x00, 0x01))},
		{"memory with max", mod(sec(secMemory, 0x01, 0x01, 0x01, 0x02))},
		{"memory limits with min > max (validation error, not malformed)", mod(sec(secMemory, 0x01, 0x01, 0x02, 0x01))},
		{"memory limits are u32 LEB128 (65536 pages)", mod(sec(secMemory, 0x01, 0x01, 0x80, 0x80, 0x04, 0x80, 0x80, 0x04))},
		{"immutable global", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x41, 0x00, opEnd))},
		{"mutable global", mod(sec(secGlobal, 0x01, typeI64, 0x01, 0x42, 0x00, opEnd))},
		{"f32 global", mod(sec(secGlobal, 0x01, typeF32, 0x00, 0x43, 0x00, 0x00, 0x80, 0x3F, opEnd))},
		{"f64 global", mod(sec(secGlobal, 0x01, typeF64, 0x00, 0x44, 0, 0, 0, 0, 0, 0, 0xF0, 0x3F, opEnd))},
		{"global init: f32.const whose payload bytes are all 0x0B (end)", mod(sec(secGlobal, 0x01, typeF32, 0x00, 0x43, 0x0B, 0x0B, 0x0B, 0x0B, opEnd))},
		{"global init: f64.const whose payload bytes are all 0x0B (end)", mod(sec(secGlobal, 0x01, typeF64, 0x00, 0x44, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, opEnd))},
		{"global init: i32.const with a two-byte LEB128 (0x8B 0x00 is 11)", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x41, 0x8B, 0x00, opEnd))},
		{"global init: global.get with a multi-byte index", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x23, 0x80, 0x01, opEnd))},
		{"global init: empty expression decodes (validator rejects)", mod(sec(secGlobal, 0x01, typeI32, 0x00, opEnd))},
		{"global init: two constants decode (validator rejects)", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x41, 0x00, 0x41, 0x00, opEnd))},
		{"global init: non-constant instruction decodes (validator rejects)", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x01, 0x41, 0x00, opEnd))},
		{"global initialised from global.get", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x23, 0x00, opEnd))},
		{"export func", mod(sec(secExport, 0x01, 0x01, 'a', 0x00, 0x00))},
		{"export table, memory, global", mod(sec(secExport, 0x03, 0x01, 'a', 0x01, 0x00, 0x01, 'b', 0x02, 0x00, 0x01, 'c', 0x03, 0x00))},
		{"export name with euro sign", mod(sec(secExport, 0x01, 0x03, 0xE2, 0x82, 0xAC, 0x00, 0x00))},
		{"export index is u32 (multi-byte)", mod(sec(secExport, 0x01, 0x01, 'a', 0x00, 0x80, 0x01))},
		{"start", mod(sec(secStart, 0x00))},
		{"element segment", mod(sec(secElement, 0x01, 0x00, 0x41, 0x00, opEnd, 0x01, 0x00))},
		{"element segment, empty func list", mod(sec(secElement, 0x01, 0x00, 0x41, 0x00, opEnd, 0x00))},
		{"data segment", mod(sec(secData, 0x01, 0x00, 0x41, 0x00, opEnd, 0x02, 0xAA, 0xBB))},
		{"data segment, empty bytes", mod(sec(secData, 0x01, 0x00, 0x41, 0x00, opEnd, 0x00))},
		{"data offset from global.get", mod(sec(secData, 0x01, 0x00, 0x23, 0x00, opEnd, 0x00))},
		{"locals groups", mod(typeSec(), funcSec(), codeSec([]byte{0x02, 0x02, typeI32, 0x01, typeF64}, opEnd))},
		{"locals group with count 0", mod(typeSec(), funcSec(), codeSec([]byte{0x01, 0x00, typeI32}, opEnd))},
		{"custom section first", mod(sec(secCustom, 0x03, 'a', 'b', 'c', 0xFF), typeSec())},
		{"custom section between sections", mod(typeSec(), sec(secCustom, 0x01, 'x'), funcSec(), codeSec([]byte{0x00}, opEnd))},
		{"custom section last", mod(typeSec(), sec(secCustom, 0x01, 'x', 0x00, 0x01))},
		{"custom section with empty name", mod(sec(secCustom, 0x00))},
		{"several custom sections", mod(sec(secCustom, 0x01, 'a'), sec(secCustom, 0x01, 'a'), sec(secCustom, 0x01, 'b'))},
		{"custom section contents after the name are never interpreted", mod(sec(secCustom, 0x01, 'x', 0xC0, 0x80, 0xFF))},
		{"all sections in order", mod(
			typeSec(),
			sec(secImport, 0x01, 0x01, 'a', 0x01, 'b', 0x00, 0x00),
			funcSec(),
			sec(secTable, 0x01, elemFuncref, 0x00, 0x01),
			sec(secMemory, 0x01, 0x00, 0x01),
			sec(secGlobal, 0x01, typeI32, 0x00, 0x41, 0x00, opEnd),
			sec(secExport, 0x01, 0x01, 'f', 0x00, 0x01),
			sec(secStart, 0x01),
			sec(secElement, 0x01, 0x00, 0x41, 0x00, opEnd, 0x01, 0x01),
			codeSec([]byte{0x00}, opEnd),
			sec(secData, 0x01, 0x00, 0x41, 0x00, opEnd, 0x01, 0x07),
		)},

		// Function bodies. The decoder reads instructions, immediates and
		// nesting; whether the bodies type-check is the validator's job.
		{"body: nop", bodyMod(0x01, opEnd)},
		{"body: unreachable", bodyMod(0x00, opEnd)},
		{"body: empty block", bodyMod(0x02, 0x40, opEnd, opEnd)},
		{"body: block with result type", bodyMod(0x02, typeI32, 0x41, 0x00, opEnd, 0x1A, opEnd)},
		{"body: loop with br", bodyMod(0x03, 0x40, 0x0C, 0x00, opEnd, opEnd)},
		{"body: if without else", bodyMod(0x41, 0x01, 0x04, 0x40, opEnd, opEnd)},
		{"body: if with else", bodyMod(0x41, 0x01, 0x04, 0x40, 0x01, 0x05, 0x01, opEnd, opEnd)},
		{"body: nested blocks", bodyMod(0x02, 0x40, 0x03, 0x40, 0x02, 0x40, opEnd, opEnd, opEnd, opEnd)},
		{"body: br_if", bodyMod(0x02, 0x40, 0x41, 0x01, 0x0D, 0x00, opEnd, opEnd)},
		{"body: br_table with labels", bodyMod(0x02, 0x40, 0x41, 0x00, 0x0E, 0x02, 0x00, 0x00, 0x00, opEnd, opEnd)},
		{"body: br_table with no labels", bodyMod(0x02, 0x40, 0x41, 0x00, 0x0E, 0x00, 0x00, opEnd, opEnd)},
		{"body: return", bodyMod(0x0F, opEnd)},
		{"body: call", bodyMod(0x10, 0x00, opEnd)},
		{"body: call_indirect", bodyMod(0x41, 0x00, 0x11, 0x00, 0x00, opEnd)},
		{"body: drop and select", bodyMod(0x41, 0, 0x41, 0, 0x41, 0, 0x1B, 0x1A, opEnd)},
		{"body: local and global access", bodyMod(0x20, 0x00, 0x21, 0x00, 0x20, 0x00, 0x22, 0x00, 0x23, 0x00, 0x24, 0x00, opEnd)},
		{"body: i32.load with memarg", bodyMod(0x41, 0x00, 0x28, 0x02, 0x00, 0x1A, opEnd)},
		{"body: i64.store32 with offset", bodyMod(0x41, 0x00, 0x42, 0x00, 0x3E, 0x02, 0x80, 0x01, opEnd)},
		{"body: memory.size and memory.grow", bodyMod(0x3F, 0x00, 0x1A, 0x41, 0x01, 0x40, 0x00, 0x1A, opEnd)},
		{"body: i32.const -1 (one byte)", bodyMod(0x41, 0x7F, 0x1A, opEnd)},
		{"body: i32.const -1 (five bytes)", bodyMod(0x41, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0x1A, opEnd)},
		{"body: i32.const min", bodyMod(0x41, 0x80, 0x80, 0x80, 0x80, 0x78, 0x1A, opEnd)},
		{"body: i64.const min", bodyMod(0x42, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x7F, 0x1A, opEnd)},
		{"body: f32.const 1.0", bodyMod(0x43, 0x00, 0x00, 0x80, 0x3F, 0x1A, opEnd)},
		{"body: f64.const 1.0", bodyMod(0x44, 0, 0, 0, 0, 0, 0, 0xF0, 0x3F, 0x1A, opEnd)},
		{"body: f32.const whose payload bytes look like end/else", bodyMod(0x43, 0x0B, 0x05, 0x0B, 0x05, 0x1A, opEnd)},
		{"body: first numeric opcode (i32.eqz)", bodyMod(0x41, 0x00, 0x45, 0x1A, opEnd)},
		{"body: last numeric opcode (f64.reinterpret_i64)", bodyMod(0x42, 0x00, 0xBF, 0x1A, opEnd)},
		{"body: i32.wrap_i64", bodyMod(0x42, 0x00, 0xA7, 0x1A, opEnd)},
	}
}

// malformedCases must all be rejected by the decoder (spec 5, "malformed").
func malformedCases() []decodeCase {
	return []decodeCase{
		// Preamble, 5.5.1.
		{"empty input", nil},
		{"short magic", header[:3]},
		{"magic only", header[:4]},
		{"short version", header[:7]},
		{"wrong magic byte", []byte{0x00, 0x61, 0x73, 0x6E, 0x01, 0x00, 0x00, 0x00}},
		{"magic in wrong case", []byte{0x00, 0x41, 0x53, 0x4D, 0x01, 0x00, 0x00, 0x00}},
		{"version 0", []byte{0x00, 0x61, 0x73, 0x6D, 0x00, 0x00, 0x00, 0x00}},
		{"version 2", []byte{0x00, 0x61, 0x73, 0x6D, 0x02, 0x00, 0x00, 0x00}},
		{"version big-endian", []byte{0x00, 0x61, 0x73, 0x6D, 0x00, 0x00, 0x00, 0x01}},
		{"version with a nonzero high byte", []byte{0x00, 0x61, 0x73, 0x6D, 0x01, 0x00, 0x00, 0x01}},

		// Section framing, 5.5.2.
		{"section id with no size", bs(header, []byte{secType})},
		{"section size runs past the end", bs(header, []byte{secType, 0x05, 0x01, typeFunc, 0x00, 0x00})},
		{"section size larger than its contents", bs(header, []byte{secType, 0x05, 0x01, typeFunc, 0x00, 0x00, 0x00})},
		{"section size smaller than its contents", bs(header, []byte{secType, 0x03, 0x01, typeFunc, 0x00, 0x00})},
		{"section size u32 has six bytes", bs(header, []byte{secType, 0x84, 0x80, 0x80, 0x80, 0x80, 0x00, 0x01, typeFunc, 0x00, 0x00})},
		{"section size u32 has unused bits set", bs(header, []byte{secCustom, 0xFF, 0xFF, 0xFF, 0xFF, 0x1F})},
		{"unknown section id 12", bs(header, []byte{0x0C, 0x00})},
		{"unknown section id 13", bs(header, []byte{0x0D, 0x00})},
		{"unknown section id 0xFF", bs(header, []byte{0xFF, 0x00})},
		{"custom section too short for its name", mod(sec(secCustom, 0x05, 'a'))},
		{"custom section with truncated name length", mod([]byte{secCustom, 0x01, 0x80})},
		{"empty custom section (no name length)", mod([]byte{secCustom, 0x00})},

		// Ordering and uniqueness, 5.5.2.
		{"duplicate type section", mod(typeSec(), typeSec())},
		{"duplicate memory section", mod(sec(secMemory, 0x00), sec(secMemory, 0x00))},
		{"duplicate type section with a custom section between", mod(typeSec(), sec(secCustom, 0x01, 'x'), typeSec())},
		{"table section after memory section with a custom section between", mod(sec(secMemory, 0x00), sec(secCustom, 0x01, 'x'), sec(secTable, 0x00))},
		{"function section before type section", mod(sec(secFunc, 0x00), typeSec())},
		{"memory section before table section", mod(sec(secMemory, 0x00), sec(secTable, 0x00))},
		{"global section before memory section", mod(sec(secGlobal, 0x00), sec(secMemory, 0x00))},
		{"export section before global section", mod(sec(secExport, 0x00), sec(secGlobal, 0x00))},
		{"element section before start section", mod(sec(secElement, 0x00), sec(secStart, 0x00))},
		{"data section before code section", mod(sec(secData, 0x00), sec(secCode, 0x00))},
		{"import section before type section", mod(sec(secImport, 0x00), typeSec())},
		{"code section before function section", mod(sec(secCode, 0x00), sec(secFunc, 0x00))},

		// Function and code section must agree, 5.5.9 / 5.5.13.
		{"function section without code section", mod(typeSec(), funcSec())},
		{"code section without function section", mod(typeSec(), codeSec([]byte{0x00}, opEnd))},
		{"two functions, one body", mod(typeSec(), sec(secFunc, 0x02, 0x00, 0x00), codeSec([]byte{0x00}, opEnd))},
		{"one function, two bodies", mod(typeSec(), funcSec(), sec(secCode, 0x02, 0x02, 0x00, opEnd, 0x02, 0x00, opEnd))},

		// Code entries, 5.5.13.
		{"code entry size too large", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x03, 0x00, opEnd, opEnd))},
		{"code entry size too small", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x01, 0x00, opEnd))},
		{"code entry size runs past section", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x05, 0x00, opEnd))},
		{"locals total 2^32 overflows", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x0A,
			0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F, typeI32, 0x01, typeI32, opEnd))},
		{"local of invalid type v128", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x04, 0x01, 0x01, 0x7B, opEnd))},
		{"local of block type 0x40", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x04, 0x01, 0x01, 0x40, opEnd))},
		{"locals vector truncated", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x02, 0x01, 0x01))},

		// Types, 5.3.
		{"functype marker is not 0x60", mod(sec(secType, 0x01, 0x61, 0x00, 0x00))},
		{"functype marker is a value type", mod(sec(secType, 0x01, typeI32, 0x00, 0x00))},
		{"param of invalid type v128", mod(sec(secType, 0x01, typeFunc, 0x01, 0x7B, 0x00))},
		{"param of invalid type 0x40", mod(sec(secType, 0x01, typeFunc, 0x01, 0x40, 0x00))},
		{"result of invalid type funcref", mod(sec(secType, 0x01, typeFunc, 0x00, 0x01, 0x70))},
		{"param vector longer than the data", mod(sec(secType, 0x01, typeFunc, 0x05, typeI32, 0x00))},
		{"type vector longer than the data", mod(sec(secType, 0x02, typeFunc, 0x00, 0x00))},
		{"type vector with huge count and no data", mod([]byte{secType, 0x05, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F})},
		{"vector count has six bytes", mod([]byte{secType, 0x07, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00, 0x00})},
		{"vector count has unused bits set", mod([]byte{secType, 0x05, 0xFF, 0xFF, 0xFF, 0xFF, 0x1F})},
		{"import vector with huge count", mod([]byte{secImport, 0x05, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F})},
		{"code vector with huge count", mod([]byte{secCode, 0x05, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F})},
		{"data vector with huge byte count", mod(sec(secData, 0x01, 0x00, 0x41, 0x00, opEnd, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F))},

		// Limits, memory, table, global types, 5.3.4 - 5.3.7.
		{"limits flag 2", mod(sec(secMemory, 0x01, 0x02, 0x01))},
		{"limits flag 4 (64-bit memory, not in 1.0)", mod(sec(secMemory, 0x01, 0x04, 0x01))},
		{"limits flag 3", mod(sec(secMemory, 0x01, 0x03, 0x01, 0x02))},
		{"limits missing min", mod(sec(secMemory, 0x01, 0x00))},
		{"limits flag 1 missing max", mod(sec(secMemory, 0x01, 0x01, 0x01))},
		{"table elemtype externref (0x6F)", mod(sec(secTable, 0x01, 0x6F, 0x00, 0x01))},
		{"table elemtype is a value type", mod(sec(secTable, 0x01, typeI32, 0x00, 0x01))},
		{"table with bad limits flag", mod(sec(secTable, 0x01, elemFuncref, 0x02, 0x01))},
		{"global mutability 2", mod(sec(secGlobal, 0x01, typeI32, 0x02, 0x41, 0x00, opEnd))},
		{"global of invalid type", mod(sec(secGlobal, 0x01, 0x7B, 0x00, 0x41, 0x00, opEnd))},
		{"global missing init expression", mod(sec(secGlobal, 0x01, typeI32, 0x00))},
		{"global init: f32.const payload cut short by the section end", mod(sec(secGlobal, 0x01, typeF32, 0x00, 0x43, 0x00, 0x00, opEnd))},
		{"global init: f64.const payload cut short by the section end", mod(sec(secGlobal, 0x01, typeF64, 0x00, 0x44, 0, 0, 0, 0, 0, 0, opEnd))},
		{"global init: i32.const LEB128 swallows the end byte (0x8B 0x0B)", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x41, 0x8B, opEnd))},
		{"global init: i32.const with no immediate", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x41))},
		{"global: second entry missing", mod(sec(secGlobal, 0x02, typeI32, 0x00, 0x41, 0x00, opEnd))},
		{"global init expression not terminated", mod(sec(secGlobal, 0x01, typeI32, 0x00, 0x41, 0x00))},

		// Imports and exports, 5.5.5 / 5.5.10.
		{"import descriptor tag 4", mod(sec(secImport, 0x01, 0x01, 'a', 0x01, 'b', 0x04, 0x00))},
		{"import descriptor tag 0xFF", mod(sec(secImport, 0x01, 0x01, 'a', 0x01, 'b', 0xFF, 0x00))},
		{"import missing descriptor", mod(sec(secImport, 0x01, 0x01, 'a', 0x01, 'b'))},
		{"import field name overruns", mod(sec(secImport, 0x01, 0x01, 'a', 0x05, 'b', 0x00, 0x00))},
		{"export descriptor tag 4", mod(sec(secExport, 0x01, 0x01, 'a', 0x04, 0x00))},
		{"export missing index", mod(sec(secExport, 0x01, 0x01, 'a', 0x00))},
		{"export name longer than the section", mod(sec(secExport, 0x01, 0x09, 'a', 0x00, 0x00))},

		// Names are UTF-8, 5.2.4. Surrogates, overlong forms and stray
		// continuation bytes are all malformed.
		{"export name: lone continuation byte", mod(sec(secExport, 0x01, 0x01, 0x80, 0x00, 0x00))},
		{"export name: 0xFF", mod(sec(secExport, 0x01, 0x01, 0xFF, 0x00, 0x00))},
		{"export name: truncated two-byte sequence", mod(sec(secExport, 0x01, 0x01, 0xC2, 0x00, 0x00))},
		{"export name: overlong NUL (C0 80)", mod(sec(secExport, 0x01, 0x02, 0xC0, 0x80, 0x00, 0x00))},
		{"export name: overlong '/' (E0 80 AF)", mod(sec(secExport, 0x01, 0x03, 0xE0, 0x80, 0xAF, 0x00, 0x00))},
		{"export name: UTF-16 surrogate (ED A0 80)", mod(sec(secExport, 0x01, 0x03, 0xED, 0xA0, 0x80, 0x00, 0x00))},
		{"export name: above U+10FFFF (F4 90 80 80)", mod(sec(secExport, 0x01, 0x04, 0xF4, 0x90, 0x80, 0x80, 0x00, 0x00))},
		{"export name: five-byte form (F8 88 80 80 80)", mod(sec(secExport, 0x01, 0x05, 0xF8, 0x88, 0x80, 0x80, 0x80, 0x00, 0x00))},
		{"import module name: invalid UTF-8", mod(sec(secImport, 0x01, 0x01, 0xFF, 0x01, 'b', 0x03, typeI32, 0x00))},
		{"import field name: invalid UTF-8", mod(sec(secImport, 0x01, 0x01, 'a', 0x01, 0xFF, 0x03, typeI32, 0x00))},
		{"custom section name: invalid UTF-8", mod(sec(secCustom, 0x01, 0xFF))},

		// Start, element and data, 5.5.11 - 5.5.14.
		{"start section empty", mod(sec(secStart))},
		{"start index truncated", mod([]byte{secStart, 0x01, 0x80})},
		{"element offset expression not terminated", mod(sec(secElement, 0x01, 0x00, 0x41, 0x00, 0x01, 0x00))},
		{"element function vector truncated", mod(sec(secElement, 0x01, 0x00, 0x41, 0x00, opEnd, 0x03, 0x00))},
		{"data offset expression not terminated", mod(sec(secData, 0x01, 0x00, 0x41, 0x00, 0x00))},
		{"data bytes truncated", mod(sec(secData, 0x01, 0x00, 0x41, 0x00, opEnd, 0x03, 0xAA))},
		{"trailing byte after the data section", bs(mod(sec(secData, 0x00)), []byte{0x00})},

		// Instruction encoding, 5.4.
		{"body without end", bodyMod(0x01)},
		{"body: block without end", bodyMod(0x02, 0x40, opEnd)},
		{"body: extra end", bodyMod(opEnd, opEnd)},
		{"body: else outside any if", bodyMod(0x05, opEnd)},
		{"body: else in a block", bodyMod(0x02, 0x40, 0x05, opEnd, opEnd)},
		{"body: two elses", bodyMod(0x41, 0, 0x04, 0x40, 0x05, 0x05, opEnd, opEnd)},
		{"body: if without end", bodyMod(0x41, 0x00, 0x04, 0x40, opEnd)},
		{"body: block type is a type index (multi-value, not in 1.0)", bodyMod(0x02, 0x00, opEnd, opEnd)},
		{"body: block type v128", bodyMod(0x02, 0x7B, opEnd, opEnd)},
		{"body: block type funcref", bodyMod(0x02, 0x70, opEnd, opEnd)},
		{"body: block type missing", bodyMod(0x02)},
		{"body: call_indirect reserved byte nonzero", bodyMod(0x41, 0x00, 0x11, 0x00, 0x01, opEnd)},
		{"body: call_indirect reserved byte missing", bodyMod(0x11, 0x00)},
		{"body: memory.size reserved byte nonzero", bodyMod(0x3F, 0x01, 0x1A, opEnd)},
		{"body: memory.grow reserved byte nonzero", bodyMod(0x41, 0x00, 0x40, 0x01, 0x1A, opEnd)},
		{"body: br_table count past the data", bodyMod(0x0E, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F, opEnd)},
		{"body: br_table missing default", bodyMod(0x0E, 0x00)},
		{"body: i32.const overlong (six bytes)", bodyMod(0x41, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00, 0x1A, opEnd)},
		{"body: i32.const with bad unused bits", bodyMod(0x41, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F, 0x1A, opEnd)},
		{"body: i32.const truncated", bodyMod(0x41, 0x80)},
		{"body: i64.const with bad unused bits", bodyMod(0x42, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01, 0x1A, opEnd)},
		{"body: i64.const eleven bytes", bodyMod(0x42, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x1A, opEnd)},
		{"body: f32.const truncated", bodyMod(0x43, 0x00, 0x00)},
		{"body: f64.const truncated", bodyMod(0x44, 0, 0, 0, 0, 0, 0, 0)},
		{"body: load missing offset", bodyMod(0x28, 0x02)},
		{"body: local.get missing index", bodyMod(0x20)},
		{"body: opcode 0x06 (reserved)", bodyMod(0x06, opEnd)},
		{"body: opcode 0x07 (reserved)", bodyMod(0x07, opEnd)},
		{"body: opcode 0x08 (reserved)", bodyMod(0x08, opEnd)},
		{"body: opcode 0x09 (reserved)", bodyMod(0x09, opEnd)},
		{"body: opcode 0x0A (reserved)", bodyMod(0x0A, opEnd)},
		{"body: opcode 0x12 (return_call, post-1.0)", bodyMod(0x12, 0x00, opEnd)},
		{"body: opcode 0x1C (select t, post-1.0)", bodyMod(0x1C, 0x00, opEnd)},
		{"body: opcode 0x25 (table.get, post-1.0)", bodyMod(0x25, 0x00, opEnd)},
		{"body: opcode 0x27 (reserved)", bodyMod(0x27, opEnd)},
		{"body: opcode 0xC0 (i32.extend8_s, post-1.0)", bodyMod(0x41, 0x00, 0xC0, 0x1A, opEnd)},
		{"body: opcode 0xC4 (i64.extend32_s, post-1.0)", bodyMod(0x42, 0x00, 0xC4, 0x1A, opEnd)},
		{"body: opcode 0xD0 (ref.null, post-1.0)", bodyMod(0xD0, 0x70, opEnd)},
		{"body: prefix 0xFC (sat trunc/bulk memory, post-1.0)", bodyMod(0xFC, 0x00, opEnd)},
		{"body: prefix 0xFD (SIMD, post-1.0)", bodyMod(0xFD, 0x00, opEnd)},
		{"body: prefix 0xFE (threads, post-1.0)", bodyMod(0xFE, 0x00, opEnd)},
		{"body: opcode 0xFF", bodyMod(0xFF, opEnd)},
	}
}

func TestDecodeValid(t *testing.T) {
	for _, tc := range validCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := testDecode(tc.in)
			skipIfNotWired(t, err)
			require.NoError(t, err)
		})
	}
}

func TestDecodeMalformed(t *testing.T) {
	for _, tc := range malformedCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := testDecode(tc.in)
			skipIfNotWired(t, err)
			require.Error(t, err)
		})
	}
}

// Cutting a well-formed module anywhere inside a section (or inside a section
// header) must give an error and never a panic. A cut exactly at a section
// boundary may or may not be a well-formed module (dropping the code section
// but not the function section is malformed, dropping the data section is
// fine), so there we only require that the decoder returns.
func TestDecodeTruncatedFixtures(t *testing.T) {
	for _, path := range fixturePaths(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)

			err = testDecode(data)
			skipIfNotWired(t, err)
			require.NoError(t, err, "the full file must decode")

			boundaries := sectionBoundaries(t, data)
			for n := 0; n < len(data); n++ {
				if boundaries[n] {
					_ = testDecode(data[:n])
					continue
				}
				require.Error(t, testDecode(data[:n]), "cut at byte %d of %d", n, len(data))
			}
		})
	}
}

// Real modules assembled by wat2wasm (see `make testdata-wasm`). Between them
// they cover every MVP section.
func TestDecodeFixtures(t *testing.T) {
	for _, path := range fixturePaths(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			err = testDecode(data)
			skipIfNotWired(t, err)
			require.NoError(t, err)
		})
	}
}

func fixturePaths(t testing.TB) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "testdata", "wasm", "*.wasm"))
	require.NoError(t, err)
	require.NotEmpty(t, paths, "no fixtures; run `make testdata-wasm`")
	return paths
}

// sectionBoundaries returns the offsets at which data[:n] is a complete
// sequence of sections (n == 8 is the bare header). It walks the framing
// itself with a throwaway LEB128 reader so it does not depend on the code
// under test. Fixtures only use sizes below 2^14, so two bytes suffice.
func sectionBoundaries(t testing.TB, data []byte) map[int]bool {
	t.Helper()
	out := map[int]bool{len(header): true}
	i := len(header)
	for i < len(data) {
		i++ // section id
		size, w := 0, 0
		for shift := 0; ; shift += 7 {
			b := data[i]
			i++
			size |= int(b&0x7F) << shift
			w++
			if b&0x80 == 0 {
				break
			}
		}
		require.LessOrEqual(t, w, 2, "fixture section size too large for this helper")
		i += size
		out[i] = true
	}
	return out
}

// FuzzDecode checks the one hard requirement CLAUDE.md puts on the decoder:
// arbitrary input yields an error or a module, never a panic, hang or
// runaway allocation. Seed corpus: every case above plus the fixture files.
func FuzzDecode(f *testing.F) {
	for _, tc := range validCases() {
		f.Add(tc.in)
	}
	for _, tc := range malformedCases() {
		f.Add(tc.in)
	}
	paths, _ := filepath.Glob(filepath.Join("..", "testdata", "wasm", "*.wasm"))
	for _, p := range paths {
		if data, err := os.ReadFile(p); err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		err := testDecode(data)
		skipIfNotWired(t, err)
	})
}
