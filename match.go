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
	property     value.Maybe[interfaceMember]
	argStr       map[int]string
	argPath      map[int]ObjectPath
	arg0NS       value.Maybe[string]
}

type signalMatch struct {
	interfaceMember
	stringFields map[int]func(reflect.Value) string
	objectFields map[int]func(reflect.Value) string
}

// MatchNotification returns a match for the given notification.
//
// The provided notification type must be registered with
// [RegisterSignalType] or [RegisterPropertyChangeType] prior to
// calling MatchNotification.
func MatchNotification[NotificationT any]() *Match {
	t := reflect.TypeFor[NotificationT]()
	bt := derefType(t)

	prop, ok := propNameFor(bt)
	if ok {
		return &Match{
			property: value.Just(prop),
		}
	}

	sig, ok := signalNameFor(bt)
	if !ok {
		panic(fmt.Errorf("unknown notification type %s", bt))
	}

	sm := signalMatch{
		interfaceMember: sig,
		stringFields:    map[int]func(reflect.Value) string{},
		objectFields:    map[int]func(reflect.Value) string{},
	}

	inf, err := getStructInfo(bt)
	if err != nil {
		panic(fmt.Errorf("getting signal struct info for %s: %w", bt, err))
	}
	for i, field := range inf.StructFields {
		fieldBottom := derefType(field.Type)
		if fieldBottom == reflect.TypeFor[ObjectPath]() {
			sm.objectFields[i] = field.StringGetter()
		} else if fieldBottom.Kind() == reflect.String {
			sm.stringFields[i] = field.StringGetter()
		}
	}

	return &Match{
		signal: value.Just(sm),
	}
}

// MatchAllSignals returns a Match for all signals.
func MatchAllSignals() *Match {
	return &Match{}
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
		kv("path_namespace", p.String())
	}
	if pm, ok := m.property.GetOK(); ok {
		kv("interface", "org.freedesktop.DBus.Properties")
		kv("member", "PropertiesChanged")
		kv("arg0", pm.Interface)
	}

	if sm, ok := m.signal.GetOK(); ok {
		kv("interface", sm.Interface)
		kv("member", sm.Member)

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
		if hdr.Interface != sm.Interface || hdr.Member != sm.Member {
			return false
		}

		for i, want := range m.argStr {
			if got := sm.stringFields[i](body.Elem()); got != want {
				return false
			}
		}
		for i, want := range m.argPath {
			if f := sm.stringFields[i]; f != nil {
				if got := ObjectPath(f(body.Elem())); got != want && !got.IsChildOf(want) {
					return false
				}
			}
			if f := sm.objectFields[i]; f != nil {
				if got := ObjectPath(f(body.Elem())); got != want && !got.IsChildOf(want) {
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

// matchesProperty reports whether the given property change matches
// the filter.
func (m *Match) matchesProperty(hdr *header, prop interfaceMember, body reflect.Value) bool {
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
	if pm.Interface != prop.Interface || pm.Member != prop.Member {
		return false
	}

	return true
}

// Sender restricts the match to a single source Peer.
func (m *Match) Peer(p Peer) *Match {
	m.sender = value.Just(p.Name())
	return m
}

// Object restricts the match to a single source path.
func (m *Match) Object(o ObjectPath) *Match {
	m.objectPrefix = value.Absent[ObjectPath]()
	m.object = value.Just(o.Clean())
	return m
}

// ObjectPrefix restricts the match to sending Objects rooted at the
// given path prefix.
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

// ArgStr restricts the match to signals whose i-th body field is a
// string equal to val.
//
// ArgStr can only be used on signal matches, not property matches.
func (m *Match) ArgStr(i int, val string) *Match {
	sm, ok := m.signal.GetOK()
	if !ok {
		panic(fmt.Errorf("ArgStr applied to property match %s, can only be applied to signal matches", m.property.Get()))
	}
	if sm.stringFields[i] == nil {
		panic(fmt.Errorf("invalid ArgStr match on arg %d, argument is not a string", i))
	}
	if m.argStr == nil {
		m.argStr = map[int]string{}
	}
	m.argStr[i] = val
	return m
}

// ArgPathPrefix restricts the Match to signals whose i-th body field
// is a string or ObjectPath with the given prefix.
//
// ArgPathPrefix can only be used on signal matches, not property
// matches.
func (m *Match) ArgPathPrefix(i int, val ObjectPath) *Match {
	sm, ok := m.signal.GetOK()
	if !ok {
		panic(fmt.Errorf("ArgPathPrefix applied to property match %s, can only be applied to signal matches", m.property.Get()))
	}
	if sm.stringFields[i] == nil && sm.objectFields[i] == nil {
		panic(fmt.Errorf("invalid ArgPathPrefix match on arg %d, argument is not a string or an ObjectPath", i))
	}
	if m.argPath == nil {
		m.argPath = map[int]ObjectPath{}
	}
	m.argPath[i] = val
	return m
}

// Arg0Namespace restricts the Match to signals whose first body field
// is a peer or interface name with the given dot-separated prefix.
//
// Arg0Namespace can only be used on signal matches, not property
// matches.
func (m *Match) Arg0Namespace(val string) *Match {
	sm, ok := m.signal.GetOK()
	if !ok {
		panic(fmt.Errorf("Arg0Namespace applied to property match %s, can only be applied to signal matches", m.property.Get()))
	}
	if sm.stringFields[0] == nil {
		panic(errors.New("invalid Arg0Namespace match, argument 0 is not a string"))
	}
	m.arg0NS = value.Just(val)
	return m
}

func escapeMatchArg(s string) string {
	s = strings.ReplaceAll(s, "'", "'\\''")
	return "'" + s + "'"
}
