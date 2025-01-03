package dbus

import (
	"context"
)

type Peer struct {
	c    *Conn
	name string
}

func (p Peer) Ping(ctx context.Context, opts ...CallOption) error {
	return p.Conn().call(ctx, p.name, "/", "org.freedesktop.DBus.Peer", "Ping", nil, nil, opts...)
}

func (p Peer) Conn() *Conn  { return p.c }
func (p Peer) Name() string { return p.name }

func (p Peer) String() string {
	if p.c == nil {
		return "<no peer>"
	}
	return p.name
}

func (p Peer) Object(path ObjectPath) Object {
	return Object{
		p:    p,
		path: path,
	}
}

type PeerIdentity struct {
	UID           uint32   `dbus:"key=UnixUserID"`
	GIDs          []uint32 `dbus:"key=UnixGroupIDs"`
	PIDFD         File     `dbus:"key=ProcessFD"`
	PID           uint32   `dbus:"key=ProcessID"`
	SecurityLabel []byte   `dbus:"key=LinuxSecurityLabel"`

	Unknown map[string]Variant `dbus:"vardict"`
}

func (p Peer) Identity(ctx context.Context, opts ...CallOption) (PeerIdentity, error) {
	var resp PeerIdentity
	if err := p.Conn().bus.Call(ctx, "GetConnectionCredentials", p.name, &resp, opts...); err != nil {
		return PeerIdentity{}, err
	}
	return resp, nil
}

func (p Peer) UID(ctx context.Context, opts ...CallOption) (uint32, error) {
	var uid uint32
	if err := p.Conn().bus.Call(ctx, "GetConnectionUnixUser", p.name, &uid, opts...); err != nil {
		return 0, err
	}
	return uid, nil
}

func (p Peer) PID(ctx context.Context, opts ...CallOption) (uint32, error) {
	var pid uint32
	if err := p.Conn().bus.Call(ctx, "GetConnectionUnixProcessID", p.name, &pid, opts...); err != nil {
		return 0, err
	}
	return pid, nil
}

func (p Peer) Exists(ctx context.Context, opts ...CallOption) (bool, error) {
	var exists bool
	if err := p.Conn().bus.Call(ctx, "NameHasOwner", p.name, &exists, opts...); err != nil {
		return false, err
	}
	return exists, nil
}

func (p Peer) Owner(ctx context.Context, opts ...CallOption) (Peer, error) {
	var name string
	if err := p.Conn().bus.Call(ctx, "GetNameOwner", p.name, &name, opts...); err != nil {
		return Peer{}, err
	}
	return p.Conn().Peer(name), nil
}

func (p Peer) QueuedOwners(ctx context.Context, opts ...CallOption) ([]Peer, error) {
	var names []string
	if err := p.Conn().bus.Call(ctx, "ListQueuedOwners", p.name, &names, opts...); err != nil {
		return nil, err
	}
	ret := make([]Peer, len(names))
	for i, n := range names {
		ret[i] = p.Conn().Peer(n)
	}
	return ret, nil
}
