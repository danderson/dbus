package dbus

import (
	"bytes"
	"context"
	"testing"

	"github.com/danderson/dbus/fragments"
	"github.com/google/go-cmp/cmp"
)

func TestMarshalVariant(t *testing.T) {
	tests := []struct {
		in            any
		want          []byte // empty for error
		wantUnmarshal any
	}{
		{},
		{
			byte(5),
			[]byte{
				// Signature string "y"
				0x01, 0x79, 0x00,
				// val
				0x05,
			},
			Variant{byte(5)},
		},

		{
			true,
			[]byte{
				// Signature string "b"
				0x01, 0x62, 0x00,
				// pad to bool
				0x00,
				// val
				0x00, 0x00, 0x00, 0x01,
			},
			Variant{true},
		},

		{
			[]uint16{1, 2, 3},
			[]byte{
				// Signature string "an"
				0x02, 0x61, 0x71, 0x00,
				// val
				0x00, 0x00, 0x00, 0x06,
				0x00, 0x01,
				0x00, 0x02,
				0x00, 0x03,
			},
			Variant{[]uint16{1, 2, 3}},
		},

		{
			mustParseSignature("uu"),
			[]byte{
				// Signature string "g"
				0x01, 0x67, 0x00,
				// val
				0x04, 0x28, 0x75, 0x75, 0x29, 0x00,
			},
			Variant{mustParseSignature("uu")},
		},

		{
			Simple{A: 2, B: true},
			[]byte{
				// Signature string "(qq)"
				0x04, 0x28, 0x6e, 0x62, 0x29, 0x00,
				// pad to struct
				0x00, 0x00,
				// val
				0x00, 0x02, // A
				0x00, 0x00, // pad
				0x00, 0x00, 0x00, 0x01, // B
			},
			Variant{struct {
				Field0 int16
				Field1 bool
			}{2, true}},
		},

		{
			Variant{uint16(42)},
			[]byte{
				// Signature string "v"
				0x01, 0x76, 0x00,
				// Inner signature string "q"
				0x01, 0x71, 0x00,
				// val
				0x00, 0x2a,
			},
			Variant{Variant{uint16(42)}},
		},
	}

	for _, tc := range tests {
		v := Variant{tc.in}
		enc := fragments.Encoder{
			Order:  fragments.BigEndian,
			Mapper: encoderFor,
		}
		if err := enc.Value(context.Background(), v); err != nil {
			if len(tc.want) != 0 {
				t.Errorf("Marshal(Variant{%T}) got err: %v", tc.in, err)
			} else if testing.Verbose() {
				t.Logf("Marshal(Variant{%T}) = err: %v", tc.in, err)
			}
			continue
		} else if len(tc.want) == 0 {
			t.Errorf("Marshal(Variant{%T}) encoded successfully, want error", tc.in)
			continue
		} else if !bytes.Equal(enc.Out, tc.want) {
			t.Errorf("Marshal(Variant{%T}) wrong encoding:\n  got: % x\n want: % x", tc.in, enc.Out, tc.want)
		} else if testing.Verbose() {
			t.Logf("Marshal(Variant{%T:%#v}) = % x", tc.in, tc.in, enc.Out)
		}

		if tc.wantUnmarshal == nil {
			continue
		}
		var gotU Variant
		dec := fragments.Decoder{
			Order:  fragments.BigEndian,
			Mapper: decoderFor,
			In:     bytes.NewBuffer(enc.Out),
		}
		if err := dec.Value(context.Background(), &gotU); err != nil {
			t.Errorf("Unmarshal(Marshal(Variant{%T})) got err: %v", tc.in, err)
		}
		if diff := cmp.Diff(gotU, tc.wantUnmarshal, cmp.Comparer(func(a, b Signature) bool {
			return a.String() == b.String()
		})); diff != "" {
			t.Error(diff)
		} else {
			t.Logf("Unmarshal(...) = %#v", gotU)
		}
	}
}
