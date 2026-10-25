package decode

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DanielleB-R/wombat/wasm"
)

// These tests check what the decoder produced, not just that it accepted the
// input. Expected values come from the bytes written in each case (read off
// the spec grammar) or, for the fixture files, from `wasm-objdump -x`.

func decodeOrSkip(t *testing.T, in []byte) *wasm.Module {
	t.Helper()
	m, err := testDecodeModule(in)
	skipIfNotWired(t, err)
	require.NoError(t, err)
	require.NotNil(t, m)
	return m
}

func importsOf(m *wasm.Module) []wasm.Import {
	if m.Import == nil {
		return nil
	}
	return m.Import.Imports
}

func requireFuncType(t *testing.T, got wasm.FunctionType, params, results []wasm.ValType) {
	t.Helper()
	// slices.Equal treats nil and empty as equal; the representation of "no
	// results" is up to the decoder.
	require.True(t, slices.Equal(params, got.Parameters), "params: want %v, got %v", params, got.Parameters)
	require.True(t, slices.Equal(results, got.Results), "results: want %v, got %v", results, got.Results)
}

func TestDecodeTypeSectionContents(t *testing.T) {
	// One of each shape: (), (i32 i64) -> f32, (f64) -> (), () -> i32.
	in := mod(sec(secType, 0x04,
		typeFunc, 0x00, 0x00,
		typeFunc, 0x02, typeI32, typeI64, 0x01, typeF32,
		typeFunc, 0x01, typeF64, 0x00,
		typeFunc, 0x00, 0x01, typeI32,
	))
	m := decodeOrSkip(t, in)
	require.NotNil(t, m.Type)
	require.Len(t, m.Type.FunctionTypes, 4)
	requireFuncType(t, m.Type.FunctionTypes[0], nil, nil)
	requireFuncType(t, m.Type.FunctionTypes[1], []wasm.ValType{wasm.I32, wasm.I64}, []wasm.ValType{wasm.F32})
	requireFuncType(t, m.Type.FunctionTypes[2], []wasm.ValType{wasm.F64}, nil)
	requireFuncType(t, m.Type.FunctionTypes[3], nil, []wasm.ValType{wasm.I32})
}

func TestDecodeImportContents(t *testing.T) {
	// Every import kind and the descriptor shapes from spec 5.5.5:
	//   0x00 typeidx | 0x01 tabletype | 0x02 limits | 0x03 globaltype
	tests := []struct {
		name string
		body []byte // import section body: vec count, then imports
		want []wasm.Import
	}{
		{
			name: "function, index 5",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x00, 0x05},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.FuncKind, FuncTypeIdx: 5}}},
		},
		{
			name: "function, multi-byte index (0x80 0x01 = 128)",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x00, 0x80, 0x01},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.FuncKind, FuncTypeIdx: 128}}},
		},
		{
			name: "table, min only",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x01, elemFuncref, 0x00, 0x03},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.TableKind, TableLimits: wasm.Limits{Min: 3}}}},
		},
		{
			name: "table, min and max",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x01, elemFuncref, 0x01, 0x02, 0x09},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.TableKind, TableLimits: wasm.Limits{HasMax: true, Min: 2, Max: 9}}}},
		},
		{
			name: "memory, min and max",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x02, 0x01, 0x01, 0x02},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.MemKind, MemLimits: wasm.Limits{HasMax: true, Min: 1, Max: 2}}}},
		},
		{
			name: "memory, multi-byte limits (65536 pages)",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x02, 0x01, 0x80, 0x80, 0x04, 0x80, 0x80, 0x04},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.MemKind, MemLimits: wasm.Limits{HasMax: true, Min: 65536, Max: 65536}}}},
		},
		{
			name: "memory, min 0 with no max",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x02, 0x00, 0x00},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.MemKind}}},
		},
		{
			name: "global, immutable i32",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x03, typeI32, 0x00},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.GlobalKind, GlobalType: wasm.GlobalType{ValType: wasm.I32}}}},
		},
		{
			name: "global, mutable i64",
			body: []byte{0x01, 0x01, 'a', 0x01, 'b', 0x03, typeI64, 0x01},
			want: []wasm.Import{{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.GlobalKind, GlobalType: wasm.GlobalType{ValType: wasm.I64, Mutable: true}}}},
		},
		{
			name: "global, immutable f32 and f64",
			body: []byte{0x02, 0x01, 'a', 0x01, 'b', 0x03, typeF32, 0x00, 0x01, 'a', 0x01, 'c', 0x03, typeF64, 0x00},
			want: []wasm.Import{
				{Module: "a", Name: "b", Desc: wasm.ImportDesc{Kind: wasm.GlobalKind, GlobalType: wasm.GlobalType{ValType: wasm.F32}}},
				{Module: "a", Name: "c", Desc: wasm.ImportDesc{Kind: wasm.GlobalKind, GlobalType: wasm.GlobalType{ValType: wasm.F64}}},
			},
		},
		{
			name: "empty module and field names",
			body: []byte{0x01, 0x00, 0x00, 0x00, 0x00},
			want: []wasm.Import{{Desc: wasm.ImportDesc{Kind: wasm.FuncKind}}},
		},
		{
			name: "multi-byte UTF-8 names (euro sign, koala)",
			body: []byte{0x01, 0x03, 0xE2, 0x82, 0xAC, 0x04, 0xF0, 0x9F, 0x90, 0xA8, 0x00, 0x00},
			want: []wasm.Import{{Module: "€", Name: "\U0001F428", Desc: wasm.ImportDesc{Kind: wasm.FuncKind}}},
		},
		{
			name: "several imports keep their order across kinds",
			body: []byte{
				0x04,
				0x01, 'm', 0x01, 'f', 0x00, 0x01,
				0x01, 'm', 0x01, 'g', 0x02, 0x00, 0x01,
				0x01, 'm', 0x01, 'h', 0x00, 0x00,
				0x01, 'n', 0x01, 'f', 0x03, typeI32, 0x00,
			},
			want: []wasm.Import{
				{Module: "m", Name: "f", Desc: wasm.ImportDesc{Kind: wasm.FuncKind, FuncTypeIdx: 1}},
				{Module: "m", Name: "g", Desc: wasm.ImportDesc{Kind: wasm.MemKind, MemLimits: wasm.Limits{Min: 1}}},
				{Module: "m", Name: "h", Desc: wasm.ImportDesc{Kind: wasm.FuncKind, FuncTypeIdx: 0}},
				{Module: "n", Name: "f", Desc: wasm.ImportDesc{Kind: wasm.GlobalKind, GlobalType: wasm.GlobalType{ValType: wasm.I32}}},
			},
		},
		{
			name: "empty section",
			body: []byte{0x00},
			want: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(sec(secImport, tc.body...)))
			got := importsOf(m)
			// Comparing whole structs also checks that fields belonging to
			// the other kinds are left at their zero values.
			if len(tc.want) == 0 {
				require.Empty(t, got)
				return
			}
			require.Equal(t, tc.want, got)
		})
	}
}

