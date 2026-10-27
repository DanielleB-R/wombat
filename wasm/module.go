package wasm

// RawModule contains non-decoded sections of the WASM file
type RawModule struct {
	Sections []RawSection
}

// RawSection contains one non-decoded section of the WASM file
type RawSection struct {
	ID       byte
	Contents []byte
}

// Module contains a fully decoded WASM module
type Module struct {
	Type     []FunctionType
	Import   []Import
	Function []uint32
	Table    []Limits
	Memory   []Limits
	Global   []GlobalDefinition
	Export   []Export
	Start    *uint32
	Element  []Element
	Code     []Function
	Data     []DataEntry
}

// ValType represents the type of a value.
type ValType byte

// Constants for the different types in WASM.
const (
	I32 ValType = 0x7f
	I64 ValType = 0x7e
	F32 ValType = 0x7d
	F64 ValType = 0x7c
)

// IsValid determines whether the type value matches a known type.
func (v ValType) IsValid() bool {
	return v >= F64 && v <= I32
}

// FunctionType represents a functype in the WASM file.
type FunctionType struct {
	Parameters []ValType
	Results    []ValType
}

// ExternKind represents the kind of an import or export.
type ExternKind byte

// The possible values for ExternKind.
const (
	FuncKind   ExternKind = 0x00
	TableKind  ExternKind = 0x01
	MemKind    ExternKind = 0x02
	GlobalKind ExternKind = 0x03
)

// IsValid returns whether the value matches the spec.
func (v ExternKind) IsValid() bool {
	return v <= GlobalKind
}

// GlobalType represents the type of a global.
type GlobalType struct {
	ValType ValType
	Mutable bool
}

// ImportDesc represents the inner structure of the import from a Wasm file.
type ImportDesc struct {
	Kind        ExternKind
	FuncTypeIdx uint32
	TableLimits Limits
	MemLimits   Limits
	GlobalType  GlobalType
}

// Import represents an import from a WebAssembly file.
type Import struct {
	Module string
	Name   string
	Desc   ImportDesc
}

// GlobalDefinition contains the definition of a global, including initializer
type GlobalDefinition struct {
	Type GlobalType
	Init *Expr
}

// Export contains the definition of a single export
type Export struct {
	Name  string
	Kind  ExternKind
	Index uint32
}

// Element represents a single element
type Element struct {
	TableIdx uint32
	Offset   *Expr
	Init     []uint32
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

// DataEntry contains one entry from the data section
type DataEntry struct {
	MemIdx uint32
	Offset *Expr
	Init   []byte
}

// Limits represents WASM limits; a minimum with an optional maximum
type Limits struct {
	HasMax bool
	Min    uint32
	Max    uint32
}
