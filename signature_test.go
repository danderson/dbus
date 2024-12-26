package dbus_test

import (
	"testing"

	"github.com/danderson/dbus"
)

func TestSignatureOf(t *testing.T) {
	type Tree struct {
		Left  *Tree
		Right *Tree
	}
	type Simple struct {
		A int16
		B bool
		C string
	}
	type Nested struct {
		A int64
		B Simple
		C *Simple
	}
	type Embedded struct {
		Simple
		D float64
	}
	type EmbedShadow struct {
		A      bool
		Simple // B, C visible
	}
	type Arrays struct {
		A []string
		B []Simple
		C [][]Nested
	}

	tests := []struct {
		in   any
		want string
	}{
		{byte(0), "y"},
		{bool(false), "b"},
		{int16(0), "n"},
		{uint16(0), "q"},
		{int32(0), "i"},
		{uint32(0), "u"},
		{int64(0), "x"},
		{uint64(0), "t"},
		{float64(0), "d"},
		{string(""), "s"},
		{dbus.Signature(""), "g"},
		{dbus.ObjectPath(""), "o"},
		{(*dbus.FileDescriptor)(nil), "h"},
		{[]string{}, "as"},
		{[4]byte{}, "ay"},
		{[][]string{}, "aas"},
		{map[string]int64{}, "a{sx}"},
		{Simple{}, "(nbs)"},
		{[]Simple{}, "a(nbs)"},
		{Nested{}, "(x(nbs)(nbs))"},
		{[]Nested{}, "a(x(nbs)(nbs))"},
		{Embedded{}, "(nbsd)"},
		{EmbedShadow{}, "(bbs)"},
		{Arrays{}, "(asa(nbs)aa(x(nbs)(nbs)))"},
		{dbus.Variant{int16(0)}, "v"},

		{},
		{struct{}{}, ""},
		{Tree{}, ""},
		{map[Simple]bool{}, ""},
		{map[[2]int64]bool{}, ""},
		{map[dbus.Variant]bool{}, ""},
		{func() int { return 2 }, ""},
	}

	for _, tc := range tests {
		got, err := dbus.SignatureOf(tc.in)
		gotErr := err != nil
		wantErr := tc.want == ""
		if gotErr != wantErr {
			wanted := "no error"
			if wantErr {
				wanted = "error"
			}
			t.Errorf("SignatureOf(%T) got err %v, want %s", tc.in, err, wanted)
		}
		if string(got) != tc.want {
			t.Errorf("SignatureOf(%T) = %q, want %q", tc.in, got, tc.want)
		} else if testing.Verbose() {
			t.Logf("SignatureOf(%T) = %q, err=%v", tc.in, got, err)
		}
	}
}
