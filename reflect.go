package dbus

import (
	"iter"
	"reflect"
)

// alignAsStruct reports whether t aligns like a DBus struct, i.e. to
// 8 byte boundaries.
func alignAsStruct(t reflect.Type) bool {
	t = derefType(t)
	if t.Kind() != reflect.Struct {
		return false
	}
	fs, err := getStructInfo(t)
	if err != nil {
		panic(err)
	}
	return !fs.NoPad
}

func derefType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func derefZero(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

func derefAlloc(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	return v
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
