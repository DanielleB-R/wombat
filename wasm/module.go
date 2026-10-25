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

type ExternKind byte

const (
	FuncKind   ExternKind = 0x00
	TableKind  ExternKind = 0x01
	MemKind    ExternKind = 0x02
	GlobalKind ExternKind = 0x03
)

func (v ExternKind) IsValid() bool {
	return v <= GlobalKind
}

type GlobalType struct {
	ValType ValType
	Mutable bool
}

type ImportDesc struct {
	Kind        ExternKind
	FuncIdx     uint32
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

type FunctionSection struct{}

type TableSection struct{}

type MemorySection struct{}

type GlobalSection struct{}

type ExportSection struct{}

type StartSection struct{}

type ElementSection struct{}

type CodeSection struct{}

type DataSection struct{}

type Limits struct {
	HasMax bool
	Min    uint32
	Max    uint32
}