// testdata/wasm/imports.wasm, as `wasm-objdump -x` describes it:
//
//	Type[2]:   (i32, i64) -> f64 ; (i32) -> nil
//	Import[5]: func sig=1 env.log ; func sig=0 env.conv ;
//	           memory initial=1 max=4 env.mem ; table funcref initial=2 env.tab ;
//	           global i32 immutable env.g
func TestDecodeImportsFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", "imports.wasm"))
	require.NoError(t, err)
	m := decodeOrSkip(t, data)

	require.NotNil(t, m.Type)
	require.Len(t, m.Type.FunctionTypes, 2)
	requireFuncType(t, m.Type.FunctionTypes[0], []wasm.ValType{wasm.I32, wasm.I64}, []wasm.ValType{wasm.F64})
	requireFuncType(t, m.Type.FunctionTypes[1], []wasm.ValType{wasm.I32}, nil)

	require.Equal(t, []wasm.Import{
		{Module: "env", Name: "log", Desc: wasm.ImportDesc{Kind: wasm.FuncKind, FuncTypeIdx: 1}},
		{Module: "env", Name: "conv", Desc: wasm.ImportDesc{Kind: wasm.FuncKind, FuncTypeIdx: 0}},
		{Module: "env", Name: "mem", Desc: wasm.ImportDesc{Kind: wasm.MemKind, MemLimits: wasm.Limits{HasMax: true, Min: 1, Max: 4}}},
		{Module: "env", Name: "tab", Desc: wasm.ImportDesc{Kind: wasm.TableKind, TableLimits: wasm.Limits{Min: 2}}},
		{Module: "env", Name: "g", Desc: wasm.ImportDesc{Kind: wasm.GlobalKind, GlobalType: wasm.GlobalType{ValType: wasm.I32}}},
	}, importsOf(m))
}

func TestDecodeNoImportSection(t *testing.T) {
	m := decodeOrSkip(t, mod(typeSec()))
	require.Empty(t, importsOf(m))
}

func TestDecodeTableContents(t *testing.T) {
	// Spec 5.3.5: a table type is the element type 0x70 (funcref) followed by
	// limits, and 5.5.6 makes the section a vector of them. A second table is
	// a validation error in 1.0, not a decoding one, so it must decode.
	tests := []struct {
		name string
		body []byte
		want []wasm.Limits
	}{
		{"min only", []byte{0x01, elemFuncref, 0x00, 0x03}, []wasm.Limits{{Min: 3}}},
		{"min and max", []byte{0x01, elemFuncref, 0x01, 0x02, 0x09}, []wasm.Limits{{HasMax: true, Min: 2, Max: 9}}},
		{"min 0", []byte{0x01, elemFuncref, 0x00, 0x00}, []wasm.Limits{{}}},
		{"multi-byte limits", []byte{0x01, elemFuncref, 0x01, 0x80, 0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, []wasm.Limits{{HasMax: true, Min: 128, Max: 4294967295}}},
		{"two tables decode (validator rejects)", []byte{0x02, elemFuncref, 0x00, 0x01, elemFuncref, 0x01, 0x04, 0x05}, []wasm.Limits{{Min: 1}, {HasMax: true, Min: 4, Max: 5}}},
		{"empty section", []byte{0x00}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(sec(secTable, tc.body...)))
			if len(tc.want) == 0 {
				require.True(t, m.Table == nil || len(m.Table.Tables) == 0)
				return
			}
			require.NotNil(t, m.Table)
			require.Equal(t, tc.want, m.Table.Tables)
		})
	}
}

// testdata/wasm/local.wasm: `wasm-objdump -x` says Table[1]: funcref initial=2.
func TestDecodeTableFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", "local.wasm"))
	require.NoError(t, err)
	m := decodeOrSkip(t, data)
	require.NotNil(t, m.Table)
	require.Equal(t, []wasm.Limits{{Min: 2}}, m.Table.Tables)
}

func TestDecodeMemoryContents(t *testing.T) {
	// Spec 5.3.4 / 5.5.7: a memory type is just limits. Limits that break the
	// 2^16 page cap or have min > max are validation errors (3.2.3), not
	// decoding ones, so they must decode.
	tests := []struct {
		name string
		body []byte
		want []wasm.Limits
	}{
		{"min only", []byte{0x01, 0x00, 0x01}, []wasm.Limits{{Min: 1}}},
		{"min and max", []byte{0x01, 0x01, 0x01, 0x02}, []wasm.Limits{{HasMax: true, Min: 1, Max: 2}}},
		{"min 0", []byte{0x01, 0x00, 0x00}, []wasm.Limits{{}}},
		{"65536 pages, three-byte LEB128", []byte{0x01, 0x01, 0x80, 0x80, 0x04, 0x80, 0x80, 0x04}, []wasm.Limits{{HasMax: true, Min: 65536, Max: 65536}}},
		{"over the page cap decodes (validator rejects)", []byte{0x01, 0x00, 0x81, 0x80, 0x04}, []wasm.Limits{{Min: 65537}}},
		{"min above max decodes (validator rejects)", []byte{0x01, 0x01, 0x02, 0x01}, []wasm.Limits{{HasMax: true, Min: 2, Max: 1}}},
		{"two memories decode (validator rejects)", []byte{0x02, 0x00, 0x01, 0x01, 0x02, 0x03}, []wasm.Limits{{Min: 1}, {HasMax: true, Min: 2, Max: 3}}},
		{"empty section", []byte{0x00}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(sec(secMemory, tc.body...)))
			if len(tc.want) == 0 {
				require.True(t, m.Memory == nil || len(m.Memory.Memories) == 0)
				return
			}
			require.NotNil(t, m.Memory)
			require.Equal(t, tc.want, m.Memory.Memories)
		})
	}
}

