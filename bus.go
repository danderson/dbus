package dbus

import (
	"context"
	"errors"
	"maps"

	"github.com/creachadair/mds/mapset"
	"github.com/danderson/dbus/fragments"
)

// Claim creates a [Claim] for ownership of a bus name.
//
// Bus names may have multiple claims by different clients, in which
// case behavior is determined by the [ClaimOptions] set by each
// claimant.
//
// Callers should monitor [Claim.Chan] to find out if and when the
// name gets assigned to them.
func (c *Conn) Claim(name string, opts ClaimOptions) (*Claim, error) {
	ret := &Claim{
		c:           c,
		w:           c.Watch(),
		owner:       make(chan bool, 1),
		name:        name,
		pumpStopped: make(chan struct{}),
		last:        false,
	}
	_, err := ret.w.Match(NewMatch().Signal(NameAcquired{}).ArgStr(0, name))
	if err != nil {
		ret.w.Close()
		return nil, err
	}
	_, err = ret.w.Match(NewMatch().Signal(NameLost{}).ArgStr(0, name))
	if err != nil {
		ret.w.Close()
		return nil, err
	}

	if err := ret.Request(opts); err != nil {
		ret.w.Close()
		return nil, err
	}

	go ret.pump()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.claims.Add(ret)
	return ret, nil
}

// ClaimOptions are the options for a [Claim] to a bus name.
type ClaimOptions struct {
	// AllowReplacement is whether to allow another request that sets
	// TryReplace to take over ownership from this claim.
	AllowReplacement bool
	// TryReplace is whether to attempt to replace the current owner
	// of Name, if the name already has an owner.
	//
	// Replacement is only permitted if the current primary owner
	// requested the name with AllowReplacement set. Otherwise, the
	// request for ownership joins the backup queue or returns an
	// error, depending on the NoQueue setting.
	//
	// Note that TryReplace only takes effect at the moment the
	// request is made. If the attempt fails and this claim joins the
	// backup queue, and later on the owner changes its settings to
	// allow replacement, this queued claim must explicitly repeat its
	// request with TryReplace set to take advantage of it.
	TryReplace bool
	// NoQueue, if set, causes this claim to never join the backup
	// queue for any reason.
	//
	// If ownership of the name cannot be secured when the Claim is
	// created, creation fails with an error.
	//
	// If ownership is secured and a later event causes loss of
	// ownership (such as this claim setting AllowReplacement, and
	// another client making a claim with TryReplace), the claim
	// becomes inactive until a new request is explicitly made with
	// Claim.Request.
	NoQueue bool
}

// Claim is a claim to ownership of a bus name.
//
// Multiple DBus clients may claim ownership of the same name. The bus
// tracks a single current owner, as well as a queue of other
// claimants that are eligible to succeed the current owner.
//
// The exact interaction of multiple different claims to a name
// depends on the [ClaimOptions] set by each claimant.
type Claim struct {
	c     *Conn
	w     *Watcher
	owner chan bool
	name  string

	pumpStopped chan struct{}

	last bool
	opts ClaimOptions
}

// Request makes a new request to the bus for the claimed name.
//
// If this Claim is the current owner, Request updates the
// AllowReplacement and NoQueue settings without relinquishing
// ownership (although setting AllowReplacement may enable another
// client to take over the claim).
//
// If this claim is not the current owner, the bus considers this
// claim anew with the updated [ClaimOptions], as if this client were
// making a claim for the first time.
func (c *Claim) Request(opts ClaimOptions) error {
	c.opts = opts

	var req struct {
		Name  string
		Flags uint32
	}
	req.Name = c.name
	if c.opts.AllowReplacement {
		req.Flags |= 0x1
	}
	if c.opts.TryReplace {
		req.Flags |= 0x2
	}
	if c.opts.NoQueue {
		req.Flags |= 0x4
	}

	var resp uint32
	return c.c.bus.Call(context.Background(), "RequestName", req, &resp)
}

// Close abandons the claim.
//
// If the claim is the current owner of the bus name, ownership is
// lost and may be passed on to another claimant.
func (c *Claim) Close() error {
	select {
	case <-c.pumpStopped:
		return nil
	default:
	}

	c.w.Close()
	<-c.pumpStopped

	// One final send to report loss of ownership, before closing the
	// chan
	c.send(false)
	close(c.owner)

	var ignore uint32
	return c.c.bus.Call(context.Background(), "ReleaseName", c.name, &ignore)
}

// Chan returns a channel that reports whether this claim is the
// current owner of the bus name.
func (c *Claim) Chan() <-chan bool { return c.owner }

func (c *Claim) send(isOwner bool) {
	select {
	case c.owner <- isOwner:
	case <-c.owner:
		c.owner <- isOwner
	}
}

func (c *Claim) pump() {
	defer close(c.pumpStopped)
	for sig := range c.w.Chan() {
		switch v := sig.Body.(type) {
		case *NameAcquired:
			if v.Name != c.name {
				continue
			}
			c.last = true
		case *NameLost:
			if v.Name != c.name {
				continue
			}
			c.last = false
		default:
			panic("unexpected signal")
		}
		c.send(c.last)
	}
}

// Peers returns a list of peers currently connected to the bus.
func (c *Conn) Peers(ctx context.Context, opts ...CallOption) ([]Peer, error) {
	var names []string
	if err := c.bus.Call(ctx, "ListNames", nil, &names, opts...); err != nil {
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
// An activatable [Peer] is started automatically when a request is
// sent to it, and may shut down when idle.
func (c *Conn) ActivatablePeers(ctx context.Context, opts ...CallOption) ([]Peer, error) {
	var names []string
	if err := c.bus.Call(ctx, "ListActivatableNames", nil, &names, opts...); err != nil {
		return nil, err
	}
	ret := make([]Peer, len(names))
	for i, n := range names {
		ret[i] = c.Peer(n)
	}
	return ret, nil
}

// BusID returns the unique globally unique ID of the bus to which the
// Conn is connected.
func (c *Conn) BusID(ctx context.Context, opts ...CallOption) (string, error) {
	var id string
	if err := c.bus.Call(ctx, "GetId", nil, &id, opts...); err != nil {
		return "", err
	}
	return id, nil
}

// Features returns a list of strings describing the optional features
// that the bus supports.
func (c *Conn) Features(ctx context.Context, opts ...CallOption) ([]string, error) {
	var features []string
	if err := c.bus.GetProperty(ctx, "Features", &features, opts...); err != nil {
		return nil, err
	}
	return features, nil
}

func (c *Conn) addMatch(ctx context.Context, m *Match) error {
	rule := m.filterString()
	return c.bus.Call(ctx, "AddMatch", rule, nil)
}

func (c *Conn) removeMatch(ctx context.Context, m *Match) error {
	rule := m.filterString()
	return c.bus.Call(ctx, "RemoveMatch", rule, nil)
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

func (s *NameOwnerChanged) IsDBusStruct() bool { return true }

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

func (s *PropertiesChanged) IsDBusStruct() bool { return true }

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

func (s *InterfacesAdded) IsDBusStruct() bool { return true }

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

func (s *InterfacesRemoved) IsDBusStruct() bool { return true }

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
