package decode

import (
	"bytes"
	"fmt"
	"unicode/utf8"

	"github.com/DanielleB-R/wombat/wasm"
)

// Error is an error type for the decoder
type Error byte

// Error values for the decoder
const (
	ErrFileTooShort Error = iota
	ErrValueMismatch
	ErrLengthTooLong
	ErrNotEnoughBytes
	ErrBadUTF8
	ErrBadEnum
	ErrSectionMismatch
	ErrUnexpectedElse
	ErrSectionOutOfOrder
	ErrUnknownSectionType
	ErrFunctionCodeMismatch
	ErrTooManyLocals
)

func (d Error) Error() string {
	switch d {
	case ErrFileTooShort:
		return "file too short"
	case ErrValueMismatch:
		return "fixed byte mismatch"
	case ErrLengthTooLong:
		return "length too long"
	case ErrNotEnoughBytes:
		return "not enough bytes"
	case ErrBadUTF8:
		return "invalid UTF-8 string"
	case ErrBadEnum:
		return "invalid enumerated value"
	case ErrSectionMismatch:
		return "section size mismatch"
	case ErrUnexpectedElse:
		return "invalid else instruction"
	case ErrSectionOutOfOrder:
		return "section out of order"
	case ErrUnknownSectionType:
		return "unknown section type"
	case ErrFunctionCodeMismatch:
		return "function and code length must match"
	case ErrTooManyLocals:
		return "too many locals"
	default:
		return fmt.Sprintf("unknown error %d", d)
	}
}

func expectByte(reader *bytes.Reader, value byte) error {
	found, err := reader.ReadByte()
	if err != nil {
		return ErrFileTooShort
	}
	if found != value {
		return ErrValueMismatch
	}
	return nil
}

func readLength(reader *bytes.Reader) (uint32, error) {
	length, err := readLEB128U32(reader)
	if err != nil {
		return 0, err
	}

	if length > uint32(reader.Len()) {
		return 0, ErrLengthTooLong
	}
	return length, nil
}

func readName(reader *bytes.Reader) (string, error) {
	length, err := readLength(reader)
	if err != nil {
		return "", err
	}

	nameBytes := make([]byte, length)
	n, _ := reader.Read(nameBytes)
	if n != int(length) {
		return "", ErrNotEnoughBytes
	}

	if !utf8.Valid(nameBytes) {
		return "", ErrBadUTF8
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

	minVal, err := readLEB128U32(reader)
	if err != nil {
		return limits, err
	}
	limits.Min = minVal

	if limits.HasMax {
		maxVal, err := readLEB128U32(reader)
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
		return 0, ErrFileTooShort
	}
	valType := wasm.ValType(input)
	if !valType.IsValid() {
		return 0, ErrBadEnum
	}
	return valType, nil
}

func readBool(reader *bytes.Reader) (bool, error) {
	flag, err := reader.ReadByte()
	if err != nil {
		return false, ErrFileTooShort
	}
	if flag > 1 {
		return false, ErrBadEnum
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
		index, err := readLEB128U32(reader)
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
		return 0, ErrFileTooShort
	}
	opcode := wasm.Opcode(input)
	if !opcode.IsValid() {
		return 0, ErrBadEnum
	}
	return opcode, nil
}

func readMemArg(reader *bytes.Reader) (wasm.MemArg, error) {
	var result wasm.MemArg

	align, err := readLEB128U32(reader)
	if err != nil {
		return result, err
	}
	result.Align = align

	offset, err := readLEB128U32(reader)
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
			return nil, ErrFileTooShort
		}
		if blockType != 0x40 && !wasm.ValType(blockType).IsValid() {
			return nil, ErrBadEnum
		}
		return blockType, nil
	case wasm.Br, wasm.BrIf, wasm.Call, wasm.LocalGet, wasm.LocalSet, wasm.LocalTee, wasm.GlobalGet, wasm.GlobalSet:
		// read an index
		return readLEB128U32(reader)
	case wasm.BrTable:
		idx, err := readIndexVector(reader)
		if err != nil {
			return nil, err
		}

		defaultIdx, err := readLEB128U32(reader)
		if err != nil {
			return nil, err
		}

		return wasm.BranchTable{Indices: idx, Default: defaultIdx}, nil
	case wasm.CallIndirect:
		typeIdx, err := readLEB128U32(reader)
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
		return readLEB128I32(reader)
	case wasm.I64Const:
		return readLEB128I64(reader)
	case wasm.F32Const:
		result := [4]byte{0}
		n, _ := reader.Read(result[:])
		if n != 4 {
			return nil, ErrNotEnoughBytes
		}
		return result, nil
	case wasm.F64Const:
		result := [8]byte{0}
		n, _ := reader.Read(result[:])
		if n != 8 {
			return nil, ErrNotEnoughBytes
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
				return nil, ErrUnexpectedElse
			}
		}
	}

	return &wasm.Expr{Instructions: instructions}, nil
}