// `wasm-objdump -x`: memory.wasm has Memory[1] initial=1 max=65536, and
// local.wasm has Memory[1] initial=1 max=2.
func TestDecodeMemoryFixtures(t *testing.T) {
	tests := []struct {
		file string
		want wasm.Limits
	}{
		{"memory.wasm", wasm.Limits{HasMax: true, Min: 1, Max: 65536}},
		{"local.wasm", wasm.Limits{HasMax: true, Min: 1, Max: 2}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", tc.file))
			require.NoError(t, err)
			m := decodeOrSkip(t, data)
			require.NotNil(t, m.Memory)
			require.Equal(t, []wasm.Limits{tc.want}, m.Memory.Memories)
		})
	}
}

// codeSecN is a code section of n empty function bodies (no locals, just end),
// so a function section of n entries is matched by a code section of n.
func codeSecN(n int) []byte {
	body := uleb(uint32(n))
	for range n {
		body = append(body, 0x02, 0x00, opEnd)
	}
	return sec(secCode, body...)
}

func TestDecodeFunctionContents(t *testing.T) {
	// Spec 5.5.4: a vector of type indices. Whether each index names a real
	// type is a validation matter, so out-of-range indices must still decode.
	tests := []struct {
		name string
		body []byte
		want []uint32
	}{
		{"one function", []byte{0x01, 0x00}, []uint32{0}},
		{"indices keep their order", []byte{0x03, 0x00, 0x02, 0x01}, []uint32{0, 2, 1}},
		{"repeated index", []byte{0x03, 0x01, 0x01, 0x01}, []uint32{1, 1, 1}},
		{"multi-byte index (0x80 0x01 = 128)", []byte{0x01, 0x80, 0x01}, []uint32{128}},
		{"index with no matching type decodes", []byte{0x01, 0x7F}, []uint32{127}},
		{"largest index", []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, []uint32{4294967295}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := mod(typeSec(), sec(secFunc, tc.body...), codeSecN(len(tc.want)))
			m := decodeOrSkip(t, in)
			require.NotNil(t, m.Function)
			require.Equal(t, tc.want, m.Function.FuncTypeIdxs)
		})
	}

	t.Run("empty section", func(t *testing.T) {
		m := decodeOrSkip(t, mod(typeSec(), sec(secFunc, 0x00)))
		require.True(t, m.Function == nil || len(m.Function.FuncTypeIdxs) == 0)
	})
}

// `wasm-objdump -x` lists each fixture's function signatures (sig=N is the
// type index) in the Function section.
func TestDecodeFunctionFixtures(t *testing.T) {
	tests := []struct {
		file string
		want []uint32
	}{
		{"add.wasm", []uint32{0}},
		{"fib.wasm", []uint32{0}},
		{"imports.wasm", []uint32{1}}, // only the defined function, not the 2 imported ones
		{"local.wasm", []uint32{0, 0, 2}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", tc.file))
			require.NoError(t, err)
			m := decodeOrSkip(t, data)
			require.NotNil(t, m.Function)
			require.Equal(t, tc.want, m.Function.FuncTypeIdxs)
		})
	}
}

// Initializers are decoded into wasm.Expr: every instruction up to and
// including the terminating end. Immediates are whatever the decoder chose to
// store in Inst.Immediate; the types below (int32, int64, uint32, [4]byte,
// [8]byte for the raw little-endian float bits) are the ones it uses today.
func TestDecodeGlobalContents(t *testing.T) {
	// Spec 5.5.9: vec(globaltype expr), globaltype = valtype mut (0x00 const,
	// 0x01 var).
	tests := []struct {
		name string
		body []byte
		want []wasm.GlobalDefinition
	}{
		{
			name: "immutable i32 initialised with i32.const 7",
			body: []byte{0x01, typeI32, 0x00, 0x41, 0x07, opEnd},
			want: []wasm.GlobalDefinition{{
				Type: wasm.GlobalType{ValType: wasm.I32},
				Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(7)}, {Opcode: wasm.End}}},
			}},
		},
		{
			name: "mutable i64 initialised with i64.const -1",
			body: []byte{0x01, typeI64, 0x01, 0x42, 0x7F, opEnd},
			want: []wasm.GlobalDefinition{{
				Type: wasm.GlobalType{ValType: wasm.I64, Mutable: true},
				Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.I64Const, Immediate: int64(-1)}, {Opcode: wasm.End}}},
			}},
		},
		{
			name: "i32.const with a multi-byte immediate (0x8B 0x00 = 11)",
			body: []byte{0x01, typeI32, 0x00, 0x41, 0x8B, 0x00, opEnd},
			want: []wasm.GlobalDefinition{{
				Type: wasm.GlobalType{ValType: wasm.I32},
				Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(11)}, {Opcode: wasm.End}}},
			}},
		},
		{
			name: "f32.const 1.0 keeps its little-endian bytes",
			body: []byte{0x01, typeF32, 0x00, 0x43, 0x00, 0x00, 0x80, 0x3F, opEnd},
			want: []wasm.GlobalDefinition{{
				Type: wasm.GlobalType{ValType: wasm.F32},
				Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.F32Const, Immediate: [4]byte{0x00, 0x00, 0x80, 0x3F}}, {Opcode: wasm.End}}},
			}},
		},
		{
			name: "f32.const whose payload is all 0x0B",
			body: []byte{0x01, typeF32, 0x00, 0x43, 0x0B, 0x0B, 0x0B, 0x0B, opEnd},
			want: []wasm.GlobalDefinition{{
				Type: wasm.GlobalType{ValType: wasm.F32},
				Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.F32Const, Immediate: [4]byte{0x0B, 0x0B, 0x0B, 0x0B}}, {Opcode: wasm.End}}},
			}},
		},
		{
			name: "f64.const 1.5 keeps its little-endian bytes",
			body: []byte{0x01, typeF64, 0x00, 0x44, 0, 0, 0, 0, 0, 0, 0xF8, 0x3F, opEnd},
			want: []wasm.GlobalDefinition{{
				Type: wasm.GlobalType{ValType: wasm.F64},
				Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.F64Const, Immediate: [8]byte{0, 0, 0, 0, 0, 0, 0xF8, 0x3F}}, {Opcode: wasm.End}}},
			}},
		},
		{
			name: "global.get reads its index",
			body: []byte{0x01, typeI32, 0x00, 0x23, 0x05, opEnd},
			want: []wasm.GlobalDefinition{{
				Type: wasm.GlobalType{ValType: wasm.I32},
				Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.GlobalGet, Immediate: uint32(5)}, {Opcode: wasm.End}}},
			}},
		},
		{
			name: "global.get with a multi-byte index (0x80 0x01 = 128)",
			body: []byte{0x01, typeI32, 0x00, 0x23, 0x80, 0x01, opEnd},
			want: []wasm.GlobalDefinition{{
				Type: wasm.GlobalType{ValType: wasm.I32},
				Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.GlobalGet, Immediate: uint32(128)}, {Opcode: wasm.End}}},
			}},
		},
		{
			name: "several globals keep their order",
			body: []byte{0x03,
				typeI32, 0x00, 0x41, 0x01, opEnd,
				typeI32, 0x01, 0x41, 0x02, opEnd,
				typeI64, 0x00, 0x42, 0x03, opEnd,
			},
			want: []wasm.GlobalDefinition{
				{Type: wasm.GlobalType{ValType: wasm.I32}, Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(1)}, {Opcode: wasm.End}}}},
				{Type: wasm.GlobalType{ValType: wasm.I32, Mutable: true}, Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(2)}, {Opcode: wasm.End}}}},
				{Type: wasm.GlobalType{ValType: wasm.I64}, Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.I64Const, Immediate: int64(3)}, {Opcode: wasm.End}}}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(sec(secGlobal, tc.body...)))
			require.NotNil(t, m.Global)
			require.Equal(t, tc.want, m.Global.Definitions)
		})
	}
}

