package decode

import (
	"bytes"
	"errors"
	"io"
	"unicode/utf8"

	"github.com/DanielleB-R/wombat/wasm"
)

// DecodeError is an error type for the decoder
type DecodeError byte

const (
	FileTooShort DecodeError = iota
	ValueMismatch
	LengthTooLong
	NotEnoughBytes
	BadUTF8
	BadEnum
	UnderfullSection
)

func (d DecodeError) Error() string {
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
