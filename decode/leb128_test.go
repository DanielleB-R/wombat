package decode

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// Cases come from spec section 5.2.2 (Integers): an encoding of uN/sN may be
// at most ceil(N/7) bytes, trailing padding is allowed inside that bound, and
// in the final byte the unused bits must be 0 for non-negative values and 1
// for negative ones.

func TestReadU32(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    uint32
		wantLen int
		wantErr bool
	}{
		{name: "zero", in: []byte{0x00}, want: 0, wantLen: 1},
		{name: "three", in: []byte{0x03}, want: 3, wantLen: 1},
		{name: "three padded", in: []byte{0x83, 0x00}, want: 3, wantLen: 2},
		{name: "largest one byte", in: []byte{0x7F}, want: 127, wantLen: 1},
		{name: "smallest two byte", in: []byte{0x80, 0x01}, want: 128, wantLen: 2},
		{name: "wikipedia example", in: []byte{0xE5, 0x8E, 0x26}, want: 624485, wantLen: 3},
		{name: "max uint32", in: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, want: math.MaxUint32, wantLen: 5},
		{name: "zero padded to five bytes", in: []byte{0x80, 0x80, 0x80, 0x80, 0x00}, want: 0, wantLen: 5},
		{name: "stops at first final byte", in: []byte{0x05, 0xFF, 0xFF}, want: 5, wantLen: 1},

		{name: "empty input", in: nil, wantErr: true},
		{name: "truncated after continuation", in: []byte{0x80}, wantErr: true},
		{name: "truncated mid-value", in: []byte{0xFF, 0xFF, 0xFF, 0xFF}, wantErr: true},
		{name: "unused bits set in 5th byte", in: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x1F}, wantErr: true},
		{name: "lowest unused bit set in 5th byte", in: []byte{0x80, 0x80, 0x80, 0x80, 0x10}, wantErr: true},
		{name: "six bytes, zero padded", in: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, wantErr: true},
		{name: "six bytes, continuation on fifth", in: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, n, err := testReadU32(tc.in)
			skipIfNotWired(t, err)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantLen, n, "bytes consumed")
		})
	}
}

func TestReadS32(t *testing.T) {
	tests := []struct {
		name    string
		in      []byte
		want    int32
		wantLen int
		wantErr bool
	}{
		{name: "zero", in: []byte{0x00}, want: 0, wantLen: 1},
		{name: "two", in: []byte{0x02}, want: 2, wantLen: 1},
		{name: "minus two", in: []byte{0x7E}, want: -2, wantLen: 1},
		{name: "minus two, two bytes", in: []byte{0xFE, 0x7F}, want: -2, wantLen: 2},
		{name: "minus two, three bytes", in: []byte{0xFE, 0xFF, 0x7F}, want: -2, wantLen: 3},
		{name: "largest one byte", in: []byte{0x3F}, want: 63, wantLen: 1},
		{name: "smallest one byte", in: []byte{0x40}, want: -64, wantLen: 1},
		{name: "64 needs a sign-clearing byte", in: []byte{0xC0, 0x00}, want: 64, wantLen: 2},
		{name: "minus 65", in: []byte{0xBF, 0x7F}, want: -65, wantLen: 2},
		{name: "max int32", in: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x07}, want: math.MaxInt32, wantLen: 5},
		{name: "min int32", in: []byte{0x80, 0x80, 0x80, 0x80, 0x78}, want: math.MinInt32, wantLen: 5},
		{name: "minus one, five bytes", in: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x7F}, want: -1, wantLen: 5},
		{name: "stops at first final byte", in: []byte{0x7F, 0xFF}, want: -1, wantLen: 1},

		{name: "empty input", in: nil, wantErr: true},
		{name: "truncated", in: []byte{0xFF, 0xFF}, wantErr: true},
		// The 5th byte carries value bits 28-31 in its low four bits (bit 3 is
		// the sign bit) and three unused bits above them (bits 4-6) that must
		// all equal the sign bit. So only 0x00-0x07 and 0x78-0x7F are valid.
		{name: "5th byte 0x08: sign bit set, unused bits clear (2^31 overflows)", in: []byte{0x80, 0x80, 0x80, 0x80, 0x08}, wantErr: true},
		{name: "5th byte 0x0F: sign bit set, unused bits clear (u32 max is not an s32)", in: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x0F}, wantErr: true},
		{name: "5th byte 0x17: sign bit clear, unused bit 4 set", in: []byte{0x80, 0x80, 0x80, 0x80, 0x17}, wantErr: true},
		{name: "5th byte 0x40: sign bit clear, unused bits mixed (100)", in: []byte{0x80, 0x80, 0x80, 0x80, 0x40}, wantErr: true},
		{name: "5th byte 0x68: sign bit set, unused bits mixed (110)", in: []byte{0x80, 0x80, 0x80, 0x80, 0x68}, wantErr: true},
		{name: "5th byte 0x70: sign bit clear, unused bits all set", in: []byte{0x80, 0x80, 0x80, 0x80, 0x70}, wantErr: true},
		{name: "six bytes", in: []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x00}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, n, err := testReadS32(tc.in)
			skipIfNotWired(t, err)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantLen, n, "bytes consumed")
		})
	}
}

func TestReadS64(t *testing.T) {
	ff9 := []byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}
	z9 := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80}
	with := func(prefix []byte, tail ...byte) []byte {
		return append(append([]byte{}, prefix...), tail...)
	}
	tests := []struct {
		name    string
		in      []byte
		want    int64
		wantLen int
		wantErr bool
	}{
		{name: "zero", in: []byte{0x00}, want: 0, wantLen: 1},
		{name: "minus one", in: []byte{0x7F}, want: -1, wantLen: 1},
		{name: "minus 64", in: []byte{0x40}, want: -64, wantLen: 1},
		{name: "64", in: []byte{0xC0, 0x00}, want: 64, wantLen: 2},
		{name: "max int32 as s64", in: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x07}, want: math.MaxInt32, wantLen: 5},
		{name: "max int32 plus one", in: []byte{0x80, 0x80, 0x80, 0x80, 0x08}, want: math.MaxInt32 + 1, wantLen: 5},
		{name: "max int64", in: with(ff9, 0x00), want: math.MaxInt64, wantLen: 10},
		{name: "min int64", in: with(z9, 0x7F), want: math.MinInt64, wantLen: 10},
		{name: "minus one, ten bytes", in: with(ff9, 0x7F), want: -1, wantLen: 10},

		{name: "empty input", in: nil, wantErr: true},
		{name: "truncated at nine bytes", in: ff9, wantErr: true},
		// The 10th byte carries only value bit 63 (its bit 0, the sign bit);
		// bits 1-6 are unused and must equal it. Only 0x00 and 0x7F are valid.
		{name: "10th byte 0x01: sign bit set, unused bits clear", in: with(z9, 0x01), wantErr: true},
		{name: "10th byte 0x02: sign bit clear, unused bit 1 set", in: with(z9, 0x02), wantErr: true},
		{name: "10th byte 0x40: sign bit clear, unused bits mixed", in: with(z9, 0x40), wantErr: true},
		{name: "10th byte 0x7E: sign bit clear, unused bits all set", in: with(z9, 0x7E), wantErr: true},
		{name: "eleven bytes", in: with(ff9, 0xFF, 0x00), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, n, err := testReadS64(tc.in)
			skipIfNotWired(t, err)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantLen, n, "bytes consumed")
		})
	}
}
