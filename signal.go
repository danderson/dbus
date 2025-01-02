package dbus

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"sync"

	"github.com/creachadair/mds/mapset"
	"github.com/danderson/dbus/fragments"
)

var (
	signalsMu   sync.Mutex
	signalTypes = map[string]reflect.Type{
		"org.freedesktop.DBus.NameOwnerChanged":                reflect.TypeFor[NameOwnerChanged](),
		"org.freedesktop.DBus.NameLost":                        reflect.TypeFor[NameLost](),
		"org.freedesktop.DBus.NameAcquired":                    reflect.TypeFor[NameAcquired](),
		"org.freedesktop.DBus.ActivatableServicesChanged":      reflect.TypeFor[ActivatableServicesChanged](),
		"org.freedesktop.DBus.Properties.PropertiesChanged":    reflect.TypeFor[PropertiesChanged](),
		"org.freedesktop.DBus.ObjectManager.InterfacesAdded":   reflect.TypeFor[InterfacesAdded](),
		"org.freedesktop.DBus.ObjectManager.InterfacesRemoved": reflect.TypeFor[InterfacesRemoved](),
	}
)

func RegisterSignalType[T any](interfaceName, signalName string) {
	name := interfaceName + "." + signalName
	t := reflect.TypeFor[T]()
	if _, err := SignatureFor[T](); err != nil {
		panic(fmt.Errorf("cannot use %s as dbus type for signal %s: %w", t, name, err))
	}
	signalsMu.Lock()
	defer signalsMu.Unlock()
	if prev := signalTypes[name]; t != nil {
		panic(fmt.Errorf("duplicate signal type registration for %s, existing registration %s", name, prev))
	}
	signalTypes[name] = t
}

func typeForSignal(interfaceName, signalName string, sig Signature) reflect.Type {
	name := interfaceName + "." + signalName
	signalsMu.Lock()
	defer signalsMu.Unlock()
	if ret := signalTypes[name]; ret != nil {
		return ret
	}
	if !sig.IsZero() {
		return sig.Type()
	}
	return nil
}

type NameOwnerChanged struct {
	Name string
	Prev *Peer
	New  *Peer
}

func (s *NameOwnerChanged) AlignDBus() int { return 8 }

func (s *NameOwnerChanged) SignatureDBus() Signature { return mustParseSignature("sss") }

func (s *NameOwnerChanged) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var body struct {
		Name, Prev, New string
	}
	if err := d.Value(ctx, &body); err != nil {
		return err
	}

	sender, ok := ContextSender(ctx)
	if !ok {
		return errors.New("can't unmarshal NameOwnerChanged signal, no sender in context")
	}

	s.Name = body.Name
	if body.Prev != "" {
		p := sender.Conn().Peer(body.Prev)
		s.Prev = &p
	}
	if body.New != "" {
		n := sender.Conn().Peer(body.New)
		s.New = &n
	}

	return nil
}

type NameLost struct {
	Name string
}

type NameAcquired struct {
	Name string
}

type ActivatableServicesChanged struct{}

type PropertiesChanged struct {
	Interface   Interface
	Changed     map[string]any
	Invalidated mapset.Set[string]
}

func (s *PropertiesChanged) AlignDBus() int { return 8 }

func (s *PropertiesChanged) SignatureDBus() Signature { return mustParseSignature("sa{sv}as") }

func (s *PropertiesChanged) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var body struct {
		Interface   string
		Changed     map[string]Variant
		Invalidated []string
	}
	if err := d.Value(ctx, &body); err != nil {
		return err
	}

	sender, ok := ContextSender(ctx)
	if !ok {
		return errors.New("can't unmarshal PropertiesChanged signal, no sender in context")
	}

	s.Interface = sender.Object().Interface(body.Interface)
	s.Changed = map[string]any{}
	for k, v := range body.Changed {
		s.Changed[k] = v.Value
	}
	s.Invalidated = mapset.New(body.Invalidated...)

	return nil
}

type InterfacesAdded struct {
	Object     Object
	Interfaces []Interface
}

func (s *InterfacesAdded) AlignDBus() int { return 8 }

func (s *InterfacesAdded) SignatureDBus() Signature { return mustParseSignature("oa{sa{sv}}") }

func (s *InterfacesAdded) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var body struct {
		Path        ObjectPath
		IfsAndProps map[string]map[string]Variant
	}
	if err := d.Value(ctx, &body); err != nil {
		return err
	}

	sender, ok := ContextSender(ctx)
	if !ok {
		return errors.New("can't unmarshal InterfacesAdded signal, no sender in context")
	}

	// TODO: check path is a child of iface.Object()
	s.Object = sender.Peer().Object(body.Path)
	s.Interfaces = s.Interfaces[:0]
	for k := range maps.Keys(body.IfsAndProps) {
		s.Interfaces = append(s.Interfaces, s.Object.Interface(k))
	}

	return nil
}

type InterfacesRemoved struct {
	Object     Object
	Interfaces []Interface
}

func (s *InterfacesRemoved) AlignDBus() int { return 8 }

func (s *InterfacesRemoved) SignatureDBus() Signature { return mustParseSignature("oa{sa{sv}}") }

func (s *InterfacesRemoved) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var body struct {
		Path ObjectPath
		Ifs  []string
	}
	if err := d.Value(ctx, &body); err != nil {
		return err
	}

	sender, ok := ContextSender(ctx)
	if !ok {
		return errors.New("can't unmarshal InterfacesRemoved signal, no sender in context")
	}

	s.Object = sender.Peer().Object(body.Path)
	s.Interfaces = s.Interfaces[:0]
	for _, iface := range body.Ifs {
		s.Interfaces = append(s.Interfaces, s.Object.Interface(iface))
	}
	return nil
}
