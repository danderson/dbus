package dbus

import (
	"cmp"
	"fmt"
	"iter"
	"reflect"
	"slices"
	"strconv"
	"strings"
)

// InlineLayout marks a struct as being inlined. A struct with a field
// of type InlineLayout will be laid out in DBus messages without the
// initial 8-byte alignment that DBus structs normally enforce.
type InlineLayout struct{}

// structField is the information about a struct field that needs to
// be marshaled/unmarshaled.
type structField struct {
	Name  string
	Index [][]int
	Type  reflect.Type

	// VarDictFields are the key-specific fields associated with this
	// structField. This structField must be a vardict map
	// (map[K]any).
	//
	// VarDictFields is always of type map[K]*varDictField, but has to
	// be a reflect.Value here because the vardict's key type is only
	// known at runtime.
	VarDictFields reflect.Value
}

// IsVarDict reports whether the struct field is a vardict, with
// attached associated fields.
func (f *structField) IsVarDict() bool {
	return f.VarDictFields.IsValid()
}

// VarDictKeyCmp returns a comparison function for the vardict's key
// type. Panics if the field is not a vardict.
func (f *structField) VarDictKeyCmp() func(a, b reflect.Value) int {
	return mapKeyCmp(f.Type.Key())
}

// VarDictField returns the varDictField information for the field
// associated with the given vardict key, or nil if there is no
// associated field.
func (f *structField) VarDictField(key reflect.Value) *varDictField {
	ret := f.VarDictFields.MapIndex(key)
	if ret.IsZero() {
		return nil
	}
	return ret.Interface().(*varDictField)
}

// GetWithZero loads the struct field from structVal. If loading
// requires traversing a nil pointer into an embedded struct,
// GetWithZero returns a non-settable zero value of the field.
func (f *structField) GetWithZero(structVal reflect.Value) reflect.Value {
	v := structVal
	for i, hop := range f.Index {
		if i > 0 {
			if v.IsNil() {
				return reflect.Zero(f.Type)
			}
			v = v.Elem()
		}
		v = v.FieldByIndex(hop)
	}
	return v
}

// GetWithAlloc loads the struct field from structVal. If loading
// requires traversing a nil pointer into an embedded struct,
// GetWithAlloc allocates zero values appropriately. The returned
// [reflect.Value] is settable.
func (f *structField) GetWithAlloc(structVal reflect.Value) reflect.Value {
	v := structVal
	for i, hop := range f.Index {
		if i > 0 {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.FieldByIndex(hop)
	}
	return v
}

func (f *structField) String() string {
	var ret strings.Builder
	kindStr := ""
	if ks := f.Type.Kind().String(); ks != f.Type.String() {
		kindStr = fmt.Sprintf(" (%s)", ks)
	}
	fmt.Fprintf(&ret, "%s: %s%s at %v", f.Name, f.Type, kindStr, f.Index)
	if f.VarDictFields.IsValid() {
		ret.WriteString(", vardict fields:")
		ks := f.VarDictFields.MapKeys()
		slices.SortFunc(ks, mapKeyCmp(f.VarDictFields.Type().Key()))
		for _, k := range ks {
			v := f.VarDictField(k)
			encodeZero := ""
			if v.EncodeZero {
				encodeZero = "(encode zero) "
			}
			fmt.Fprintf(&ret, "\n  %v: %s%s", v.StrKey, v, encodeZero)
		}
	}
	return ret.String()
}

// varDictField describes an "associated field" of a vardict. An
// associated field stores the vardict value for a particular key with
// strong typing, as opposed to the vardict's default any values.
type varDictField struct {
	*structField
	Key    reflect.Value
	StrKey string
	// EncodeZero is whether to encode zero values into the
	// vardict. By default, zero values are presumed to be unset
	// optional values and skipped.
	EncodeZero bool
}

// structInfo is the information about a struct relevant to
// marshaling/unmarshaling.
type structInfo struct {
	// Name is the struct's name, for use in diagnostics.
	Name string
	// Type is the struct's type, for use in diagnostics.
	Type reflect.Type
	// NoPad, if true, specifies that the struct should be aligned
	// according to the alignment of its first encoded field, instead
	// of the customary 8-byte alignment.
	NoPad bool

	// StructFields is the information about each struct field
	// eligible for DBus encoding/decoding.
	StructFields []*structField
}

func (s *structInfo) String() string {
	var ret strings.Builder
	name, typ := s.Name, s.Type.String()
	if s.Type.Kind() == reflect.Struct {
		typ = "struct"
	}
	fmt.Fprintf(&ret, "%s: %s, fields:\n", name, typ)
	for _, f := range s.StructFields {
		ret.WriteString(f.String())
		ret.WriteByte('\n')
	}
	return ret.String()
}

// getStructInfo returns the structInfo for t.
//
// getStructInfo returns an error if t is not a struct, or if the
// struct is malformed in a way that prevents its use for dbus
// messaging.
func getStructInfo(t reflect.Type) (*structInfo, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%s is not a struct", t)
	}

	ret := &structInfo{
		Name: t.String(),
		Type: t,
	}

	var (
		varDictMap    *structField
		varDictFields []*varDictField
	)
	for field := range structFields(t, nil) {
		if !field.IsExported() {
			if field.Type == reflect.TypeFor[InlineLayout]() {
				ret.NoPad = true
			}
			continue
		}

		encodeZero, isVardict, vardictKey := parseStructTag(field)
		fieldInfo := &structField{
			Name:  field.Name,
			Type:  field.Type,
			Index: allocSteps(t, field.Index),
		}

		if isVardict {
			if !isValidVarDictMapType(fieldInfo.Type) {
				return nil, fmt.Errorf("vardict map %s.%s must be a map[K]any", ret.Name, fieldInfo.Name)
			}
			fieldInfo.VarDictFields = reflect.MakeMap(reflect.MapOf(
				fieldInfo.Type.Key(),
				reflect.TypeFor[*varDictField]()))
			varDictMap = fieldInfo
			ret.StructFields = append(ret.StructFields, fieldInfo)
		} else if vardictKey != "" {
			varDictFields = append(varDictFields, &varDictField{
				structField: fieldInfo,
				StrKey:      vardictKey,
				EncodeZero:  encodeZero,
			})
		} else {
			ret.StructFields = append(ret.StructFields, fieldInfo)
		}
	}

	if len(varDictFields) == 0 {
		// Simple struct, all done.
		return ret, nil
	}

	// Struct containing vardict fields. Vardict struct. Validate its
	// shape and parse out keys for later use.

	if varDictMap == nil {
		return nil, fmt.Errorf("vardict fields declared in struct %s, but no map[K]any tagged with 'vardict'", ret.Name)
	}

	seen := map[string]*varDictField{}
	keyParser := mapKeyParser(varDictMap.Type.Key())
	for _, f := range varDictFields {
		v, err := keyParser(f.StrKey)
		if err != nil {
			return nil, fmt.Errorf("invalid key %q for vardict field %s.%s (expected type %s): %w", f.StrKey, ret.Name, f.Name, varDictMap.Type.Key(), err)
		}

		// Careful, v.String() only returns the underlying value if
		// it's a string or implements Stringer! Other values get a
		// placeholder with just the type name. fmt has special
		// handling of reflect.Value to always print the underlying
		// value.
		canonicalKey := fmt.Sprint(v)
		f.Key = v
		if prev := seen[canonicalKey]; prev != nil {
			if canonicalKey != f.StrKey {
				return nil, fmt.Errorf("duplicate vardict key %q (canonicalized from %q) in struct %s, used by %s and %s", canonicalKey, f.StrKey, ret.Name, f.Name, prev.Name)
			}
			return nil, fmt.Errorf("duplicate vardict key %q for type %s", f.StrKey, ret.Name)
		}
		// Parsing the key can change its value (e.g. ParseBool
		// coerces "true", "TRUE", "1" to bool(true)). Store the
		// canonical key.
		f.StrKey = canonicalKey
		varDictMap.VarDictFields.SetMapIndex(f.Key, reflect.ValueOf(f))
	}

	return ret, nil
}

