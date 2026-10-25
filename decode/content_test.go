package decode

import (
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