// `wasm-objdump -x`: local.wasm has global[0] i32 immutable init i32=42 and
// global[1] f64 immutable init f64=1.5 (0x3FF8000000000000).
func TestDecodeGlobalFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", "local.wasm"))
	require.NoError(t, err)
	m := decodeOrSkip(t, data)
	require.NotNil(t, m.Global)
	require.Equal(t, []wasm.GlobalDefinition{
		{
			Type: wasm.GlobalType{ValType: wasm.I32},
			Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(42)}, {Opcode: wasm.End}}},
		},
		{
			Type: wasm.GlobalType{ValType: wasm.F64},
			Init: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.F64Const, Immediate: [8]byte{0, 0, 0, 0, 0, 0, 0xF8, 0x3F}}, {Opcode: wasm.End}}},
		},
	}, m.Global.Definitions)
}

func TestDecodeExportContents(t *testing.T) {
	// Spec 5.5.10: vec(name exportdesc), exportdesc = kind byte then an index
	// (0x00 funcidx, 0x01 tableidx, 0x02 memidx, 0x03 globalidx). Duplicate
	// names and out-of-range indices are validation errors, not decoding ones.
	tests := []struct {
		name string
		body []byte
		want []wasm.Export
	}{
		{"function", []byte{0x01, 0x01, 'a', 0x00, 0x03}, []wasm.Export{{Name: "a", Kind: wasm.FuncKind, Index: 3}}},
		{"table", []byte{0x01, 0x01, 'a', 0x01, 0x00}, []wasm.Export{{Name: "a", Kind: wasm.TableKind}}},
		{"memory", []byte{0x01, 0x01, 'a', 0x02, 0x00}, []wasm.Export{{Name: "a", Kind: wasm.MemKind}}},
		{"global", []byte{0x01, 0x01, 'a', 0x03, 0x07}, []wasm.Export{{Name: "a", Kind: wasm.GlobalKind, Index: 7}}},
		{"multi-byte index (0x80 0x01 = 128)", []byte{0x01, 0x01, 'a', 0x00, 0x80, 0x01}, []wasm.Export{{Name: "a", Kind: wasm.FuncKind, Index: 128}}},
		{"largest index", []byte{0x01, 0x01, 'a', 0x00, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, []wasm.Export{{Name: "a", Kind: wasm.FuncKind, Index: 4294967295}}},
		{"empty name", []byte{0x01, 0x00, 0x00, 0x00}, []wasm.Export{{Name: "", Kind: wasm.FuncKind}}},
		{"multi-byte UTF-8 name", []byte{0x01, 0x03, 0xE2, 0x82, 0xAC, 0x00, 0x00}, []wasm.Export{{Name: "€", Kind: wasm.FuncKind}}},
		{"name containing a NUL byte", []byte{0x01, 0x03, 'a', 0x00, 'b', 0x00, 0x00}, []wasm.Export{{Name: "a\x00b", Kind: wasm.FuncKind}}},
		{"duplicate names decode (validator rejects)", []byte{0x02, 0x01, 'a', 0x00, 0x00, 0x01, 'a', 0x00, 0x01}, []wasm.Export{{Name: "a", Kind: wasm.FuncKind, Index: 0}, {Name: "a", Kind: wasm.FuncKind, Index: 1}}},
		{"same function under two names", []byte{0x02, 0x01, 'a', 0x00, 0x04, 0x01, 'b', 0x00, 0x04}, []wasm.Export{{Name: "a", Kind: wasm.FuncKind, Index: 4}, {Name: "b", Kind: wasm.FuncKind, Index: 4}}},
		{
			name: "all four kinds keep their order",
			body: []byte{0x04, 0x01, 'g', 0x03, 0x02, 0x01, 'm', 0x02, 0x00, 0x01, 't', 0x01, 0x00, 0x01, 'f', 0x00, 0x05},
			want: []wasm.Export{
				{Name: "g", Kind: wasm.GlobalKind, Index: 2},
				{Name: "m", Kind: wasm.MemKind},
				{Name: "t", Kind: wasm.TableKind},
				{Name: "f", Kind: wasm.FuncKind, Index: 5},
			},
		},
		{"empty section", []byte{0x00}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(sec(secExport, tc.body...)))
			if len(tc.want) == 0 {
				require.True(t, m.Export == nil || len(m.Export.Exports) == 0)
				return
			}
			require.NotNil(t, m.Export)
			require.Equal(t, tc.want, m.Export.Exports)
		})
	}
}

