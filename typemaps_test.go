package dbus

import "testing"

func TestTypeMaps(t *testing.T) {
	for want, typ := range strToType {
		if got := typeToStr[typ]; got != want {
			t.Errorf("typeToStr[%v] = %q, want %q", typ, got, want)
		}
	}

	for want, b := range typeToStr {
		if got := strToType[b]; got != want {
			t.Errorf("strToType[%q] = %v, want %v", b, got, want)
		}
	}

	for kind, typ := range kindToType {
		if got := typeToStr[typ]; got == 0 {
			t.Errorf("kindToType[%v] = %v, has no typeToStr", kind, typ)
		}
	}
}
