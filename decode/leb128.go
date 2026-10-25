package decode

import (
	"bytes"
	"fmt"
)

// LEB128Error is the error type for the LEB128 code
type LEB128Error byte

// These are the possible error values
const (
	ErrIncompleteNumber LEB128Error = iota
	ErrNumberTooLarge
	ErrUnusableBits
)

func (l LEB128Error) Error() string {
	switch l {
	case ErrIncompleteNumber:
		return "incomplete number"
	case ErrNumberTooLarge:
		return "number too large"
	case ErrUnusableBits:
		return "unusable bits"
	default:
		return fmt.Sprintf("unknown error %d", l)
	}
}

// ReadLEB128U32 decodes an unsigned 32 bit integer in LEB128 format from the front of the reader
func ReadLEB128U32(reader *bytes.Reader) (uint32, error) {
	var result uint32
	n := 0

	for {
		if n >= 5 {
			return 0, ErrNumberTooLarge
		}
		thisByte, err := reader.ReadByte()
		if err != nil { // ReadByte only returns EOF
			return 0, ErrIncompleteNumber
		}
		if n == 4 && thisByte&0x70 != 0 {
			return 0, ErrUnusableBits
		}
		result |= uint32(thisByte&0x7f) << (n * 7)
		if thisByte&0x80 == 0 {
			break
		}
		n++
	}

	return result, nil
}

// ReadLEB128I32 decodes a signed 32 bit integer in LEB128 format from the front of the reader
func ReadLEB128I32(reader *bytes.Reader) (int32, error) {
	var result int32
	n := 0
	var thisByte byte
	var err error

	for {
		if n >= 5 {
			return 0, ErrNumberTooLarge
		}
		thisByte, err = reader.ReadByte()
		if err != nil { // ReadByte only returns EOF
			return 0, ErrIncompleteNumber
		}

		result |= int32(thisByte&0x7f) << (n * 7)
		if thisByte&0x80 == 0 {
			break
		}
		n++
	}

	if thisByte&0x40 != 0 || (n == 4 && thisByte&0x08 != 0) {
		result |= (^0) << ((n + 1) * 7)
	}
	if n == 4 && thisByte&0x70 != (thisByte&0x08>>3)*0x70 {
		return 0, ErrUnusableBits
	}

	return result, nil
}

// ReadLEB128I64 decodes a signed 64 bit integer in LEB128 format from the front of the reader
func ReadLEB128I64(reader *bytes.Reader) (int64, error) {
	var result int64
	n := 0
	var thisByte byte
	var err error

	for {
		if n >= 10 {
			return 0, ErrNumberTooLarge
		}
		thisByte, err = reader.ReadByte()
		if err != nil { // ReadByte only returns EOF
			return 0, ErrIncompleteNumber
		}

		result |= int64(thisByte&0x7f) << (n * 7)
		if thisByte&0x80 == 0 {
			break
		}
		n++
	}

	if thisByte&0x40 != 0 || (n == 9 && thisByte&0x08 != 0) {
		result |= (^0) << ((n + 1) * 7)
	}
	if n == 9 && thisByte&0x7e != (thisByte&0x01)*0x7e {
		return 0, ErrUnusableBits
	}

	return result, nil
}