// `wasm-objdump -x` for each fixture's Export section. Function indices count
// imports first, so imports.wasm (two imported functions) exports its one
// defined function as index 2.
func TestDecodeExportFixtures(t *testing.T) {
	tests := []struct {
		file string
		want []wasm.Export
	}{
		{"add.wasm", []wasm.Export{{Name: "add", Kind: wasm.FuncKind, Index: 0}}},
		{"fib.wasm", []wasm.Export{{Name: "fib", Kind: wasm.FuncKind, Index: 0}}},
		{"memory.wasm", []wasm.Export{{Name: "memory", Kind: wasm.MemKind, Index: 0}}},
		{"imports.wasm", []wasm.Export{{Name: "go", Kind: wasm.FuncKind, Index: 2}}},
		{"local.wasm", []wasm.Export{
			{Name: "call_it", Kind: wasm.FuncKind, Index: 2},
			{Name: "helper", Kind: wasm.FuncKind, Index: 0},
			{Name: "counter", Kind: wasm.GlobalKind, Index: 0},
			{Name: "memory", Kind: wasm.MemKind, Index: 0},
			{Name: "table", Kind: wasm.TableKind, Index: 0},
			{Name: "euro_€", Kind: wasm.FuncKind, Index: 0},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", tc.file))
			require.NoError(t, err)
			m := decodeOrSkip(t, data)
			require.NotNil(t, m.Export)
			require.Equal(t, tc.want, m.Export.Exports)
		})
	}
}

func opcodesOf(e *wasm.Expr) []wasm.Opcode {
	out := make([]wasm.Opcode, len(e.Instructions))
	for i, in := range e.Instructions {
		out[i] = in.Opcode
	}
	return out
}

func TestDecodeCodeLocals(t *testing.T) {
	// Spec 5.5.13: locals are run-length groups, vec(n:u32 t:valtype). The
	// decoder keeps the groups as written; expanding them is the
	// interpreter's business (a group may declare up to 2^32-1 locals).
	tests := []struct {
		name   string
		locals []byte
		want   []wasm.Local
	}{
		{"no locals", []byte{0x00}, nil},
		{"one group", []byte{0x01, 0x03, typeI32}, []wasm.Local{{Count: 3, Type: wasm.I32}}},
		{"groups stay separate even for the same type", []byte{0x03, 0x01, typeI32, 0x01, typeI32, 0x02, typeF64},
			[]wasm.Local{{Count: 1, Type: wasm.I32}, {Count: 1, Type: wasm.I32}, {Count: 2, Type: wasm.F64}}},
		{"every value type", []byte{0x04, 0x01, typeI32, 0x01, typeI64, 0x01, typeF32, 0x01, typeF64},
			[]wasm.Local{{Count: 1, Type: wasm.I32}, {Count: 1, Type: wasm.I64}, {Count: 1, Type: wasm.F32}, {Count: 1, Type: wasm.F64}}},
		{"group of count 0 is kept", []byte{0x01, 0x00, typeI32}, []wasm.Local{{Count: 0, Type: wasm.I32}}},
		{"multi-byte count (0x80 0x01 = 128)", []byte{0x01, 0x80, 0x01, typeI64}, []wasm.Local{{Count: 128, Type: wasm.I64}}},
		{"one group of 2^32-1 locals is within the limit", []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F, typeI32}, []wasm.Local{{Count: 4294967295, Type: wasm.I32}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(typeSec(), funcSec(), codeSec(tc.locals, opEnd)))
			require.NotNil(t, m.Code)
			require.Len(t, m.Code.Codes, 1)
			got := m.Code.Codes[0].Locals
			require.True(t, slices.Equal(tc.want, got), "want %v, got %v", tc.want, got)
		})
	}
}

func TestDecodeCodeBodies(t *testing.T) {
	// Bodies are decoded into instructions, including the final end and any
	// nested ends. Immediates use the dynamic types the decoder chose.
	inst := func(op wasm.Opcode, imm any) wasm.Inst { return wasm.Inst{Opcode: op, Immediate: imm} }
	tests := []struct {
		name string
		body []byte
		want []wasm.Inst
	}{
		{"empty body", []byte{opEnd}, []wasm.Inst{inst(wasm.End, nil)}},
		{"nop and unreachable", []byte{0x01, 0x00, opEnd}, []wasm.Inst{inst(wasm.Nop, nil), inst(wasm.Unreachable, nil), inst(wasm.End, nil)}},
		{"i32.const then drop", []byte{0x41, 0x07, 0x1A, opEnd}, []wasm.Inst{inst(wasm.I32Const, int32(7)), inst(wasm.Drop, nil), inst(wasm.End, nil)}},
		{"i64.const -1", []byte{0x42, 0x7F, 0x1A, opEnd}, []wasm.Inst{inst(wasm.I64Const, int64(-1)), inst(wasm.Drop, nil), inst(wasm.End, nil)}},
		{"f32.const 1.0 bytes", []byte{0x43, 0x00, 0x00, 0x80, 0x3F, 0x1A, opEnd}, []wasm.Inst{inst(wasm.F32Const, [4]byte{0x00, 0x00, 0x80, 0x3F}), inst(wasm.Drop, nil), inst(wasm.End, nil)}},
		{"f64.const payload all 0x0B", []byte{0x44, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x1A, opEnd}, []wasm.Inst{inst(wasm.F64Const, [8]byte{0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B, 0x0B}), inst(wasm.Drop, nil), inst(wasm.End, nil)}},
		{"local.get, local.set, local.tee", []byte{0x20, 0x03, 0x21, 0x04, 0x22, 0x80, 0x01, opEnd},
			[]wasm.Inst{inst(wasm.LocalGet, uint32(3)), inst(wasm.LocalSet, uint32(4)), inst(wasm.LocalTee, uint32(128)), inst(wasm.End, nil)}},
		{"global.get and global.set", []byte{0x23, 0x01, 0x24, 0x02, opEnd},
			[]wasm.Inst{inst(wasm.GlobalGet, uint32(1)), inst(wasm.GlobalSet, uint32(2)), inst(wasm.End, nil)}},
		{"call", []byte{0x10, 0x02, opEnd}, []wasm.Inst{inst(wasm.Call, uint32(2)), inst(wasm.End, nil)}},
		{"call_indirect keeps the type index, drops the reserved byte", []byte{0x11, 0x01, 0x00, opEnd}, []wasm.Inst{inst(wasm.CallIndirect, uint32(1)), inst(wasm.End, nil)}},
		{"return and select", []byte{0x1B, 0x0F, opEnd}, []wasm.Inst{inst(wasm.Select, nil), inst(wasm.Return, nil), inst(wasm.End, nil)}},
		{"i32.load align 2 offset 128", []byte{0x28, 0x02, 0x80, 0x01, opEnd}, []wasm.Inst{inst(wasm.I32Load, wasm.MemArg{Align: 2, Offset: 128}), inst(wasm.End, nil)}},
		{"i64.store32 align 0 offset 0", []byte{0x3E, 0x00, 0x00, opEnd}, []wasm.Inst{inst(wasm.I64Store32, wasm.MemArg{}), inst(wasm.End, nil)}},
		{"memory.size and memory.grow have no immediate", []byte{0x3F, 0x00, 0x40, 0x00, opEnd}, []wasm.Inst{inst(wasm.MemSize, nil), inst(wasm.MemGrow, nil), inst(wasm.End, nil)}},
		{"br, br_if", []byte{0x0C, 0x01, 0x0D, 0x02, opEnd}, []wasm.Inst{inst(wasm.Br, uint32(1)), inst(wasm.BrIf, uint32(2)), inst(wasm.End, nil)}},
		{"br_table with labels and a default", []byte{0x0E, 0x02, 0x00, 0x01, 0x02, opEnd},
			[]wasm.Inst{inst(wasm.BrTable, wasm.BranchTable{Indices: []uint32{0, 1}, Default: 2}), inst(wasm.End, nil)}},
		{"br_table with only a default", []byte{0x0E, 0x00, 0x05, opEnd},
			[]wasm.Inst{inst(wasm.BrTable, wasm.BranchTable{Indices: []uint32{}, Default: 5}), inst(wasm.End, nil)}},
		{"numeric opcodes (i32.add, f64.reinterpret_i64)", []byte{0x6A, 0xBF, opEnd}, []wasm.Inst{inst(0x6A, nil), inst(0xBF, nil), inst(wasm.End, nil)}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, bodyMod(tc.body...))
			require.Len(t, m.Code.Codes, 1)
			got := m.Code.Codes[0].Body
			require.NotNil(t, got)
			// The immediate of a "no vector" br_table may be nil or empty.
			require.Len(t, got.Instructions, len(tc.want))
			for i, want := range tc.want {
				if bt, ok := want.Immediate.(wasm.BranchTable); ok {
					gotBT, isBT := got.Instructions[i].Immediate.(wasm.BranchTable)
					require.True(t, isBT, "instruction %d immediate is %T", i, got.Instructions[i].Immediate)
					require.True(t, slices.Equal(bt.Indices, gotBT.Indices))
					require.Equal(t, bt.Default, gotBT.Default)
					continue
				}
				require.Equal(t, want, got.Instructions[i], "instruction %d", i)
			}
		})
	}
}

