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

const (
	I32 ValType = 0x7f
	I64 ValType = 0x7e
	F32 ValType = 0x7d
	F64 ValType = 0x7c
)

func (v ValType) IsValid() bool {
	return v >= F64 && v <= I32
}

type FunctionType struct {
	Parameters []ValType
	Results    []ValType
}

type TypeSection struct {
	FunctionTypes []FunctionType
}

type ImportSection struct{}

type FunctionSection struct{}

type TableSection struct{}

type MemorySection struct{}

type GlobalSection struct{}

type ExportSection struct{}

type StartSection struct{}

type ElementSection struct{}

type CodeSection struct{}

type DataSection struct{}
