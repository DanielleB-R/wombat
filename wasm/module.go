package wasm

type RawModule struct {
	Sections []RawSection
}

type RawSection struct {
	Id       byte
	Length   uint32
	Contents []byte
}

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