// parseStructTag returns the information contained in field's "dbus"
// struct tag.
func parseStructTag(field reflect.StructField) (encodeZero, isVardict bool, vardictKey string) {
	for _, f := range strings.Split(field.Tag.Get("dbus"), ",") {
		if f == "encodeZero" {
			encodeZero = true
		} else if f == "vardict" {
			isVardict = true
		} else if val, ok := strings.CutPrefix(f, "key="); ok {
			if val == "@" {
				vardictKey = field.Name
			} else {
				vardictKey = val
			}
		}
	}
	return encodeZero, isVardict, vardictKey
}

// isValidVarDictMapType reports whether t is a valid vardict type,
// i.e. a map[K]any where K is a valid dbus map key type.
func isValidVarDictMapType(t reflect.Type) bool {
	return t.Kind() == reflect.Map && mapKeyKinds.Has(t.Key().Kind()) && t.Elem() == reflect.TypeFor[any]()
}

// mapKeyParser returns a function that converts strings into values
// of the given map key type.
func mapKeyParser(t reflect.Type) func(string) (reflect.Value, error) {
	if !mapKeyKinds.Has(t.Kind()) {
		panic("mapKeyParser called on type that can't be a map key")
	}

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

// mapKeyCmp returns a comparison function for the given map key type.
func mapKeyCmp(t reflect.Type) func(a, b reflect.Value) int {
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

// allocSteps partitions a multi-hop traversal of struct fields into
// segments that end at either the final value, or at a struct pointer
// that might be nil.
//
// This partition is used by [structField.GetWithZero] and
// [structField.GetWithAlloc] to load embedded struct fields that
// require traversing a nil pointer.
func allocSteps(t reflect.Type, idx []int) [][]int {
	var ret [][]int
	prev := 0
	t = t.Field(idx[0]).Type
	for i := 1; i < len(idx); i++ {
		if t.Kind() == reflect.Pointer && t.Elem().Kind() == reflect.Struct {
			// Hop through a struct pointer that might be nil, cut.
			ret = append(ret, idx[prev:i])
			prev = i
			t = t.Elem()
		}
		t = t.Field(idx[i]).Type
	}
	ret = append(ret, idx[prev:])
	return ret
}

// alignAsStruct reports whether t aligns like a DBus struct, i.e. to
// 8 byte boundaries.
func alignAsStruct(t reflect.Type) bool {
	t, _ = derefType(t)
	if t.Kind() != reflect.Struct {
		return false
	}
	fs, err := getStructInfo(t)
	if err != nil {
		panic(err)
	}
	return !fs.NoPad
}

func structFields(t reflect.Type, idx []int) iter.Seq[reflect.StructField] {
	return func(yield func(reflect.StructField) bool) {
		for i := range t.NumField() {
			f := t.Field(i)
			idx = append(idx, i)
			if f.Anonymous {
				at := f.Type
				if at.Kind() == reflect.Pointer {
					at = at.Elem()
				}
				if at.Kind() == reflect.Struct {
					for af := range structFields(at, idx) {
						if !yield(af) {
							return
						}
					}
					idx = idx[:len(idx)-1]
					continue
				}
			}
			f.Index = append([]int(nil), idx...)
			if !yield(f) {
				return
			}
			idx = idx[:len(idx)-1]
		}
	}
}
