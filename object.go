package dbus

import (
	"context"
	"encoding/xml"
	"fmt"
	"maps"
)

// Object is an object exposed by a [Peer].
type Object struct {
	p    Peer
	path ObjectPath
}

// Conn returns the DBus connection associated with the object.
func (o Object) Conn() *Conn { return o.p.Conn() }

// Peer returns the peer that is exposing the object.
func (o Object) Peer() Peer { return o.p }

// Path returns the object's path.
func (o Object) Path() ObjectPath { return o.path }

func (o Object) String() string {
	if o.path == "" {
		return fmt.Sprintf("%s:<no object>", o.Peer())
	}
	return fmt.Sprintf("%s:%s", o.Peer(), o.path)
}

// Interface returns a named interface on the object.
//
// The returned value is a purely local handle. It does not indicate
// that the object supports the requested interface.
func (o Object) Interface(name string) Interface {
	return Interface{
		o:    o,
		name: name,
	}
}

// Introspect returns the object's description of the interfaces it
// implements.
//
// Note that while DBus objects are generally well behaved, this
// description is not verified or enforced by the bus, and may not
// accurately reflect the object's implementation.
//
// Introspect returns a [CallError] if the queried object does not
// implement the [org.freedesktop.DBus.Introspectable] interface.
//
// [org.freedesktop.DBus.Introspectable]: https://dbus.freedesktop.org/doc/dbus-specification.html#standard-interfaces-introspectable
func (o Object) Introspect(ctx context.Context, opts ...CallOption) (*ObjectDescription, error) {
	var resp string
	if err := o.Conn().call(ctx, o.p.name, o.path, "org.freedesktop.DBus.Introspectable", "Introspect", nil, &resp, opts...); err != nil {
		return nil, err
	}
	var ret ObjectDescription
	if err := xml.Unmarshal([]byte(resp), &ret); err != nil {
		return nil, err
	}
	return &ret, nil
}

// ManagedObjects returns the children of the current Object, and the
// interfaces they implement.
//
// ManagedObjects returns a [CallError] if the queried object does not
// implement the [org.freedesktop.DBus.ObjectManager] interface.
//
// [org.freedesktop.DBus.ObjectManager]: https://dbus.freedesktop.org/doc/dbus-specification.html#standard-interfaces-objectmanager
func (o Object) ManagedObjects(ctx context.Context, opts ...CallOption) (map[Object][]Interface, error) {
	// object path -> interface name -> map[property name]value
	var resp map[ObjectPath]map[string]map[string]Variant
	err := o.Conn().call(ctx, o.p.name, o.path, "org.freedesktop.DBus.ObjectManager", "GetManagedObjects", nil, &resp, opts...)
	if err != nil {
		return nil, err
	}
	ret := make(map[Object][]Interface, len(resp))
	for path, ifs := range resp {
		// TODO: validate that path is a subpath of the current object
		child := o.Peer().Object(path)
		ifaces := make([]Interface, 0, len(ifs))
		for ifname := range maps.Keys(ifs) {
			ifaces = append(ifaces, child.Interface(ifname))
		}
		ret[o.Peer().Object(path)] = ifaces
	}
	return ret, nil
}
