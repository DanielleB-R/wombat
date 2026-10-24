package decode

import "errors"

func ReadLEB128U32(buffer []byte) (uint32, int, error) {
	var result uint32 = 0
	bytes := 0

	for {
		if bytes >= len(buffer) {
			return 0, 0, errors.New("Incomplete number")
		}
		if bytes >= 5 {
			return 0, 0, errors.New("Number too large")
		}
		if bytes == 4 && buffer[bytes]&0x70 != 0 {
			return 0, 0, errors.New("Unusable bits")
		}
		result |= uint32(buffer[bytes]&0x7f) << (bytes * 7)
		if buffer[bytes]&0x80 == 0 {
			break
		}
		bytes += 1
	}

	return result, bytes + 1, nil
}
