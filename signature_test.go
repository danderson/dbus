package dbus

import (
	"os"
	"reflect"
	"testing"
)

func TestSignatureOf(t *testing.T) {
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
		{Signature{}, "g"},
		{ObjectPath(""), "o"},
		{(*os.File)(nil), "h"},
		{[]string{}, "as"},
		{[4]byte{}, "ay"},
		{[][]string{}, "aas"},
		{map[string]int64{}, "a{sx}"},
		{Simple{}, "(nb)"},
		{[]Simple{}, "a(nb)"},
		{Nested{}, "(y(nb))"},
		{[]Nested{}, "a(y(nb))"},
		{Embedded{}, "(nby)"},
		{EmbeddedShadow{}, "(nby)"},
		{Arrays{}, "(asa(nb)aa(y(nb)))"},
		{ptr(any(int16(0))), "v"},
		{struct{ A any }{int16(0)}, "(v)"},
		{VarDict{}, "(a{sv})"},
		{VarDictByte{}, "(a{yv})"},
		{struct{}{}, "()"},

		{},
		{Tree{}, ""},
		{map[Simple]bool{}, ""},
		{map[[2]int64]bool{}, ""},
		{map[any]bool{}, ""},
		{func() int { return 2 }, ""},
	}

	for _, tc := range tests {
		gotSig, err := SignatureOf(tc.in)
		gotErr := err != nil
		wantErr := tc.want == ""
		if gotErr != wantErr {
			wanted := "no error"
			if wantErr {
				wanted = "error"
			}
			t.Errorf("SignatureOf(%T) got err %v, want %s", tc.in, err, wanted)
		}
		if got := gotSig.String(); got != tc.want {
			t.Errorf("SignatureOf(%T).String() = %q, want %q", tc.in, got, tc.want)
		} else if testing.Verbose() {
			t.Logf("SignatureOf(%T).String() = %q, err=%v", tc.in, got, err)
		}
	}
}

func TestParseSignature(t *testing.T) {
	tests := []struct {
		in      string
		want    reflect.Type
		wantErr bool
	}{
		{
			"(nb)",
			reflect.TypeFor[struct {
				Field0 int16
				Field1 bool
			}](),
			false,
		},
		{"y", reflect.TypeFor[byte](), false},
		{"b", reflect.TypeFor[bool](), false},
		{"n", reflect.TypeFor[int16](), false},
		{"q", reflect.TypeFor[uint16](), false},
		{"i", reflect.TypeFor[int32](), false},
		{"u", reflect.TypeFor[uint32](), false},
		{"x", reflect.TypeFor[int64](), false},
		{"t", reflect.TypeFor[uint64](), false},
		{"d", reflect.TypeFor[float64](), false},
		{"s", reflect.TypeFor[string](), false},
		{"g", reflect.TypeFor[Signature](), false},
		{"o", reflect.TypeFor[ObjectPath](), false},
		{"h", reflect.TypeFor[*os.File](), false},
		{"as", reflect.TypeFor[[]string](), false},
		{"ay", reflect.TypeFor[[]byte](), false},
		{"aas", reflect.TypeFor[[][]string](), false},
		{"a{sx}", reflect.TypeFor[map[string]int64](), false},
		{"(nb)", reflect.TypeFor[struct {
			Field0 int16
			Field1 bool
		}](), false},
		{"a(nb)", reflect.TypeFor[[]struct {
			Field0 int16
			Field1 bool
		}](), false},
		{"(y(nb))", reflect.TypeFor[struct {
			Field0 uint8
			Field1 struct {
				Field0 int16
				Field1 bool
			}
		}](), false},
		{"a(y(nb))", reflect.TypeFor[[]struct {
			Field0 uint8
			Field1 struct {
				Field0 int16
				Field1 bool
			}
		}](), false},
		{"(nby)", reflect.TypeFor[struct {
			Field0 int16
			Field1 bool
			Field2 uint8
		}](), false},
		{"(ny)", reflect.TypeFor[struct {
			Field0 int16
			Field1 uint8
		}](), false},
		{"(asa(nb)aa(y(nb)))", reflect.TypeFor[struct {
			Field0 []string
			Field1 []struct {
				Field0 int16
				Field1 bool
			}
			Field2 [][]struct {
				Field0 uint8
				Field1 struct {
					Field0 int16
					Field1 bool
				}
			}
		}](), false},
		{"v", reflect.TypeFor[any](), false},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, gotErr := ParseSignature(tc.in)
			if gotErr != nil {
				if tc.wantErr {
					return
				}
				t.Errorf("ParseSignature(%q) got err %v", tc.in, gotErr)
			} else if gotType := got.Type(); !reflect.DeepEqual(gotType, tc.want) {
				t.Errorf("ParseSignature(%q) got %s, want %s", tc.in, gotType, tc.want)
			}

			if gotStr := got.String(); gotStr != tc.in {
				t.Errorf("ParseSignature(%q).String() = %q, want %q", tc.in, gotStr, tc.in)
			}
		})
	}
}
