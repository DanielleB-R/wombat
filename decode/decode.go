package decode

import (
	"errors"
)

// ReadLEB128U32 decodes an unsigned 32 bit integer in LEB128 format from the front of the buffer
func ReadLEB128U32(buffer []byte) (uint32, int, error) {
	var result uint32
	bytes := 0

	for {
		if bytes >= len(buffer) {
			return 0, 0, errors.New("incomplete number")
		}
		if bytes >= 5 {
			return 0, 0, errors.New("number too large")
		}
		if bytes == 4 && buffer[bytes]&0x70 != 0 {
			return 0, 0, errors.New("unusable bits")
		}
		result |= uint32(buffer[bytes]&0x7f) << (bytes * 7)
		if buffer[bytes]&0x80 == 0 {
			break
		}
		bytes++
	}

	return result, bytes + 1, nil
}

// ReadLEB128I32 decodes a signed 32 bit integer in LEB128 format from the front of the buffer
func ReadLEB128I32(buffer []byte) (int32, int, error) {
	var result int32
	bytes := 0

	for {
		if bytes >= len(buffer) {
			return 0, 0, errors.New("incomplete number")
		}
		if bytes >= 5 {
			return 0, 0, errors.New("number too large")
		}
		result |= int32(buffer[bytes]&0x7f) << (bytes * 7)
		if buffer[bytes]&0x80 == 0 {
			break
		}
		bytes++
	}

	if buffer[bytes]&0x40 != 0 || (bytes == 4 && buffer[bytes]&0x08 != 0) {
		result |= (^0) << ((bytes + 1) * 7)
	}
	if bytes == 4 && buffer[bytes]&0x70 != (buffer[bytes]&0x08>>3)*0x70 {
		return 0, 0, errors.New("unusable bits")
	}

	return result, bytes + 1, nil
}
