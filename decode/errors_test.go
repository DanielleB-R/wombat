package decode

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// isDecodeError reports whether err carries one of the package's typed errors,
// as opposed to a bare io.EOF or similar leaking from the reader.
func isDecodeError(err error) bool {
	var d Error
	var l LEB128Error
	return errors.As(err, &d) || errors.As(err, &l)
}

// Error() is called by fmt and loggers on whatever value they are handed; it
// must not panic, even for a value outside the declared set.
func TestErrorMessages(t *testing.T) {
	for e := ErrFileTooShort; e <= ErrTooManyLocals; e++ {
		require.NotEmpty(t, e.Error(), "decode.Error(%d)", e)
	}
	for e := ErrIncompleteNumber; e <= ErrUnusableBits; e++ {
		require.NotEmpty(t, e.Error(), "decode.LEB128Error(%d)", e)
	}
	require.NotPanics(t, func() { _ = Error(0xFF).Error() })
	require.NotPanics(t, func() { _ = LEB128Error(0xFF).Error() })
}

// Callers (and the spec harness) tell failures apart with errors.Is, so every
// rejection must surface as a typed error, never a raw io.EOF.
func TestMalformedErrorsAreTyped(t *testing.T) {
	for _, tc := range malformedCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := testDecode(tc.in)
			skipIfNotWired(t, err)
			require.Error(t, err)
			require.True(t, isDecodeError(err), "untyped error: %v", err)
		})
	}
}

func TestTruncationErrorsAreTyped(t *testing.T) {
	for _, path := range fixturePaths(t) {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			boundaries := sectionBoundaries(t, data)
			for n := 0; n < len(data); n++ {
				if boundaries[n] {
					continue
				}
				err := testDecode(data[:n])
				skipIfNotWired(t, err)
				require.Error(t, err, "cut at byte %d", n)
				require.True(t, isDecodeError(err), "cut at byte %d: untyped error %v", n, err)
			}
		})
	}
}

// Each malformation class maps to its own error, so the spec harness can match
// the .wast failure text ("unexpected end", "section size mismatch", ...).
func TestDecodeErrorKinds(t *testing.T) {
	huge := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}
	tests := []struct {
		name string
		in   []byte
		want error
	}{
		{"wrong magic", []byte{0x00, 0x61, 0x73, 0x6E, 0x01, 0x00, 0x00, 0x00}, ErrValueMismatch},
		{"wrong version", []byte{0x00, 0x61, 0x73, 0x6D, 0x02, 0x00, 0x00, 0x00}, ErrValueMismatch},
		{"unknown section id", bs(header, []byte{0x0C, 0x00}), ErrUnknownSectionType},
		{"duplicate section", mod(typeSec(), typeSec()), ErrSectionOutOfOrder},
		{"sections out of order", mod(sec(secMemory, 0x00), sec(secTable, 0x00)), ErrSectionOutOfOrder},
		{"section size runs past the end", bs(header, []byte{secType, 0x05, 0x01}), ErrLengthTooLong},
		{"section larger than its contents", mod(sec(secStart, 0x00, 0x00)), ErrSectionMismatch},
		{"type section with trailing byte", mod(sec(secType, 0x00, 0x00)), ErrSectionMismatch},
		{"code entry larger than its contents", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x03, 0x00, opEnd, opEnd)), ErrSectionMismatch},
		{"function without code", mod(typeSec(), funcSec()), ErrFunctionCodeMismatch},
		{"code without function", mod(typeSec(), codeSec([]byte{0x00}, opEnd)), ErrFunctionCodeMismatch},
		{"vector count past the data", mod(sec(secType, huge...)), ErrLengthTooLong},
		{"name length past the data", mod(sec(secExport, bs([]byte{0x01}, huge)...)), ErrLengthTooLong},
		{"name is not UTF-8", mod(sec(secExport, 0x01, 0x01, 0xFF, 0x00, 0x00)), ErrBadUTF8},
		{"invalid value type", mod(sec(secType, 0x01, typeFunc, 0x01, 0x7B, 0x00)), ErrBadEnum},
		{"invalid export kind", mod(sec(secExport, 0x01, 0x01, 'a', 0x04, 0x00)), ErrBadEnum},
		{"invalid limits flag", mod(sec(secMemory, 0x01, 0x02, 0x00)), ErrBadEnum},
		{"reserved opcode", bodyMod(0x06, opEnd), ErrBadEnum},
		{"table element type is not funcref", mod(sec(secTable, 0x01, 0x6F, 0x00, 0x00)), ErrValueMismatch},
		{"functype marker is not 0x60", mod(sec(secType, 0x01, 0x61, 0x00, 0x00)), ErrValueMismatch},
		{"memory.size reserved byte nonzero", bodyMod(0x3F, 0x01, opEnd), ErrValueMismatch},
		{"else without if", bodyMod(0x05, opEnd), ErrUnexpectedElse},
		{"second else in one if", bodyMod(0x41, 0x00, 0x04, 0x40, 0x05, 0x05, opEnd, opEnd), ErrUnexpectedElse},
		{"too many locals", mod(typeSec(), funcSec(), sec(secCode, 0x01, 0x0A,
			0x02, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F, typeI32, 0x01, typeI32, opEnd)), ErrTooManyLocals},
		{"vector count with unused bits", mod([]byte{secType, 0x05, 0xFF, 0xFF, 0xFF, 0xFF, 0x1F}), ErrUnusableBits},
		{"vector count with six bytes", mod([]byte{secType, 0x07, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00, 0x00}), ErrNumberTooLarge},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := testDecode(tc.in)
			skipIfNotWired(t, err)
			require.ErrorIs(t, err, tc.want)
		})
	}
}