func TestDecodeCodeStructuredControl(t *testing.T) {
	// Nesting is kept flat: block/loop/if, else and every end appear in order.
	// Block types are not asserted here; the decoder may store them however
	// it likes.
	tests := []struct {
		name string
		body []byte
		want []wasm.Opcode
	}{
		{"empty block", []byte{0x02, 0x40, opEnd, opEnd}, []wasm.Opcode{wasm.Block, wasm.End, wasm.End}},
		{"block with a result type", []byte{0x02, typeI32, 0x41, 0x00, opEnd, 0x1A, opEnd}, []wasm.Opcode{wasm.Block, wasm.I32Const, wasm.End, wasm.Drop, wasm.End}},
		{"loop with a branch", []byte{0x03, 0x40, 0x0C, 0x00, opEnd, opEnd}, []wasm.Opcode{wasm.Loop, wasm.Br, wasm.End, wasm.End}},
		{"if without else", []byte{0x41, 0x01, 0x04, 0x40, 0x01, opEnd, opEnd}, []wasm.Opcode{wasm.I32Const, wasm.If, wasm.Nop, wasm.End, wasm.End}},
		{"if with else", []byte{0x41, 0x01, 0x04, 0x40, 0x01, 0x05, 0x01, opEnd, opEnd}, []wasm.Opcode{wasm.I32Const, wasm.If, wasm.Nop, wasm.Else, wasm.Nop, wasm.End, wasm.End}},
		{"nested block, loop, block", []byte{0x02, 0x40, 0x03, 0x40, 0x02, 0x40, opEnd, opEnd, opEnd, opEnd}, []wasm.Opcode{wasm.Block, wasm.Loop, wasm.Block, wasm.End, wasm.End, wasm.End, wasm.End}},
		{"an end after a nested end closes the function, not the block", []byte{0x02, 0x40, opEnd, 0x01, opEnd}, []wasm.Opcode{wasm.Block, wasm.End, wasm.Nop, wasm.End}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, bodyMod(tc.body...))
			require.Len(t, m.Code.Codes, 1)
			require.Equal(t, tc.want, opcodesOf(m.Code.Codes[0].Body))
		})
	}
}

func TestDecodeCodeMultipleFunctions(t *testing.T) {
	// Two functions with different locals and bodies, in order.
	f1 := []byte{0x01, 0x01, typeI32, 0x41, 0x01, 0x1A, opEnd}
	f2 := []byte{0x00, 0x01, opEnd}
	code := sec(secCode, bs([]byte{0x02, byte(len(f1))}, f1, []byte{byte(len(f2))}, f2)...)
	in := mod(typeSec(), sec(secFunc, 0x02, 0x00, 0x00), code)
	m := decodeOrSkip(t, in)
	require.Len(t, m.Code.Codes, 2)
	require.Equal(t, []wasm.Local{{Count: 1, Type: wasm.I32}}, m.Code.Codes[0].Locals)
	require.Equal(t, []wasm.Opcode{wasm.I32Const, wasm.Drop, wasm.End}, opcodesOf(m.Code.Codes[0].Body))
	require.Empty(t, m.Code.Codes[1].Locals)
	require.Equal(t, []wasm.Opcode{wasm.Nop, wasm.End}, opcodesOf(m.Code.Codes[1].Body))
}

// `wasm-objdump -d` for add.wasm: local.get 0; local.get 1; i32.add; end.
func TestDecodeCodeFixtureAdd(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", "add.wasm"))
	require.NoError(t, err)
	m := decodeOrSkip(t, data)
	require.Len(t, m.Code.Codes, 1)
	require.Equal(t, []wasm.Inst{
		{Opcode: wasm.LocalGet, Immediate: uint32(0)},
		{Opcode: wasm.LocalGet, Immediate: uint32(1)},
		{Opcode: 0x6A},
		{Opcode: wasm.End},
	}, m.Code.Codes[0].Body.Instructions)
}

