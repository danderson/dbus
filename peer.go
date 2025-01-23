package dbus

import (
	"bytes"
	"context"
)

// Peer is a named bus endpoint.
//
// A Peer may provide access to [Object] and [Interface] values, emit
// [Signal] notifications, or make method calls to other bus
// participants.
type Peer struct {
	c    *Conn
	name string
}

// Conn returns the DBus connection associated with the peer.
func (p Peer) Conn() *Conn { return p.c }

// Name returns the name of the peer.
func (p Peer) Name() string { return p.name }

func (p Peer) String() string {
	if p.name == "" {
		return "<no peer>"
	}
	return p.name
}

// Object returns a named object on the peer.
//
// The returned value is a purely local handle. It does not indicate
// that the peer is providing an object at the requested path.
func (p Peer) Object(path ObjectPath) Object {
	return Object{
		p:    p,
		path: path,
	}
}

// Ping checks that the peer is reachable.
//
// Ping returns a [CallError] if the queried peer does not implement
// the [org.freedesktop.DBus.Peer] interface on its root object. In
// practice, all DBus client implementations implement this interface,
// although they are not strictly required to.
//
// [org.freedesktop.DBus.Peer]: https://dbus.freedesktop.org/doc/dbus-specification.html#standard-interfaces-peer
func (p Peer) Ping(ctx context.Context, opts ...CallOption) error {
	return p.Conn().call(ctx, p.name, "/", "org.freedesktop.DBus.Peer", "Ping", nil, nil, opts...)
}

// PeerIdentity describes the identity of a Peer.
type PeerIdentity struct {
	// UID is the Unix user ID of the peer, or nil if uid information is
	// not available.
	UID *uint32 `dbus:"key=UnixUserID"`
	// GIDs are the Unix group IDs of the peer.
	GIDs []uint32 `dbus:"key=UnixGroupIDs"`
	// PIDFD is a file handle that represents the Peer's
	// process. PIDFD should be preferred over PID, as it is not
	// vulnerable to time-of-check/time-of-use vulnerabilities.
	PIDFD *File `dbus:"key=ProcessFD"`
	// PID is the Unix process ID of the peer, or nil if pid
	// information is not available. Note that PIDs are not unique
	// identities, and therefore are vulnerable to
	// time-of-check/time-of-use attacks.
	PID *uint32 `dbus:"key=ProcessID"`
	// SecurityLabel is the peer's Linux security label, as returned
	// by the SO_PEERSEC Unix socket option.
	//
	// The security label's format is specific to the running "major"
	// LSM (Linux Security Module). DBus does not provide a means to
	// discover which LSM provided this value.
	//
	// On SELinux systems, SecurityLabel is the peer's SELinux
	// context.
	//
	// On Smack systems, SecurityLabel is the Smack label.
	//
	// On AppArmor systems, SecurityLabel is the AppArmor profile name
	// and enforcement mode.
	SecurityLabel []byte `dbus:"key=LinuxSecurityLabel"`

	// Unknown collects identity values provided by the bus that are
	// not known to this library.
	Unknown map[string]Variant `dbus:"vardict"`
}

// Identity returns the peer's identity descriptor.
//
// The returned identity is provided by the bus itself, and guaranteed
// to be accurate (bugs in the bus implementation notwithstanding).
func (p Peer) Identity(ctx context.Context, opts ...CallOption) (PeerIdentity, error) {
	var resp PeerIdentity
	if err := p.Conn().bus.Call(ctx, "GetConnectionCredentials", p.name, &resp, opts...); err != nil {
		return PeerIdentity{}, err
	}
	// The SELinux security context is reported with a trailing null
	// byte. Remove it, it's just a weird historical artifact.
	resp.SecurityLabel, _ = bytes.CutSuffix(resp.SecurityLabel, []byte("\x00"))
	return resp, nil
}

// UID returns the Unix user ID for the peer, if available.
//
// Deprecated: use [Peer.Identity] instead, which returns more
// complete identity information.
func (p Peer) UID(ctx context.Context, opts ...CallOption) (uint32, error) {
	var uid uint32
	if err := p.Conn().bus.Call(ctx, "GetConnectionUnixUser", p.name, &uid, opts...); err != nil {
		return 0, err
	}
	return uid, nil
}

// PID returns the Unix process ID for the peer, if available.
//
// PIDs are vulnerable to time-of-check/time-of-use attacks, and
// should not be used to make authentication or authorization
// decisions.
//
// Deprecated: use [Peer.Identity] instead, which returns more
// complete identity information. In particular, when available, the
// PIDFD field of [PeerIdentity] provides a more robust handle for the
// peer's process that is not vulnerable to time-of-check/time-of-use
// attacks.
func (p Peer) PID(ctx context.Context, opts ...CallOption) (uint32, error) {
	var pid uint32
	if err := p.Conn().bus.Call(ctx, "GetConnectionUnixProcessID", p.name, &pid, opts...); err != nil {
		return 0, err
	}
	return pid, nil
}

// Exists reports whether the peer is currently present on the bus.
//
// Exists does not take activatable services into account. This means
// that Exists may report false for a Peer that is started on demand
// when communicated with.
//
// Exists is intended for debugging only. In particular, you should
// not use Exists to see if a peer exists before communicating with
// it. Instead, verify the existence of the peer and the objects and
// interfaces you need by attempting to use them, and handling errors
// appropriately.
func (p Peer) Exists(ctx context.Context, opts ...CallOption) (bool, error) {
	var exists bool
	if err := p.Conn().bus.Call(ctx, "NameHasOwner", p.name, &exists, opts...); err != nil {
		return false, err
	}
	return exists, nil
}

// Owner returns the current owner of this peer's name.
//
// If the Peer is a handle to a peer's unique connection name (like
// ":1.42"), Owner returns the same Peer. If the Peer is a well-known
// bus name (like "org.freedesktop.DBus"), Owner returns the Peer for
// the current owner's unique connection name.
func (p Peer) Owner(ctx context.Context, opts ...CallOption) (Peer, error) {
	var name string
	if err := p.Conn().bus.Call(ctx, "GetNameOwner", p.name, &name, opts...); err != nil {
		return Peer{}, err
	}
	return p.Conn().Peer(name), nil
}

// QueuedOwners returns the list of peers that have requested
// ownership of this peer's name, but do not currently own the
// name. To retrieve the current owner, use [Peer.Owner].
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
