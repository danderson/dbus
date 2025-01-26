package dbus

import (
	"context"
	"errors"
	"maps"

	"github.com/creachadair/mds/mapset"
	"github.com/danderson/dbus/fragments"
)

const (
	ifaceBus        = "org.freedesktop.DBus"
	ifacePeer       = "org.freedesktop.DBus.Peer"
	ifaceIntrospect = "org.freedesktop.DBus.Introspectable"
	ifaceObjects    = "org.freedesktop.DBus.ObjectManager"
	ifaceProps      = "org.freedesktop.DBus.Properties"
)

// Peers returns a list of peers currently connected to the bus.
func (c *Conn) Peers(ctx context.Context) ([]Peer, error) {
	var names []string
	if err := c.bus.Interface(ifaceBus).Call(ctx, "ListNames", nil, &names); err != nil {
		return nil, err
	}
	ret := make([]Peer, len(names))
	for i, n := range names {
		ret[i] = c.Peer(n)
	}
	return ret, nil
}

// ActivatablePeers returns a list of activatable peers.
//
// An activatable Peer is started automatically when a request is sent
// to it, and may shut down when idle.
func (c *Conn) ActivatablePeers(ctx context.Context) ([]Peer, error) {
	var names []string
	if err := c.bus.Interface(ifaceBus).Call(ctx, "ListActivatableNames", nil, &names); err != nil {
		return nil, err
	}
	ret := make([]Peer, len(names))
	for i, n := range names {
		ret[i] = c.Peer(n)
	}
	return ret, nil
}

// BusID returns the globally unique ID of the bus to which the Conn
// is connected.
func (c *Conn) BusID(ctx context.Context) (string, error) {
	var id string
	if err := c.bus.Interface(ifaceBus).Call(ctx, "GetId", nil, &id); err != nil {
		return "", err
	}
	return id, nil
}

// Features returns a list of strings describing the optional features
// that the bus supports.
func (c *Conn) Features(ctx context.Context) ([]string, error) {
	var features []string
	if err := c.bus.Interface(ifaceBus).GetProperty(ctx, "Features", &features); err != nil {
		return nil, err
	}
	return features, nil
}

func (c *Conn) addMatch(ctx context.Context, m *Match) error {
	rule := m.filterString()
	return c.bus.Interface(ifaceBus).Call(ctx, "AddMatch", rule, nil)
}

func (c *Conn) removeMatch(ctx context.Context, m *Match) error {
	rule := m.filterString()
	return c.bus.Interface(ifaceBus).Call(ctx, "RemoveMatch", rule, nil)
}

// NameOwnerChanged signals that a name has changed owners.
//
// It corresponds to the [org.freedesktop.DBus.NameOwnerChanged] signal.
//
// [org.freedesktop.DBus.NameOwnerChanged]: https://dbus.freedesktop.org/doc/dbus-specification.html#bus-messages-name-owner-changed
type NameOwnerChanged struct {
	// Name is the bus name whose ownership has changed.
	Name string
	// Prev is the previous owner of Name, or nil if Name has just
	// been created.
	Prev *Peer
	// New is the current owner of Name, or nil if Name is defunct.
	New *Peer
}

func (s *NameOwnerChanged) SignatureDBus() Signature { return mustParseSignature("sss") }

func (s *NameOwnerChanged) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var body struct {
		Name, Prev, New string
	}
	if err := d.Value(ctx, &body); err != nil {
		return err
	}

	emitter, ok := ContextEmitter(ctx)
	if !ok {
		return errors.New("can't unmarshal NameOwnerChanged signal, no emitter in context")
	}

	s.Name = body.Name
	if body.Prev != "" {
		p := emitter.Conn().Peer(body.Prev)
		s.Prev = &p
	}
	if body.New != "" {
		n := emitter.Conn().Peer(body.New)
		s.New = &n
	}

	return nil
}

// NameLost signals to the receiving client that it has lost ownership
// of a bus name.
//
// It corresponds to the [org.freedesktop.DBus.NameLost] signal.
//
// [org.freedesktop.DBus.NameLost]: https://dbus.freedesktop.org/doc/dbus-specification.html#bus-messages-name-lost
type NameLost struct {
	Name string
}

// NameAcquired signals to the receiving client that it has gained
// ownership of a bus name.
//
// It corresponds to the [org.freedesktop.DBus.NameAcquired] signal.
//
// [org.freedesktop.DBus.NameAcquired]: https://dbus.freedesktop.org/doc/dbus-specification.html#bus-messages-name-acquired
type NameAcquired struct {
	Name string
}