// Every fixture must decode with one code entry per function-section entry,
// and its bodies must end with end.
func TestDecodeCodeFixturesMatchFunctions(t *testing.T) {
	for _, path := range fixturePaths(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			m := decodeOrSkip(t, data)
			var nFuncs, nCode int
			if m.Function != nil {
				nFuncs = len(m.Function.FuncTypeIdxs)
			}
			if m.Code != nil {
				nCode = len(m.Code.Codes)
				for i, c := range m.Code.Codes {
					require.NotNil(t, c.Body, "body %d", i)
					require.NotEmpty(t, c.Body.Instructions, "body %d", i)
					require.Equal(t, wasm.End, c.Body.Instructions[len(c.Body.Instructions)-1].Opcode, "body %d", i)
				}
			}
			require.Equal(t, nFuncs, nCode)
		})
	}
}

func TestDecodeStartContents(t *testing.T) {
	// Spec 5.5.11: the section holds a single funcidx. Index 0 is a real
	// start function and must not be confused with an absent section. Whether
	// the index exists, or has type [] -> [], is checked by the validator.
	tests := []struct {
		name string
		body []byte
		want uint32
	}{
		{"index 0", []byte{0x00}, 0},
		{"index 1", []byte{0x01}, 1},
		{"multi-byte index (0x80 0x01 = 128)", []byte{0x80, 0x01}, 128},
		{"padded index (0x81 0x00 = 1)", []byte{0x81, 0x00}, 1},
		{"largest index", []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, 4294967295},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(sec(secStart, tc.body...)))
			require.NotNil(t, m.Start, "a present start section must be distinguishable from none")
			require.Equal(t, tc.want, m.Start.FuncIdx)
		})
	}

	t.Run("absent section", func(t *testing.T) {
		m := decodeOrSkip(t, mod(typeSec()))
		require.Nil(t, m.Start)
	})
}

// `wasm-objdump -x`: only local.wasm has a start section (function 1); the
// other fixtures have none.
func TestDecodeStartFixtures(t *testing.T) {
	for _, path := range fixturePaths(t) {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			m := decodeOrSkip(t, data)
			if name == "local.wasm" {
				require.NotNil(t, m.Start)
				require.Equal(t, uint32(1), m.Start.FuncIdx)
				return
			}
			require.Nil(t, m.Start)
		})
	}
}

func TestDecodeElementContents(t *testing.T) {
	// Spec 5.5.12: vec(tableidx offset:expr init:vec(funcidx)). Table
	// indices other than 0 and offsets that are not i32 constants are
	// validation errors in 1.0, not decoding ones.
	i32const := func(v int32) []wasm.Inst {
		return []wasm.Inst{{Opcode: wasm.I32Const, Immediate: v}, {Opcode: wasm.End}}
	}
	tests := []struct {
		name string
		body []byte
		want []wasm.Element
	}{
		{
			name: "offset 0 with two functions",
			body: []byte{0x01, 0x00, 0x41, 0x00, opEnd, 0x02, 0x00, 0x01},
			want: []wasm.Element{{TableIdx: 0, Offset: &wasm.Expr{Instructions: i32const(0)}, Init: []uint32{0, 1}}},
		},
		{
			name: "function indices keep their order and may repeat",
			body: []byte{0x01, 0x00, 0x41, 0x00, opEnd, 0x04, 0x03, 0x01, 0x03, 0x00},
			want: []wasm.Element{{Offset: &wasm.Expr{Instructions: i32const(0)}, Init: []uint32{3, 1, 3, 0}}},
		},
		{
			name: "multi-byte function index (0x80 0x01 = 128)",
			body: []byte{0x01, 0x00, 0x41, 0x00, opEnd, 0x01, 0x80, 0x01},
			want: []wasm.Element{{Offset: &wasm.Expr{Instructions: i32const(0)}, Init: []uint32{128}}},
		},
		{
			name: "multi-byte offset (0x80 0x01 = 128)",
			body: []byte{0x01, 0x00, 0x41, 0x80, 0x01, opEnd, 0x01, 0x00},
			want: []wasm.Element{{Offset: &wasm.Expr{Instructions: i32const(128)}, Init: []uint32{0}}},
		},
		{
			name: "offset whose LEB128 byte is 0x0B (an i32.const 11, then end)",
			body: []byte{0x01, 0x00, 0x41, 0x0B, opEnd, 0x01, 0x00},
			want: []wasm.Element{{Offset: &wasm.Expr{Instructions: i32const(11)}, Init: []uint32{0}}},
		},
		{
			name: "offset from global.get",
			body: []byte{0x01, 0x00, 0x23, 0x02, opEnd, 0x01, 0x00},
			want: []wasm.Element{{Offset: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.GlobalGet, Immediate: uint32(2)}, {Opcode: wasm.End}}}, Init: []uint32{0}}},
		},
		{
			name: "empty function list",
			body: []byte{0x01, 0x00, 0x41, 0x05, opEnd, 0x00},
			want: []wasm.Element{{Offset: &wasm.Expr{Instructions: i32const(5)}}},
		},
		{
			name: "table index 1 decodes (validator rejects)",
			body: []byte{0x01, 0x01, 0x41, 0x00, opEnd, 0x01, 0x00},
			want: []wasm.Element{{TableIdx: 1, Offset: &wasm.Expr{Instructions: i32const(0)}, Init: []uint32{0}}},
		},
		{
			name: "several segments keep their order",
			body: []byte{0x02, 0x00, 0x41, 0x00, opEnd, 0x01, 0x07, 0x00, 0x41, 0x04, opEnd, 0x02, 0x08, 0x09},
			want: []wasm.Element{
				{Offset: &wasm.Expr{Instructions: i32const(0)}, Init: []uint32{7}},
				{Offset: &wasm.Expr{Instructions: i32const(4)}, Init: []uint32{8, 9}},
			},
		},
		{"empty section", []byte{0x00}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(sec(secElement, tc.body...)))
			if len(tc.want) == 0 {
				require.True(t, m.Element == nil || len(m.Element.Elements) == 0)
				return
			}
			require.NotNil(t, m.Element)
			require.Len(t, m.Element.Elements, len(tc.want))
			for i, want := range tc.want {
				got := m.Element.Elements[i]
				require.Equal(t, want.TableIdx, got.TableIdx, "segment %d table", i)
				require.Equal(t, want.Offset, got.Offset, "segment %d offset", i)
				require.True(t, slices.Equal(want.Init, got.Init), "segment %d init: want %v, got %v", i, want.Init, got.Init)
			}
		})
	}
}

