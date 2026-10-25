package wasm

// RawModule contains non-decoded sections of the WASM file
type RawModule struct {
	Sections []RawSection
}

// RawSection contains one non-decoded section of the WASM file
type RawSection struct {
	ID       byte
	Length   uint32
	Contents []byte
}

// Module contains a fully decoded WASM module
type Module struct {
	Type     *TypeSection
	Import   *ImportSection
	Function *FunctionSection
	Table    *TableSection
	Memory   *MemorySection
	Global   *GlobalSection
	Export   *ExportSection
	Start    *StartSection
	Element  *ElementSection
	Code     *CodeSection
	Data     *DataSection
}

// ValType represents the type of a value
type ValType byte

// Constants for the different types in WASM
const (
	I32 ValType = 0x7f
	I64 ValType = 0x7e
	F32 ValType = 0x7d
	F64 ValType = 0x7c
)

// IsValid determines whether the type value matches a known type
func (v ValType) IsValid() bool {
	return v >= F64 && v <= I32
}

// FunctionType represents a functype in the WASM file
type FunctionType struct {
	Parameters []ValType
	Results    []ValType
}

// TypeSection contains the type section data
type TypeSection struct {
	FunctionTypes []FunctionType
}

// ExternKind represents the kind of an import or export
type ExternKind byte

// The possible values for ExternKind
const (
	FuncKind   ExternKind = 0x00
	TableKind  ExternKind = 0x01
	MemKind    ExternKind = 0x02
	GlobalKind ExternKind = 0x03
)

// IsValid returns whether the value matches the spec
func (v ExternKind) IsValid() bool {
	return v <= GlobalKind
}

// GlobalType represents the type of a global
type GlobalType struct {
	ValType ValType
	Mutable bool
}

// ImportDesc represents the inner structure of the import from a Wasm file
type ImportDesc struct {
	Kind        ExternKind
	FuncTypeIdx uint32
	TableLimits Limits
	MemLimits   Limits
	GlobalType  GlobalType
}

// Import represents an import from a WebAssembly file
type Import struct {
	Module string
	Name   string
	Desc   ImportDesc
}

// ImportSection contains the import section data
type ImportSection struct {
	Imports []Import
}

// FunctionSection contains the function section data
type FunctionSection struct {
	FuncTypeIdxs []uint32
}

// Len returns the length of the function section, absent meaning 0
func (f *FunctionSection) Len() int {
	if f == nil {
		return 0
	}
	return len(f.FuncTypeIdxs)
}

// TableSection contains the table section data
type TableSection struct {
	Tables []Limits
}

// MemorySection contains the memory section data
type MemorySection struct {
	Memories []Limits
}

// GlobalDefinition contains the definition of a global, including initializer
type GlobalDefinition struct {
	Type GlobalType
	Init *Expr
}

// GlobalSection contains the global section data
type GlobalSection struct {
	Definitions []GlobalDefinition
}

// Export contains the definition of a single export
type Export struct {
	Name  string
	Kind  ExternKind
	Index uint32
}

// ExportSection contains the export section data
type ExportSection struct {
	Exports []Export
}

// StartSection contains the start section data
type StartSection struct {
	FuncIdx uint32
}

// Element represents a single element
type Element struct {
	TableIdx uint32
	Offset   *Expr
	Init     []uint32
}

// ElementSection contains the element section data
type ElementSection struct {
	Elements []Element
}

// Local contains the definition of one local var
type Local struct {
	Count uint32
	Type  ValType
}

// Function contains one entry from the code section
type Function struct {
	Locals []Local
	Body   *Expr
}

// CodeSection contains the code section data
type CodeSection struct {
	Codes []Function
}

// Len returns the length of the code section, absent meaning 0
func (f *CodeSection) Len() int {
	if f == nil {
		return 0
	}
	return len(f.Codes)
}

// DataEntry contains one entry from the data section
type DataEntry struct {
	MemIdx uint32
	Offset *Expr
	Init   []byte
}

// DataSection contains the data section data
type DataSection struct {
	Entries []DataEntry
}

// Limits represents WASM limits; a minimum with an optional maximum
type Limits struct {
	HasMax bool
	Min    uint32
	Max    uint32
}
