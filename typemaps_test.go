package dbus

import "testing"

func TestTypeMaps(t *testing.T) {
	for want, typ := range strToType {
		if got := typeToStr[typ]; got == want {
			continue
		}
		if got := kindToStr[typ.Kind()]; got == want {
			continue
		}
		t.Errorf("strToType[%q] = %v, but no reverse mapping in typeToStr or kindToStr", typ, want)
	}

	for want, b := range typeToStr {
		if got := strToType[b]; got != want {
			t.Errorf("strToType[%q] = %v, want %v", b, got, want)
		}
	}

	for kind, typ := range kindToType {
		str := kindToStr[kind]
		if str == 0 {
			t.Errorf("kindToType[%v] = %v, has no kindToStr", kind, typ)
		}

		if gotType := strToType[str]; gotType != typ {
			t.Errorf("kindToType[%s] = %s, kindToStr[%s] = %q, but no strToType[%q] = %s, want %s", kind, typ, kind, str, str, gotType, typ)
		}
	}

	for _, kind := range mapKeyKinds.Slice() {
		if kindToStr[kind] == 0 {
			t.Errorf("map key kind %s has no kindToStr", kind)
		}

		if kindToType[kind] == nil {
			t.Errorf("map key kind %s has no kindToType", kind)
		}
	}
}
