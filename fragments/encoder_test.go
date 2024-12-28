package fragments_test

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"

	"github.com/danderson/dbus/fragments"
)

func TestEncoder(t *testing.T) {
	tests := []struct {
		name string
		in   func(*fragments.Encoder)
		want []byte
	}{
		{
			"raw bytes",
			func(e *fragments.Encoder) {
				e.Write([]byte{1, 2, 3})
			},
			[]byte{0x01, 0x02, 0x03},
		},

		{
			"byte array",
			func(e *fragments.Encoder) {
				e.Bytes([]byte{1, 2, 3})
			},
			[]byte{
				0x00, 0x00, 0x00, 0x03, // length
				0x01, 0x02, 0x03, // val
			},
		},

		{
			"string",
			func(e *fragments.Encoder) {
				e.String("foo")
			},
			[]byte{
				0x00, 0x00, 0x00, 0x03, // length
				0x66, 0x6f, 0x6f, // val
				0x00, // terminator
			},
		},

		{
			"uints",
			func(e *fragments.Encoder) {
				e.Uint8(42)
				e.Uint16(66)
				e.Uint32(42)
				e.Uint64(66)
			},
			[]byte{
				0x2a,
				0x00, // pad
				0x00, 0x42,
				0x00, 0x00, 0x00, 0x2a,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x42,
			},
		},

		{
			"uints padding",
			func(e *fragments.Encoder) {
				e.Uint64(66)
				e.Write([]byte{0})
				e.Uint32(42)
				e.Write([]byte{0})
				e.Uint16(66)
				e.Write([]byte{0})
				e.Uint8(42)
			},
			[]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x42,
				0x00,             // raw
				0x00, 0x00, 0x00, // pad
				0x00, 0x00, 0x00, 0x2a,
				0x00, // raw
				0x00, // pad
				0x00, 0x42,
				0x00, // raw
				0x2a,
			},
		},

		{
			"struct padding",
			func(e *fragments.Encoder) {
				e.Struct()
				e.Uint64(66)
				e.Struct()
				e.Uint32(42)
				e.Struct()
				e.Uint16(66)
				e.Struct()
				e.Uint8(42)
			},
			[]byte{
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x42,
				0x00, 0x00, 0x00, 0x2a,
				0x00, 0x00, 0x00, 0x00, // pad
				0x00, 0x42,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pad
				0x2a,
			},
		},

		{
			"array",
			func(e *fragments.Encoder) {
				e.Array(2, false)
				e.Uint16(1)
				e.Uint16(2)
			},
			[]byte{
				0x00, 0x00, 0x00, 0x02, // length
				0x00, 0x01,
				0x00, 0x02,
			},
		},

		{
			"empty array",
			func(e *fragments.Encoder) {
				e.Array(0, false)
			},
			[]byte{
				0x00, 0x00, 0x00, 0x00, // length
			},
		},

		{
			"struct array",
			func(e *fragments.Encoder) {
				e.Array(2, true)
				e.Struct()
				e.Uint16(1)
				e.Struct()
				e.Uint16(2)
			},
			[]byte{
				0x00, 0x00, 0x00, 0x02, // length
				0x00, 0x00, 0x00, 0x00, // pad
				0x00, 0x01,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // pad
				0x00, 0x02,
			},
		},

		{
			"empty struct array",
			func(e *fragments.Encoder) {
				e.Array(0, true)
			},
			[]byte{
				0x00, 0x00, 0x00, 0x00, // length
				0x00, 0x00, 0x00, 0x00, // pad
			},
		},

		{
			"mapper",
			func(e *fragments.Encoder) {
				e.Mapper = func(t reflect.Type) fragments.EncoderFunc {
					return func(e *fragments.Encoder, v reflect.Value) error {
						e.Write([]byte(v.Type().String()))
						return nil
					}
				}
				e.Value("foo")
				e.Value(uint16(42))
			},
			[]byte{
				0x73, 0x74, 0x72, 0x69, 0x6e, 0x67, // "string"
				0x75, 0x69, 0x6e, 0x74, 0x31, 0x36, // "uint16"
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := fragments.Encoder{
				Order: binary.BigEndian,
			}
			tc.in(&e)
			if got := e.Out; !bytes.Equal(got, tc.want) {
				t.Errorf("incorrect encode:\n  got: % x\n want: % x", got, tc.want)
			} else if testing.Verbose() {
				t.Logf("encoder got: % x", got)
			}
		})
	}
}
