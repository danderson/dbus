package dbus

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/creachadair/mds/value"
)

type Match struct {
	sender       value.Maybe[string]
	object       value.Maybe[ObjectPath]
	objectPrefix value.Maybe[ObjectPath]
	signal       value.Maybe[signalMatch]
	argStr       map[int]string
	argPath      map[int]ObjectPath
	arg0NS       value.Maybe[string]
}

type signalMatch struct {
	stringFields map[int]func(reflect.Value) string
	objectFields map[int]func(reflect.Value) ObjectPath
	iface        string
	member       string
}

func NewMatch() *Match {
	return &Match{}
}

func (m *Match) valid() error {
	if len(m.argStr) == 0 && len(m.argPath) == 0 && !m.arg0NS.Present() {
		return nil
	}

	sm, ok := m.signal.GetOK()
	if !ok {
		return errors.New("matches on ArgStr(), ArgPathPrefix(), or Arg0Namespace() must also match on Signal()")
	}

	for i := range m.argStr {
		if sm.stringFields[i] == nil {
			return fmt.Errorf("invalid ArgStr match on arg %d, argument is not a string", i)
		}
	}
	for i := range m.argPath {
		if sm.stringFields[i] == nil && sm.objectFields[i] == nil {
			return fmt.Errorf("invalid ArgPathPrefix match on arg %d, argument is not a string or an ObjectPath", i)
		}
	}
	if m.arg0NS.Present() && sm.stringFields[0] == nil {
		return errors.New("invalid Arg0Namespace match on arg 0, argument is not a string")
	}

	return nil
}

func (m *Match) filterString() string {
	ms := []string{"type='signal'"}
	kv := func(k string, v string) {
		ms = append(ms, fmt.Sprintf("%s=%s", k, escapeMatchArg(v)))
	}

	if s, ok := m.sender.GetOK(); ok {
		kv("sender", s)
	}
	if o, ok := m.object.GetOK(); ok {
		kv("path", o.String())
	}
	if p, ok := m.objectPrefix.GetOK(); ok {
		ms = append(ms, "path_namespace="+p.String())
		//kv("path_namespace", p.String())
	}
	if sm, ok := m.signal.GetOK(); ok {
		kv("interface", sm.iface)
		kv("member", sm.member)
	}
	for _, i := range slices.Sorted(maps.Keys(m.argStr)) {
		k := fmt.Sprintf("arg%d", i)
		kv(k, m.argStr[i])
	}
	for _, i := range slices.Sorted(maps.Keys(m.argPath)) {
		k := fmt.Sprintf("arg%dpath", i)
		kv(k, m.argPath[i].String())
	}
	if n, ok := m.arg0NS.GetOK(); ok {
		kv("arg0namespace", n)
	}
	return strings.Join(ms, ",")
}

func (m *Match) clone() *Match {
	ret := *m
	ret.argStr = maps.Clone(m.argStr)
	ret.argPath = maps.Clone(m.argPath)
	return &ret
}

func (m *Match) matches(hdr *header, body reflect.Value) bool {
	if s, ok := m.sender.GetOK(); ok && hdr.Sender != s {
		return false
	}
	if o, ok := m.object.GetOK(); ok && hdr.Path != o {
		return false
	}
	if p, ok := m.objectPrefix.GetOK(); ok && hdr.Path != p && !hdr.Path.IsChildOf(p) {
		return false
	}
	sm, ok := m.signal.GetOK()
	if ok && (hdr.Interface != sm.iface || hdr.Member != sm.member) {
		return false
	}

	for i, want := range m.argStr {
		if got := sm.stringFields[i](body.Elem()); got != want {
			return false
		}
	}
	for i, want := range m.argPath {
		if f := sm.stringFields[i]; f != nil {
			if got := f(body.Elem()); got != want.String() && !ObjectPath(got).IsChildOf(want) {
				return false
			}
		}
		if f := sm.objectFields[i]; f != nil {
			if got := f(body.Elem()); got != want && !got.IsChildOf(want) {
				return false
			}
		}
	}
	if n, ok := m.arg0NS.GetOK(); ok {
		if got := sm.stringFields[0](body.Elem()); got != n && !strings.HasPrefix(got, n+".") {
			return false
		}
	}

	return true
}

func (m *Match) Signal(v any) *Match {
	t := reflect.TypeOf(v)
	k := signalTypeToName[t]
	if k.Interface == "" || k.Signal == "" {
		panic(fmt.Errorf("unknown signal type %T", v))
	}

	sm := signalMatch{
		iface:        k.Interface,
		member:       k.Signal,
		stringFields: map[int]func(reflect.Value) string{},
		objectFields: map[int]func(reflect.Value) ObjectPath{},
	}
	switch t.Kind() {
	case reflect.String:
		sm.stringFields[0] = func(v reflect.Value) string { return v.String() }
	case reflect.Struct:
		inf, err := getStructInfo(t)
		if err != nil {
			panic(fmt.Errorf("getting signal struct info for %s: %w", t, err))
		}
		for i, field := range inf.StructFields {
			if field.Type == reflect.TypeFor[ObjectPath]() {
				sm.objectFields[i] = func(v reflect.Value) ObjectPath {
					return field.GetWithZero(v).Interface().(ObjectPath)
				}
			} else if field.Type.Kind() == reflect.String {
				sm.stringFields[i] = func(v reflect.Value) string {
					return field.GetWithZero(v).String()
				}
			}
		}
	}

	m.signal = value.Just(sm)
	return m
}

func (m *Match) Sender(s string) *Match {
	m.sender = value.Just(s)
	return m
}

func (m *Match) Object(o ObjectPath) *Match {
	m.objectPrefix = value.Absent[ObjectPath]()
	m.object = value.Just(o.Clean())
	return m
}

func (m *Match) ObjectPrefix(o ObjectPath) *Match {
	m.object = value.Absent[ObjectPath]()
	if o == "/" {
		// workaround for dbus-broker bug: / means the same as not
		// specifying a path match anyway, so don't include it.
		m.objectPrefix = value.Absent[ObjectPath]()
	} else {
		m.objectPrefix = value.Just(o.Clean())
	}
	return m
}

func (m *Match) ArgStr(i int, val string) *Match {
	if m.argStr == nil {
		m.argStr = map[int]string{}
	}
	m.argStr[i] = val
	return m
}

func (m *Match) ArgPathPrefix(i int, val ObjectPath) *Match {
	if m.argPath == nil {
		m.argPath = map[int]ObjectPath{}
	}
	m.argPath[i] = val
	return m
}

func (m *Match) Arg0Namespace(val string) *Match {
	m.arg0NS = value.Just(val)
	return m
}

func escapeMatchArg(s string) string {
	s = strings.ReplaceAll(s, "'", "'\\''")
	return "'" + s + "'"
}
