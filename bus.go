package dbus

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"

	"github.com/creachadair/mds/mapset"
	"github.com/danderson/dbus/fragments"
)

// NameRequest is a request to take ownership of a DBus [Peer]
// name. See [Conn.RequestName] for detailed behavior.
type NameRequest struct {
	// Name is the bus name to request.
	Name string
	// ReplaceCurrent is whether to attempt to replace the current
	// primary owner of Name, if one exists. Replacement is only
	// possible if the current primary owner requested the name with
	// AllowReplacement set.
	ReplaceCurrent bool
	// NoQueue, if set, causes RequestName to return an error if
	// primary ownership of Name cannot be granted.
	NoQueue bool
	// AllowReplacement is whether to allow the requestor to be
	// replaced as primary owner, if another Peer requests the name
	// with ReplaceCurrent set.
	AllowReplacement bool
}

// RequestName asks the bus to assign an additional name to the Conn.
//
// A bus name has a single owner which receives DBus traffic for that
// name, and a queue of "backup" owners that are willing to take over
// should the current owner disconnect or abandon the name.
//
// If there are no other claims to the requested name, the Conn
// becomes the name's owner, and RequestName returns (true, nil). The
// options in [NameRequest] control behavior when there are multiple
// claims to the requested name.
//
// By default, if the name already has an owner, RequestName adds Conn
// to the queue of backup owners and returns (false, nil). The bus
// will send the [NameAcquired] signal when Conn becomes the owner of
// the name. If ownership is taken away, the bus indicates this with
// the [NameLost] signal and places Conn back in the queue of backup
// owners.
//
// [NameRequest.NoQueue] indicates that Conn should never join the
// backup queue for a name. RequestName returns an error if it cannot
// immediately become the owner. If ownership is later lost, the bus
// indicates this with the [NameLost] signal and forgets that Conn
// made any claim to the name until it requests it anew.
//
// If [NameRequest.ReplaceCurrent] is set, RequestName attempts to
// skip the queue and forcibly take ownership of the name from its
// current owner. The current owner must have set
// [NameRequest.AllowReplacement] in its own request, otherwise the
// name request is handled as if ReplaceCurrent wasn't set.
//
// [NameRequest.AllowReplacement] controls whether another client
// using [NameRequest.ReplaceCurrent] can take ownership away from
// this Conn. If set, the caller should watch the [NameLost] signal to
// detect loss of ownership.
//
// When Conn is the current owner, RequestName can be used to update
// the desired values for [NameRequest.AllowReplacement] and
// [NameRequest.NoQueue] settings. Changing these values may result in
// loss of ownership.
func (c *Conn) RequestName(ctx context.Context, req NameRequest, opts ...CallOption) (isPrimaryOwner bool, err error) {
	var resp uint32
	r := struct {
		Name  string
		Flags uint32
	}{
		Name: req.Name,
	}
	if req.AllowReplacement {
		r.Flags |= 0x1
	}
	if req.ReplaceCurrent {
		r.Flags |= 0x2
	}
	if req.NoQueue {
		r.Flags |= 0x4
	}

	if err := c.bus.Call(ctx, "RequestName", r, &resp, opts...); err != nil {
		return false, err
	}
	switch resp {
	case 1:
		// Became primary owner.
		return true, nil
	case 2:
		// Placed in queue, but not primary.
		return false, nil
	case 3:
		// Couldn't become primary owner, and request flags asked to
		// not queue.
		return false, errors.New("requested name not available")
	case 4:
		// Already the primary owner.
		return true, nil
	default:
		return false, fmt.Errorf("unknown response code %d to RequestName", resp)
	}
}

func (c *Conn) ReleaseName(ctx context.Context, name string, opts ...CallOption) error {
	var ignore uint32
	if err := c.bus.Call(ctx, "ReleaseName", name, &ignore, opts...); err != nil {
		return err
	}
	return nil
}

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

func (c *Conn) BusID(ctx context.Context, opts ...CallOption) (string, error) {
	var id string
	if err := c.bus.Call(ctx, "GetId", nil, &id, opts...); err != nil {
		return "", err
	}
	return id, nil
}

func (c *Conn) Features(ctx context.Context, opts ...CallOption) ([]string, error) {
	var features []string
	if err := c.bus.GetProperty(ctx, "Features", &features, opts...); err != nil {
		return nil, err
	}
	return features, nil
}

func (c *Conn) addMatch(ctx context.Context, m *Match) error {
	rule := m.filterString()
	log.Println(m, rule)
	return c.bus.Call(ctx, "AddMatch", rule, nil)
}

func (c *Conn) removeMatch(ctx context.Context, m *Match) error {
	rule := m.filterString()
	return c.bus.Call(ctx, "RemoveMatch", rule, nil)
}

type NameOwnerChanged struct {
	Name string
	Prev *Peer
	New  *Peer
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
