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

// Match is a filter that matches DBus signals.
type Match struct {
	sender       value.Maybe[string]
	object       value.Maybe[ObjectPath]
	objectPrefix value.Maybe[ObjectPath]
	signal       value.Maybe[signalMatch]
	property     value.Maybe[propertyMatch]
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

type propertyMatch struct {
	iface string
	prop  string
}

// Signal creates a Match for the given signal type.
//
// The provided signal type must be registered with
// [RegisterSignalType] prior to calling MatchSignal.
func MatchSignal[SignalT any]() *Match {
	t := reflect.TypeFor[SignalT]()
	bt, _ := derefType(t)
	k, ok := signalNameFor(bt)
	if !ok {
		panic(fmt.Errorf("unknown signal type %s", bt))
	}

	sm := signalMatch{
		iface:        k.Interface,
		member:       k.Method,
		stringFields: map[int]func(reflect.Value) string{},
		objectFields: map[int]func(reflect.Value) ObjectPath{},
	}

	inf, err := getStructInfo(bt)
	if err != nil {
		panic(fmt.Errorf("getting signal struct info for %s: %w", bt, err))
	}
	for i, field := range inf.StructFields {
		fieldBottom, derefField := derefType(field.Type)
		if fieldBottom == reflect.TypeFor[ObjectPath]() {
			sm.objectFields[i] = getter[ObjectPath](field, derefField)
		} else if fieldBottom.Kind() == reflect.String {
			sm.stringFields[i] = getter[string](field, derefField)
		}
	}

	return &Match{
		signal: value.Just(sm),
	}
}

// MatchProperty creates a Match for the given property type.
//
// The provided property type must be registered with
// [RegisterPropertyChangeType] prior to calling MatchProperty.
func MatchProperty[PropT any]() *Match {
	t := reflect.TypeFor[PropT]()
	bt, _ := derefType(t)
	k, ok := propNameFor(bt)
	if !ok {
		panic(fmt.Errorf("unknown property type %s", t))
	}

	pm := propertyMatch{
		iface: k.Interface,
		prop:  k.Method,
	}

	return &Match{
		property: value.Just(pm),
	}
}

// NewMatch creates a Match for all signals.
func MatchAllSignals() *Match {
	return &Match{}
}

// valid reports whether the match is structurally valid.
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

// filterString returns the match in the string format that DBus wants
// for the AddMatch and RemoveMatch methods.
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
	if pm, ok := m.property.GetOK(); ok {
		kv("interface", "org.freedesktop.DBus.Properties")
		kv("member", "PropertiesChanged")
		kv("arg0", pm.iface)
	}

	if sm, ok := m.signal.GetOK(); ok {
		kv("interface", sm.iface)
		kv("member", sm.member)

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
	}

	return strings.Join(ms, ",")
}

// clone makes a deep copy of m.
func (m *Match) clone() *Match {
	ret := *m
	ret.argStr = maps.Clone(m.argStr)
	ret.argPath = maps.Clone(m.argPath)
	return &ret
}

// matchesSignal reports whether the given signal header and body
// matches the filter, using the same match logic that the bus uses on
// the match's filterString().
//
// This is necessary because a DBus connection receives a single
// stream of signals. When multiple Watchers are active, the received
// signals are the union of all the Watchers' filters, and so each one
// needs to do additional filtering on received signals.
func (m *Match) matchesSignal(hdr *header, body reflect.Value) bool {
	if m.property.Present() {
		return false
	}

	if s, ok := m.sender.GetOK(); ok && hdr.Sender != s {
		return false
	}
	if o, ok := m.object.GetOK(); ok && hdr.Path != o {
		return false
	}
	if p, ok := m.objectPrefix.GetOK(); ok && hdr.Path != p && !hdr.Path.IsChildOf(p) {
		return false
	}

	if sm, ok := m.signal.GetOK(); ok {
		if hdr.Interface != sm.iface || hdr.Member != sm.member {
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
	}

	return true
}

// matchesSignal reports whether the given property change matches the
// filter.
func (m *Match) matchesProperty(hdr *header, prop interfaceMethod, body reflect.Value) bool {
	pm, ok := m.property.GetOK()
	if !ok {
		return false
	}

	if s, ok := m.sender.GetOK(); ok && hdr.Sender != s {
		return false
	}
	if o, ok := m.object.GetOK(); ok && hdr.Path != o {
		return false
	}
	if p, ok := m.objectPrefix.GetOK(); ok && hdr.Path != p && !hdr.Path.IsChildOf(p) {
		return false
	}
	if hdr.Interface != "org.freedesktop.DBus.Properties" || hdr.Member != "PropertiesChanged" {
		return false
	}
	if pm.iface != prop.Interface || pm.prop != prop.Method {
		return false
	}

	return true
}

func getter[T any](f *structField, derefField bool) func(reflect.Value) T {
	if derefField {
		return func(v reflect.Value) T {
			v = deref(f.GetWithZero(v))
			if !v.IsValid() {
				var zero T
				return zero
			}
			return v.Interface().(T)
		}
	} else {
		return func(v reflect.Value) T {
			return f.GetWithZero(v).Interface().(T)
		}
	}
}

func derefType(t reflect.Type) (reflect.Type, bool) {
	deref := t.Kind() == reflect.Pointer
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t, deref
}

func deref(v reflect.Value) reflect.Value {
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return reflect.Value{}
		}
		v = v.Elem()
	}
	return v
}

// Sender restricts the Match to a single sending Peer.
func (m *Match) Peer(p Peer) *Match {
	m.sender = value.Just(p.Name())
	return m
}

// Object restricts the match to a single sending Object.
func (m *Match) Object(o Object) *Match {
	m.objectPrefix = value.Absent[ObjectPath]()
	m.object = value.Just(o.Path().Clean())
	return m
}

// ObjectPrefix restricts the Match to the Objects rooted at the given
// path prefix.
//
// For example, ObjectPrefix("/mascots/gopher") matches signals
// emitted by /mascots/gopher, /mascots/gopher/plushie,
// /mascots/gopher/art/renee-french, but not /mascots/glenda.
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

// ArgStr restricts the Match to signals whose i-th body field is a
// string equal to val.
//
// ArgStr can only be used on matches created with [MatchSignal].
func (m *Match) ArgStr(i int, val string) *Match {
	if m.argStr == nil {
		m.argStr = map[int]string{}
	}
	m.argStr[i] = val
	return m
}

// ArgPathPrefix restricts the Match to signals whose i-th body field
// is an object path with the given prefix.
//
// ArgPathPrefix can only be used on matches created with
// [MatchSignal].
func (m *Match) ArgPathPrefix(i int, val ObjectPath) *Match {
	if m.argPath == nil {
		m.argPath = map[int]ObjectPath{}
	}
	m.argPath[i] = val
	return m
}

// Arg0Namespace restricts the Match to signals whose first body field
// is a peer or interface name with the given dot-separated prefix.
//
// Arg0Namespace can only be used on matches created with
// [MatchSignal].
func (m *Match) Arg0Namespace(val string) *Match {
	m.arg0NS = value.Just(val)
	return m
}

func escapeMatchArg(s string) string {
	s = strings.ReplaceAll(s, "'", "'\\''")
	return "'" + s + "'"
}
