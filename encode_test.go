package dbus_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/danderson/dbus"
)

type SelfMarshalerVal struct {
	B byte
}

func (s SelfMarshalerVal) MarshalDBus(bs []byte, ord binary.AppendByteOrder) ([]byte, error) {
	return ord.AppendUint16(bs, uint16(s.B)+1), nil
}

func (s SelfMarshalerVal) AlignDBus() int { return 3 }

func (s SelfMarshalerVal) SignatureDBus() dbus.Signature { return "q" }

type SelfMarshalerPtr struct {
	b byte
}

func (s *SelfMarshalerPtr) MarshalDBus(bs []byte, ord binary.AppendByteOrder) ([]byte, error) {
	return ord.AppendUint16(bs, uint16(s.b)+1), nil
}

func (s *SelfMarshalerPtr) AlignDBus() int { return 3 }

func (s *SelfMarshalerPtr) SignatureDBus() dbus.Signature { return "q" }

func TestTypeEncoder(t *testing.T) {
	type Simple struct {
		A int16
		B bool
	}
	type Nested struct {
		A byte
		B Simple
	}
	type Embedded struct {
		Simple
		C byte
	}
	type EmbeddedShadow struct {
		Simple
		B byte
	}
	type NestedSelfMarshalerPtr struct {
		A byte
		B SelfMarshalerPtr
	}

	var be, le = binary.BigEndian, binary.LittleEndian
	encName := map[binary.AppendByteOrder]string{
		be: "BE",
		le: "LE",
	}

	tests := []struct {
		in   any
		enc  binary.AppendByteOrder
		want []byte // empty means want error
	}{
		{byte(5), le, []byte{0x05}},
		{byte(5), be, []byte{0x05}},
		{true, le, []byte{0x01, 0x00, 0x00, 0x00}},
		{true, be, []byte{0x00, 0x00, 0x00, 0x01}},
		{false, le, []byte{0x00, 0x00, 0x00, 0x00}},
		{false, be, []byte{0x00, 0x00, 0x00, 0x00}},
		{int8(0x2b), le, []byte{0x2b}},
		{int8(0x2b), be, []byte{0x2b}},
		{int16(0x2bff), le, []byte{0xff, 0x2b}},
		{int16(0x2bff), be, []byte{0x2b, 0xff}},
		{uint16(0x2bff), le, []byte{0xff, 0x2b}},
		{uint16(0x2bff), be, []byte{0x2b, 0xff}},
		{int32(0x12342bff), le, []byte{0xff, 0x2b, 0x34, 0x12}},
		{int32(0x12342bff), be, []byte{0x12, 0x34, 0x2b, 0xff}},
		{uint32(0x12342bff), le, []byte{0xff, 0x2b, 0x34, 0x12}},
		{uint32(0x12342bff), be, []byte{0x12, 0x34, 0x2b, 0xff}},
		{int64(0x1abbccdd12342bff), le, []byte{
			0xff, 0x2b, 0x34, 0x12,
			0xdd, 0xcc, 0xbb, 0x1a,
		}},
		{int64(0x1abbccdd12342bff), be, []byte{
			0x1a, 0xbb, 0xcc, 0xdd,
			0x12, 0x34, 0x2b, 0xff,
		}},
		{uint64(0xaabbccdd12342bff), le, []byte{
			0xff, 0x2b, 0x34, 0x12,
			0xdd, 0xcc, 0xbb, 0xaa,
		}},
		{uint64(0xaabbccdd12342bff), be, []byte{
			0xaa, 0xbb, 0xcc, 0xdd,
			0x12, 0x34, 0x2b, 0xff,
		}},
		{float32(3402823700), le, []byte{
			0x00, 0x00, 0x00, 0x00,
			0x5F, 0x5A, 0xE9, 0x41,
		}},
		{float32(3402823700), be, []byte{
			0x41, 0xE9, 0x5A, 0x5F,
			0x00, 0x00, 0x00, 0x00,
		}},
		{float64(3402823700), le, []byte{
			0x00, 0x00, 0x80, 0x02,
			0x5F, 0x5A, 0xE9, 0x41,
		}},
		{float64(3402823700), be, []byte{
			0x41, 0xE9, 0x5A, 0x5F,
			0x02, 0x80, 0x00, 0x00,
		}},
		{"foobar", le, []byte{
			0x06, 0x00, 0x00, 0x00, // length
			0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72, // str
			0x00, // terminator
		}},
		{"foobar", be, []byte{
			0x00, 0x00, 0x00, 0x06, // length
			0x66, 0x6f, 0x6f, 0x62, 0x61, 0x72, // str
			0x00, // terminator
		}},
		{[]byte{1, 2, 3}, le, []byte{
			0x03, 0x00, 0x00, 0x00, // length
			0x01, 0x02, 0x03, // bytes
		}},
		{[]byte{1, 2, 3}, be, []byte{
			0x00, 0x00, 0x00, 0x03, // length
			0x01, 0x02, 0x03, // bytes
		}},
		{[][]string{{"fo", "bar"}, {"qux"}}, le, []byte{
			0x02, 0x00, 0x00, 0x00, // length (arr)

			0x02, 0x00, 0x00, 0x00, // length (arr[0])

			0x02, 0x00, 0x00, 0x00, // length ("fo")
			0x66, 0x6f, // "fo"
			0x00, // terminator
			0x00, // pad to next str

			0x03, 0x00, 0x00, 0x00, // length ("bar")
			0x62, 0x61, 0x72, // "bar",
			0x00, // terminator

			0x01, 0x00, 0x00, 0x00, // length (arr[1])

			0x03, 0x00, 0x00, 0x00, // length ("qux")
			0x71, 0x75, 0x78, // "qux"
			0x00, // terminator
		}},
		{[][]string{{"fo", "bar"}, {"qux"}}, be, []byte{
			0x00, 0x00, 0x00, 0x02, // length (arr)

			0x00, 0x00, 0x00, 0x02, // length (arr[0])

			0x00, 0x00, 0x00, 0x02, // length ("fo")
			0x66, 0x6f, // "fo"
			0x00, // terminator
			0x00, // pad to next str

			0x00, 0x00, 0x00, 0x03, // length ("bar")
			0x62, 0x61, 0x72, // "bar",
			0x00, // terminator

			0x00, 0x00, 0x00, 0x01, // length (arr[1])

			0x00, 0x00, 0x00, 0x03, // length ("qux")
			0x71, 0x75, 0x78, // "qux"
			0x00, // terminator
		}},

		{Simple{42, true}, le, []byte{
			0x2a, 0x00, // Simple.A
			0x00, 0x00, // pad to bool alignment
			0x01, 0x00, 0x00, 0x00, // Simple.B
		}},
		{Simple{42, true}, be, []byte{
			0x00, 0x2a, // Simple.A
			0x00, 0x00, // pad to bool alignment
			0x00, 0x00, 0x00, 0x01, // Simple.B
		}},

		{Nested{66, Simple{42, true}}, le, []byte{
			// Nested.A
			0x42,
			// pad to struct
			0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			// Nested.B.A
			0x2a, 0x00,
			// pad to bool
			0x00, 0x00,
			// Nested.B.B
			0x01, 0x00, 0x00, 0x00,
		}},
		{Nested{66, Simple{42, true}}, be, []byte{
			// Nested.A
			0x42,
			// pad to struct
			0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00,
			// Nested.B.A
			0x00, 0x2a,
			// pad to bool
			0x00, 0x00,
			// Nested.B.B
			0x00, 0x00, 0x00, 0x01,
		}},

		{Embedded{Simple{42, true}, 66}, le, []byte{
			// Embedded.Simple.A
			0x2a, 0x00,
			// pad to bool
			0x00, 0x00,
			// Embedded.Simple.B
			0x01, 0x00, 0x00, 0x00,
			// Embedded.C
			0x42,
		}},
		{Embedded{Simple{42, true}, 66}, be, []byte{
			// Embedded.Simple.A
			0x00, 0x2a,
			// pad to bool
			0x00, 0x00,
			// Embedded.Simple.B
			0x00, 0x00, 0x00, 0x01,
			// Embedded.C
			0x42,
		}},

		{EmbeddedShadow{Simple{42, true}, 66}, le, []byte{
			// Embedded.Simple.A
			0x2a, 0x00,
			// Embedded.B (shadowing Embedded.Simple.B)
			0x42,
		}},
		{EmbeddedShadow{Simple{42, true}, 66}, be, []byte{
			// Embedded.Simple.A
			0x00, 0x2a,
			// Embedded.B (shadowing Embedded.Simple.B)
			0x42,
		}},

		{SelfMarshalerVal{66}, le, []byte{
			0x43, 0x00,
		}},
		{SelfMarshalerVal{66}, be, []byte{
			0x00, 0x43,
		}},

		{&SelfMarshalerPtr{66}, le, []byte{0x43, 0x00}},
		{&SelfMarshalerPtr{66}, be, []byte{0x00, 0x43}},

		{&NestedSelfMarshalerPtr{66, SelfMarshalerPtr{42}}, le, []byte{
			// s.A
			0x42,
			// pad to marshaler value (deliberately weird/invalid for
			// dbus, to verify we're delegating to the Marshaler)
			0x00, 0x00,
			// s.B
			0x2b, 0x00,
		}},
		{&NestedSelfMarshalerPtr{66, SelfMarshalerPtr{42}}, be, []byte{
			// s.A
			0x42,
			// pad to marshaler value (deliberately weird/invalid for
			// dbus, to verify we're delegating to the Marshaler)
			0x00, 0x00,
			// s.B
			0x00, 0x2b,
		}},

		{[]SelfMarshalerVal{{1}, {2}}, le, []byte{
			// array length
			0x02, 0x00, 0x00, 0x00,
			// pad to multiple of 3
			0x00, 0x00,
			// arr[0]
			0x02, 0x00,
			// pad to multiple of 3
			0x00,
			0x03, 0x00,
		}},

		// no map tests here. The encoding is unstable due to
		// unpredictable map iteration ordering. The protocol doesn't
		// mind, but the tests do.
		//
		// Instead, map tests are provided via the decode testing,
		// which includes a check for round-trippability
		// (i.e. decode(encode(X)) == X) and tests maps.

		{func() int { return 2 }, le, nil},
		{func() int { return 2 }, be, nil},

		// Not addressable, and no exported fields - can't convert.
		{SelfMarshalerPtr{66}, le, []byte{}},
		{SelfMarshalerPtr{66}, be, []byte{}},
	}

	for _, tc := range tests {
		got, err := dbus.Marshal(tc.in, tc.enc)
		if err != nil {
			if len(tc.want) != 0 {
				t.Errorf("Marshal(%T) got err: %v", tc.in, err)
			} else if testing.Verbose() {
				t.Logf("Marshal(%T:%#v, %s) = err: %v", tc.in, tc.in, encName[tc.enc], err)
			}
		} else if len(tc.want) == 0 {
			t.Errorf("Marshal(%T) encoded successfully, want error", tc.in)
		} else if !bytes.Equal(got, tc.want) {
			t.Errorf("Marshal(%T) wrong encoding:\n  got: % x\n want: % x", tc.in, got, tc.want)
		} else if testing.Verbose() {
			t.Logf("Marshal(%T:%#v, %s) = % x", tc.in, tc.in, encName[tc.enc], got)
		}
	}
}

func ptr[T any](v T) *T {
	return &v
}
