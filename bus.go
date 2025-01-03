package dbus

import (
	"context"
	"errors"
	"fmt"
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
