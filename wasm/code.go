package wasm

type Expr struct {
	Instructions []Inst
}

type Inst struct {
	Opcode    Opcode
	Immediate any
}

type Opcode byte

const (
	Unreachable Opcode = 0x00
	Nop         Opcode = 0x01
	Block       Opcode = 0x02
	Loop        Opcode = 0x03
	If          Opcode = 0x04
	Else        Opcode = 0x05

	End          Opcode = 0x0b
	Br           Opcode = 0x0c
	BrIf         Opcode = 0x0d
	BrTable      Opcode = 0x0e
	Return       Opcode = 0x0f
	Call         Opcode = 0x10
	CallIndirect Opcode = 0x11

	Drop   Opcode = 0x1a
	Select Opcode = 0x1b

	LocalGet  Opcode = 0x20
	LocalSet  Opcode = 0x21
	LocalTee  Opcode = 0x22
	GlobalGet Opcode = 0x23
	GlobalSet Opcode = 0x24

	I32Load    Opcode = 0x28
	I64Load    Opcode = 0x29
	F32Load    Opcode = 0x2a
	F64Load    Opcode = 0x2b
	I32Load8S  Opcode = 0x2c
	I32Load8U  Opcode = 0x2d
	I32Load16S Opcode = 0x2e
	I32Load16U Opcode = 0x2f
	I64Load8S  Opcode = 0x30
	I64Load8U  Opcode = 0x31
	I64Load16S Opcode = 0x32
	I64Load16U Opcode = 0x33
	I64Load32S Opcode = 0x34
	I64Load32U Opcode = 0x35
	I32Store   Opcode = 0x36
	I64Store   Opcode = 0x37
	F32Store   Opcode = 0x38
	F64Store   Opcode = 0x39
	I32Store8  Opcode = 0x3a
	I32Store16 Opcode = 0x3b
	I64Store8  Opcode = 0x3c
	I64Store16 Opcode = 0x3d
	I64Store32 Opcode = 0x3e
	MemSize    Opcode = 0x3f
	MemGrow    Opcode = 0x40
	I32Const   Opcode = 0x41
	I64Const   Opcode = 0x42
	F32Const   Opcode = 0x43
	F64Const   Opcode = 0x44
)

func (o Opcode) IsValid() bool {
	invalid := (o > Else && o < End) || (o > CallIndirect && o < Drop) || (o > Select && o < LocalGet) || (o > GlobalSet && o < I32Load) || (o > 0xbf)

	return !invalid
}

type MemArg struct {
	Align  uint32
	Offset uint32
}

type BranchTable struct {
	Indices []uint32
	Default uint32
}
