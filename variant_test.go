package dbus_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/danderson/dbus"
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
			dbus.Variant{byte(5)},
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
			dbus.Variant{true},
		},

		{
			[]uint16{1, 2, 3},
			[]byte{
				// Signature string "an"
				0x02, 0x61, 0x71, 0x00,
				// val
				0x00, 0x00, 0x00, 0x03,
				0x00, 0x01,
				0x00, 0x02,
				0x00, 0x03,
			},
			dbus.Variant{[]uint16{1, 2, 3}},
		},

		{
			dbus.Signature("uu"),
			[]byte{
				// Signature string "g"
				0x01, 0x67, 0x00,
				// val
				0x02, 0x75, 0x75, 0x00,
			},
			dbus.Variant{dbus.Signature("uu")},
		},

		{
			struct{ A, B uint16 }{1, 2},
			[]byte{
				// Signature string "(qq)"
				0x04, 0x28, 0x71, 0x71, 0x29, 0x00,
				// pad to struct
				0x00, 0x00,
				// val
				0x00, 0x01,
				0x00, 0x02,
			},
			nil, // TODO: support structs
		},

		{
			dbus.Variant{uint16(42)},
			[]byte{
				// Signature string "v"
				0x01, 0x76, 0x00,
				// Inner signature string "q"
				0x01, 0x71, 0x00,
				// val
				0x00, 0x2a,
			},
			dbus.Variant{dbus.Variant{uint16(42)}},
		},
	}

	for _, tc := range tests {
		v := dbus.Variant{tc.in}
		got, err := dbus.Marshal(v, binary.BigEndian)
		if err != nil {
			if len(tc.want) != 0 {
				t.Errorf("Marshal(dbus.Variant{%T}) got err: %v", tc.in, err)
			} else if testing.Verbose() {
				t.Logf("Marshal(dbus.Variant{%T}) = err: %v", tc.in, err)
			}
			continue
		} else if len(tc.want) == 0 {
			t.Errorf("Marshal(dbus.Variant{%T}) encoded successfully, want error", tc.in)
			continue
		} else if !bytes.Equal(got, tc.want) {
			t.Errorf("Marshal(dbus.Variant{%T}) wrong encoding:\n  got: % x\n want: % x", tc.in, got, tc.want)
		} else if testing.Verbose() {
			t.Logf("Marshal(dbus.Variant{%T:%#v}) = % x", tc.in, tc.in, got)
		}

		if tc.wantUnmarshal == nil {
			continue
		}
		var gotU dbus.Variant
		err = dbus.Unmarshal(got, binary.BigEndian, &gotU)
		if err != nil {
			t.Errorf("Unmarshal(Marshal(dbus.Variant{%T})) got err: %v", tc.in, err)
		}
		if diff := cmp.Diff(gotU, tc.wantUnmarshal); diff != "" {
			t.Error(diff)
		} else {
			t.Logf("Unmarshal(...) = %#v", gotU)
		}
	}
}