// ActivatableServicesChanged signals that the list of activatable
// peers has changed. Use [Conn.ActivatablePeers] to obtain an updated
// list.
//
// It corresponds to the
// [org.freedesktop.DBus.ActivatableServicesChanged] signal.
//
// [org.freedesktop.DBus.ActivatableServicesChanged]: https://dbus.freedesktop.org/doc/dbus-specification.html#bus-messages-activatable-services-changed
type ActivatableServicesChanged struct{}

// PropertiesChanged signals that some of the sender's properties have
// changed.
//
// It corresponds to the
// [org.freedesktop.DBus.Properties.PropertiesChanged] signal.
//
// [org.freedesktop.DBus.Properties.PropertiesChanged]: https://dbus.freedesktop.org/doc/dbus-specification.html#standard-interfaces-properties
type PropertiesChanged struct {
	// Interface is the DBus interface whose properties have changed.
	Interface Interface
	// Changed lists changed property values, keyed by property name.
	Changed map[string]any
	// Invalidated lists the names of properties that have changed,
	// but whose updated value was not broadcast. If desired,
	// [Interface.GetProperty] may be used to read the updated value.
	Invalidated mapset.Set[string]
}

func (s *PropertiesChanged) SignatureDBus() Signature { return mustParseSignature("sa{sv}as") }

func (s *PropertiesChanged) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var body struct {
		Interface   string
		Changed     map[string]any
		Invalidated []string
	}
	if err := d.Value(ctx, &body); err != nil {
		return err
	}

	emitter, ok := ContextEmitter(ctx)
	if !ok {
		return errors.New("can't unmarshal PropertiesChanged signal, no emitter in context")
	}

	s.Interface = emitter.Object().Interface(body.Interface)
	s.Changed = body.Changed
	s.Invalidated = mapset.New(body.Invalidated...)

	return nil
}

// InterfacesAdded signals that an object is offering new interfaces
// for use.
//
// It corresponds to the
// [org.freedesktop.DBus.ObjectManager.InterfacesAdded] signal.
//
// [org.freedesktop.DBus.ObjectManager.InterfacesAdded]: https://dbus.freedesktop.org/doc/dbus-specification.html#standard-interfaces-objectmanager
type InterfacesAdded struct {
	Object     Object
	Interfaces []Interface
}

func (s *InterfacesAdded) SignatureDBus() Signature { return mustParseSignature("oa{sa{sv}}") }

func (s *InterfacesAdded) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var body struct {
		Path        ObjectPath
		IfsAndProps map[string]map[string]any
	}
	if err := d.Value(ctx, &body); err != nil {
		return err
	}

	emitter, ok := ContextEmitter(ctx)
	if !ok {
		return errors.New("can't unmarshal InterfacesAdded signal, no emitter in context")
	}

	// TODO: check path is a child of iface.Object()
	s.Object = emitter.Peer().Object(body.Path)
	s.Interfaces = s.Interfaces[:0]
	for k := range maps.Keys(body.IfsAndProps) {
		s.Interfaces = append(s.Interfaces, s.Object.Interface(k))
	}

	return nil
}

// InterfacesAdded signals that an object has ceased to offer one or
// more interfaces for use.
//
// It corresponds to the
// [org.freedesktop.DBus.ObjectManager.InterfacesRemoved] signal.
//
// [org.freedesktop.DBus.ObjectManager.InterfacesRemoved]: https://dbus.freedesktop.org/doc/dbus-specification.html#standard-interfaces-objectmanager
type InterfacesRemoved struct {
	Object     Object
	Interfaces []Interface
}

func (s *InterfacesRemoved) SignatureDBus() Signature { return mustParseSignature("oa{sa{sv}}") }

func (s *InterfacesRemoved) UnmarshalDBus(ctx context.Context, d *fragments.Decoder) error {
	var body struct {
		Path ObjectPath
		Ifs  []string
	}
	if err := d.Value(ctx, &body); err != nil {
		return err
	}

	emitter, ok := ContextEmitter(ctx)
	if !ok {
		return errors.New("can't unmarshal InterfacesRemoved signal, no emitter in context")
	}

	s.Object = emitter.Peer().Object(body.Path)
	s.Interfaces = s.Interfaces[:0]
	for _, iface := range body.Ifs {
		s.Interfaces = append(s.Interfaces, s.Object.Interface(iface))
	}
	return nil
}
