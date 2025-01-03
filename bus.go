package dbus

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/creachadair/mds/mapset"
	"github.com/danderson/dbus/fragments"
)

type NameRequestFlags byte

const (
	NameRequestAllowReplacement NameRequestFlags = 1 << iota
	NameRequestReplace
	NameRequestNoQueue
)

func (c *Conn) RequestName(ctx context.Context, name string, flags NameRequestFlags, opts ...CallOption) (isPrimaryOwner bool, err error) {
	var resp uint32
	req := struct {
		Name  string
		Flags uint32
	}{name, uint32(flags)}
	if err := c.bus.Call(ctx, "RequestName", req, &resp, opts...); err != nil {
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

// Not implemented:
//  - StartServiceByName, deprecated in favor of auto-start.
//  - UpdateActivationEnvironment, so locked down you can't really do
//    much with it any more, and should really be leaving environment
//    stuff to systemd anyway.
//  - GetAdtAuditSessionData, Solaris-only and so weird even the spec
//    doesn't know wtf it's for.
//  - GetConnectionSELinuxSecurityContext, deprecated in favor
//    of GetConnectionCredentials.
//  - GetMachineID: who cares it's a single computer bus I don't care
//    what the spec thinks
//
// TODO:
//  - AddMatch/RemoveMatch: should be internal only, behind a nicer
//    signals monitoring API.

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
