package decode

import (
	"bytes"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/DanielleB-R/wombat/wasm"
)

// Error is an error type for the decoder
type Error byte

// Error values for the decoder
const (
	FileTooShort Error = iota
	ValueMismatch
	LengthTooLong
	NotEnoughBytes
	BadUTF8
	BadEnum
	UnderfullSection
	UnexpectedElse
)

func (d Error) Error() string {
	switch d {
	case FileTooShort:
		return "file too short"
	case ValueMismatch:
		return "fixed byte mismatch"
	case LengthTooLong:
		return "length too long"
	case NotEnoughBytes:
		return "not enough bytes"
	case BadUTF8:
		return "invalid UTF-8 string"
	case BadEnum:
		return "invalid enumerated value"
	case UnderfullSection:
		return "contents didn't fill section"
	case UnexpectedElse:
		return "invalid else instruction"
	default:
		panic("invalid value")
	}
}

func expectByte(reader *bytes.Reader, value byte) error {
	found, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if found != value {
		return ValueMismatch
	}
	return nil
}

func readLength(reader *bytes.Reader) (uint32, error) {
	length, err := ReadLEB128U32(reader)
	if err != nil {
		return 0, err
	}

	if length > uint32(reader.Len()) {
		return 0, LengthTooLong
	}
	return length, nil
}

func readName(reader *bytes.Reader) (string, error) {
	length, err := readLength(reader)
	if err != nil {
		return "", err
	}

	nameBytes := make([]byte, length)
	n, err := reader.Read(nameBytes)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if n != int(length) {
		return "", NotEnoughBytes
	}

	if !utf8.Valid(nameBytes) {
		return "", BadUTF8
	}

	return string(nameBytes), nil
}

func readLimits(reader *bytes.Reader) (wasm.Limits, error) {
	var limits wasm.Limits

	flag, err := readBool(reader)
	if err != nil {
		return limits, err
	}
	limits.HasMax = flag

	minVal, err := ReadLEB128U32(reader)
	if err != nil {
		return limits, err
	}
	limits.Min = minVal

	if limits.HasMax {
		maxVal, err := ReadLEB128U32(reader)
		if err != nil {
			return limits, err
		}
		limits.Max = maxVal
	}

	return limits, nil
}

func readValueType(reader *bytes.Reader) (wasm.ValType, error) {
	input, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	valType := wasm.ValType(input)
	if !valType.IsValid() {
		return 0, BadEnum
	}
	return valType, nil
}

func readBool(reader *bytes.Reader) (bool, error) {
	flag, err := reader.ReadByte()
	if err != nil {
		return false, err
	}
	if flag > 1 {
		return false, BadEnum
	}
	return flag == 1, nil
}

func readTableType(reader *bytes.Reader) (wasm.Limits, error) {
	err := expectByte(reader, 0x70)
	if err != nil {
		return wasm.Limits{}, err
	}
	return readLimits(reader)
}

func readIndexVector(reader *bytes.Reader) ([]uint32, error) {
	length, err := readLength(reader)
	if err != nil {
		return nil, err
	}

	indexes := make([]uint32, length)

	for i := range length {
		index, err := ReadLEB128U32(reader)
		if err != nil {
			return nil, err
		}
		indexes[i] = index
	}

	return indexes, nil
}

func readGlobalType(reader *bytes.Reader) (wasm.GlobalType, error) {
	var result wasm.GlobalType

	valType, err := readValueType(reader)
	if err != nil {
		return result, err
	}
	result.ValType = valType
	mutable, err := readBool(reader)
	if err != nil {
		return result, err
	}
	result.Mutable = mutable

	return result, nil
}

func readInstruction(reader *bytes.Reader) (wasm.Inst, error) {
	var result wasm.Inst

	opcode, err := readOpcode(reader)
	if err != nil {
		return result, err
	}
	result.Opcode = opcode

	immediate, err := readImmediate(reader, opcode)
	if err != nil {
		return result, err
	}
	result.Immediate = immediate

	return result, nil
}