// The LEB128 readers distinguish running out of input, an encoding that is too
// long, and a final byte whose unused bits are wrong (spec 5.2.2).
func TestLEB128ErrorKinds(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want LEB128Error
	}{
		{"empty", nil, ErrIncompleteNumber},
		{"continuation then end", []byte{0x80}, ErrIncompleteNumber},
		{"four continuation bytes then end", []byte{0xFF, 0xFF, 0xFF, 0xFF}, ErrIncompleteNumber},
		{"six bytes", []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, ErrNumberTooLarge},
		{"unused bits in fifth byte", []byte{0x80, 0x80, 0x80, 0x80, 0x10}, ErrUnusableBits},
	}
	for _, tc := range tests {
		t.Run("u32/"+tc.name, func(t *testing.T) {
			_, _, err := testReadU32(tc.in)
			require.ErrorIs(t, err, tc.want)
		})
	}
	t.Run("s32/empty", func(t *testing.T) {
		_, _, err := testReadS32(nil)
		require.ErrorIs(t, err, ErrIncompleteNumber)
	})
	t.Run("s32/six bytes", func(t *testing.T) {
		_, _, err := testReadS32([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00})
		require.ErrorIs(t, err, ErrNumberTooLarge)
	})
	t.Run("s32/sign bit set, unused bits clear", func(t *testing.T) {
		_, _, err := testReadS32([]byte{0x80, 0x80, 0x80, 0x80, 0x08})
		require.ErrorIs(t, err, ErrUnusableBits)
	})
	t.Run("s64/empty", func(t *testing.T) {
		_, _, err := testReadS64(nil)
		require.ErrorIs(t, err, ErrIncompleteNumber)
	})
	t.Run("s64/eleven bytes", func(t *testing.T) {
		_, _, err := testReadS64([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x00})
		require.ErrorIs(t, err, ErrNumberTooLarge)
	})
	t.Run("s64/sign bit set, unused bits clear", func(t *testing.T) {
		_, _, err := testReadS64([]byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01})
		require.ErrorIs(t, err, ErrUnusableBits)
	})
}