// `wasm-objdump -x`: local.wasm has one segment, table 0, init i32=0, with
// functions 0 and 1 (bytes 01 00 41 00 0b 02 00 01).
func TestDecodeElementFixture(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", "local.wasm"))
	require.NoError(t, err)
	m := decodeOrSkip(t, data)
	require.NotNil(t, m.Element)
	require.Len(t, m.Element.Elements, 1)
	e := m.Element.Elements[0]
	require.Equal(t, uint32(0), e.TableIdx)
	require.Equal(t, []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(0)}, {Opcode: wasm.End}}, e.Offset.Instructions)
	require.Equal(t, []uint32{0, 1}, e.Init)
}

func TestDecodeDataContents(t *testing.T) {
	// Spec 5.5.14: vec(memidx offset:expr init:vec(byte)). Memory indices
	// other than 0 and offsets that are not i32 constants are validation
	// errors in 1.0, not decoding ones.
	i32const := func(v int32) *wasm.Expr {
		return &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.I32Const, Immediate: v}, {Opcode: wasm.End}}}
	}
	big := make([]byte, 200)
	for i := range big {
		big[i] = byte(i)
	}
	tests := []struct {
		name string
		body []byte
		want []wasm.DataEntry
	}{
		{
			name: "two bytes at offset 0",
			body: []byte{0x01, 0x00, 0x41, 0x00, opEnd, 0x02, 0xAA, 0xBB},
			want: []wasm.DataEntry{{Offset: i32const(0), Init: []byte{0xAA, 0xBB}}},
		},
		{
			name: "empty bytes",
			body: []byte{0x01, 0x00, 0x41, 0x04, opEnd, 0x00},
			want: []wasm.DataEntry{{Offset: i32const(4)}},
		},
		{
			name: "bytes that look like opcodes and end markers are data",
			body: []byte{0x01, 0x00, 0x41, 0x00, opEnd, 0x05, 0x0B, 0x00, 0xFF, 0x41, 0x0B},
			want: []wasm.DataEntry{{Offset: i32const(0), Init: []byte{0x0B, 0x00, 0xFF, 0x41, 0x0B}}},
		},
		{
			name: "multi-byte offset (0x80 0x01 = 128)",
			body: []byte{0x01, 0x00, 0x41, 0x80, 0x01, opEnd, 0x01, 0x07},
			want: []wasm.DataEntry{{Offset: i32const(128), Init: []byte{0x07}}},
		},
		{
			name: "offset from global.get",
			body: []byte{0x01, 0x00, 0x23, 0x01, opEnd, 0x01, 0x07},
			want: []wasm.DataEntry{{Offset: &wasm.Expr{Instructions: []wasm.Inst{{Opcode: wasm.GlobalGet, Immediate: uint32(1)}, {Opcode: wasm.End}}}, Init: []byte{0x07}}},
		},
		{
			name: "memory index 1 decodes (validator rejects)",
			body: []byte{0x01, 0x01, 0x41, 0x00, opEnd, 0x01, 0x07},
			want: []wasm.DataEntry{{MemIdx: 1, Offset: i32const(0), Init: []byte{0x07}}},
		},
		{
			name: "several segments keep their order",
			body: []byte{0x02, 0x00, 0x41, 0x00, opEnd, 0x01, 0x01, 0x00, 0x41, 0x08, opEnd, 0x02, 0x02, 0x03},
			want: []wasm.DataEntry{
				{Offset: i32const(0), Init: []byte{0x01}},
				{Offset: i32const(8), Init: []byte{0x02, 0x03}},
			},
		},
		{
			name: "200 bytes needs a two-byte length (0xC8 0x01)",
			body: bs([]byte{0x01, 0x00, 0x41, 0x00, opEnd, 0xC8, 0x01}, big),
			want: []wasm.DataEntry{{Offset: i32const(0), Init: big}},
		},
		{"empty section", []byte{0x00}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := decodeOrSkip(t, mod(sec(secData, tc.body...)))
			if len(tc.want) == 0 {
				require.True(t, m.Data == nil || len(m.Data.Entries) == 0)
				return
			}
			require.NotNil(t, m.Data)
			require.Len(t, m.Data.Entries, len(tc.want))
			for i, want := range tc.want {
				got := m.Data.Entries[i]
				require.Equal(t, want.MemIdx, got.MemIdx, "segment %d memory", i)
				require.Equal(t, want.Offset, got.Offset, "segment %d offset", i)
				require.True(t, bytes.Equal(want.Init, got.Init), "segment %d init: want % x, got % x", i, want.Init, got.Init)
			}
		})
	}
}

// A decoded segment must own its bytes: scribbling over the input afterwards
// must not change the module (the input may be a reused read buffer).
func TestDecodeDataDoesNotAliasInput(t *testing.T) {
	in := mod(sec(secData, 0x01, 0x00, 0x41, 0x00, opEnd, 0x03, 0x01, 0x02, 0x03))
	m := decodeOrSkip(t, in)
	for i := range in {
		in[i] = 0xEE
	}
	require.NotNil(t, m.Data)
	require.Equal(t, []byte{0x01, 0x02, 0x03}, m.Data.Entries[0].Init)
}

// `wasm-objdump -x`: local.wasm has one segment at offset 8 holding
// "hello\x00\xff wombat" (14 bytes); memory.wasm has an empty segment at
// offset 0 and a one-byte segment (0x01) at offset 65535.
func TestDecodeDataFixtures(t *testing.T) {
	read := func(name string) *wasm.Module {
		data, err := os.ReadFile(filepath.Join("..", "testdata", "wasm", name))
		require.NoError(t, err)
		return decodeOrSkip(t, data)
	}

	local := read("local.wasm")
	require.NotNil(t, local.Data)
	require.Len(t, local.Data.Entries, 1)
	require.Equal(t, uint32(0), local.Data.Entries[0].MemIdx)
	require.Equal(t, []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(8)}, {Opcode: wasm.End}}, local.Data.Entries[0].Offset.Instructions)
	require.Equal(t, []byte("hello\x00\xff wombat"), local.Data.Entries[0].Init)

	mem := read("memory.wasm")
	require.NotNil(t, mem.Data)
	require.Len(t, mem.Data.Entries, 2)
	require.Equal(t, []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(0)}, {Opcode: wasm.End}}, mem.Data.Entries[0].Offset.Instructions)
	require.Empty(t, mem.Data.Entries[0].Init)
	require.Equal(t, []wasm.Inst{{Opcode: wasm.I32Const, Immediate: int32(65535)}, {Opcode: wasm.End}}, mem.Data.Entries[1].Offset.Instructions)
	require.Equal(t, []byte{0x01}, mem.Data.Entries[1].Init)
}