func readOpcode(reader *bytes.Reader) (wasm.Opcode, error) {
	input, err := reader.ReadByte()
	if err != nil {
		return 0, err
	}
	opcode := wasm.Opcode(input)
	if !opcode.IsValid() {
		return 0, BadEnum
	}
	return opcode, nil
}

func readMemArg(reader *bytes.Reader) (wasm.MemArg, error) {
	var result wasm.MemArg

	align, err := ReadLEB128U32(reader)
	if err != nil {
		return result, err
	}
	result.Align = align

	offset, err := ReadLEB128U32(reader)
	if err != nil {
		return result, err
	}
	result.Offset = offset

	return result, nil
}

func readImmediate(reader *bytes.Reader, opcode wasm.Opcode) (any, error) {
	// we assume the opcode is valid
	//nolint:exhaustive // opcodes without an immediate share the default return
	switch opcode {
	case wasm.Block, wasm.Loop, wasm.If:
		blockType, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if blockType != 0x40 && !wasm.ValType(blockType).IsValid() {
			return nil, BadEnum
		}
		return blockType, nil
	case wasm.Br, wasm.BrIf, wasm.Call, wasm.LocalGet, wasm.LocalSet, wasm.LocalTee, wasm.GlobalGet, wasm.GlobalSet:
		// read an index
		return ReadLEB128U32(reader)
	case wasm.BrTable:
		idx, err := readIndexVector(reader)
		if err != nil {
			return nil, err
		}

		defaultIdx, err := ReadLEB128U32(reader)
		if err != nil {
			return nil, err
		}

		return wasm.BranchTable{Indices: idx, Default: defaultIdx}, nil
	case wasm.CallIndirect:
		typeIdx, err := ReadLEB128U32(reader)
		if err != nil {
			return nil, err
		}
		err = expectByte(reader, 0x00)
		if err != nil {
			return nil, err
		}
		return typeIdx, nil
	case wasm.I32Load, wasm.I64Load, wasm.F32Load, wasm.F64Load, wasm.I32Load8S, wasm.I32Load8U, wasm.I32Load16S, wasm.I32Load16U, wasm.I64Load8S, wasm.I64Load8U, wasm.I64Load16S, wasm.I64Load16U, wasm.I64Load32S, wasm.I64Load32U, wasm.I32Store, wasm.I64Store, wasm.F32Store, wasm.F64Store, wasm.I32Store8, wasm.I32Store16, wasm.I64Store8, wasm.I64Store16, wasm.I64Store32:
		return readMemArg(reader)
	case wasm.MemSize, wasm.MemGrow:
		err := expectByte(reader, 0x00)
		if err != nil {
			return nil, err
		}
		return nil, nil
	case wasm.I32Const:
		return ReadLEB128I32(reader)
	case wasm.I64Const:
		return ReadLEB128I64(reader)
	case wasm.F32Const:
		result := [4]byte{0}
		n, err := reader.Read(result[:])
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if n != 4 {
			return nil, NotEnoughBytes
		}
		return result, nil
	case wasm.F64Const:
		result := [8]byte{0}
		n, err := reader.Read(result[:])
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
		if n != 8 {
			return nil, NotEnoughBytes
		}
		return result, nil
	}

	return nil, nil
}

func readExpr(reader *bytes.Reader) (*wasm.Expr, error) {
	var instructions []wasm.Inst

	// entry is true if and only if an else inst would be valid
	var blockStack []bool

	for {
		inst, err := readInstruction(reader)
		if err != nil {
			return nil, err
		}
		instructions = append(instructions, inst)

		if inst.Opcode == wasm.End {
			if len(blockStack) == 0 {
				break
			}

			blockStack = blockStack[:len(blockStack)-1]
		}
		if inst.Opcode == wasm.Block || inst.Opcode == wasm.Loop {
			blockStack = append(blockStack, false)
		}
		if inst.Opcode == wasm.If {
			blockStack = append(blockStack, true)
		}
		if inst.Opcode == wasm.Else {
			if len(blockStack) != 0 && blockStack[len(blockStack)-1] {
				blockStack[len(blockStack)-1] = false
			} else {
				return nil, UnexpectedElse
			}
		}
	}

	return &wasm.Expr{Instructions: instructions}, nil
}
