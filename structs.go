package dbus

import (
	"cmp"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// structField is the information about a struct field that needs to
// be marshaled/unmarshaled.
type structField struct {
	Name  string
	Index []int
	Type  reflect.Type

	UseVariantEncoding bool
	VarDictKey         string
	EncodeZeroValue    bool
}

func (s *structField) IsVarDict() bool { return s.VarDictKey != "" }
func (s *structField) IsVarDictMap() bool {
	return (s.Type.Kind() == reflect.Map &&
		mapKeyKinds.Has(s.Type.Key().Kind()) &&
		s.Type.Elem() == variantType)
}

type varDictField struct {
	*structField
	key reflect.Value
}

type structInfo struct {
	Name string
	Type reflect.Type

	StructFields []*structField

	VarDictFields []varDictField
	VarDictMap    *structField
}

func getStructInfo(t reflect.Type) (*structInfo, error) {
	ret := &structInfo{
		Name: t.String(),
		Type: t,
	}

	var varDictFields, otherFields []*structField
	for _, field := range reflect.VisibleFields(t) {
		if field.Anonymous || !field.IsExported() {
			continue
		}
		fi := getStructField(field)
		if fi.IsVarDict() {
			varDictFields = append(varDictFields, fi)
		} else {
			otherFields = append(otherFields, fi)
		}
	}

	if len(varDictFields) == 0 {
		// Simple struct, all done.
		ret.StructFields = otherFields
		return ret, nil
	}

	// Vardict struct. Validate its shape and parse out the keys for
	// later use.

	if len(otherFields) == 0 {
		return nil, fmt.Errorf("missing map[K]dbus.Variant in vardict struct %s", ret.Name)
	}
	if len(otherFields) > 1 {
		return nil, fmt.Errorf("multiple untagged fields in vardict struct %s, expecting only one of type map[K]dbus.Variant", ret.Name)
	}
	mt := otherFields[0].Type
	if !isValidVarDictMapType(mt) {
		return nil, fmt.Errorf("untagged field %s of struct %s must be a map[K]dbus.Variant", ret.Name, otherFields[0].Name)
	}

	ret.VarDictMap = otherFields[0]
	seen := map[string]bool{}
	keyParser := typeParser(mt.Key())
	for _, f := range varDictFields {
		v, err := keyParser(f.VarDictKey)
		if err != nil {
			return nil, fmt.Errorf("invalid vardict key %q for type %s in vardict struct %s: %w", f.VarDictKey, mt.Key(), ret.Name, err)
		}
		if seen[v.String()] {
			return nil, fmt.Errorf("duplicate vardict key %q for type %s", v.String(), ret.Name)
		}
		ret.VarDictFields = append(ret.VarDictFields, varDictField{f, v})
	}

	return ret, nil
}

func getStructField(f reflect.StructField) *structField {
	ret := &structField{
		Name:  f.Name,
		Index: f.Index,
		Type:  f.Type,
	}

	t := f.Tag.Get("dbus")
	for _, f := range strings.Split(t, ",") {
		if f == "variant" {
			ret.UseVariantEncoding = true
		} else if f == "encodeZero" {
			ret.EncodeZeroValue = true
		} else if val, ok := strings.CutPrefix(f, "key="); ok {
			if val == "@" {
				val = ret.Name
			}
			ret.VarDictKey = val
		}
	}
	return ret
}

func isValidVarDictMapType(t reflect.Type) bool {
	return t.Kind() == reflect.Map && mapKeyKinds.Has(t.Key().Kind()) && t.Elem() == variantType
}

func typeParser(t reflect.Type) func(string) (reflect.Value, error) {
	switch t.Kind() {
	case reflect.Bool:
		return func(s string) (reflect.Value, error) {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(b), nil
		}
	case reflect.Int16, reflect.Int32, reflect.Int64:
		return func(s string) (reflect.Value, error) {
			i64, err := strconv.ParseInt(s, 10, int(t.Size())*8)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(i64).Convert(t), nil
		}
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(s string) (reflect.Value, error) {
			u64, err := strconv.ParseUint(s, 10, int(t.Size())*8)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(u64).Convert(t), nil
		}
	case reflect.Float32, reflect.Float64:
		return func(s string) (reflect.Value, error) {
			f64, err := strconv.ParseFloat(s, int(t.Size())*8)
			if err != nil {
				return reflect.Value{}, err
			}
			return reflect.ValueOf(f64).Convert(t), nil
		}
	case reflect.String:
		return func(s string) (reflect.Value, error) {
			return reflect.ValueOf(s), nil
		}
	default:
		panic(fmt.Sprintf("invalid dbus map key type %s", t))
	}
}

func newMapKeyCompare(t reflect.Type) func(a, b reflect.Value) int {
	switch t.Kind() {
	case reflect.Bool:
		return func(a, b reflect.Value) int {
			if a.Bool() == b.Bool() {
				return 0
			}
			if !a.Bool() {
				return -1
			}
			return 1
		}
	case reflect.Int16, reflect.Int32, reflect.Int64:
		return func(a, b reflect.Value) int {
			return cmp.Compare(a.Int(), b.Int())
		}
	case reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return func(a, b reflect.Value) int {
			return cmp.Compare(a.Uint(), b.Uint())
		}
	case reflect.Float32, reflect.Float64:
		return func(a, b reflect.Value) int {
			return cmp.Compare(a.Float(), b.Float())
		}
	case reflect.String:
		return func(a, b reflect.Value) int {
			return cmp.Compare(a.String(), b.String())
		}
	default:
		panic("invalid map key type")
	}
}
