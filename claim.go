package dbus

import (
	"context"
	"fmt"
)

// Claim requests ownership of a bus name.
//
// Bus names may have multiple active claims by different clients, but
// only one active owner at a time. The [ClaimOptions] set by each
// claimant determines the owner and rules of succession.
//
// Claiming a name does not guarantee ownership of the name. Callers
// must monitor [Claim.Chan] to find out if and when the name gets
// assigned to them.
func (c *Conn) Claim(name string, opts ClaimOptions) (*Claim, error) {
	ret := &Claim{
		c:           c,
		w:           c.Watch(),
		owner:       make(chan bool, 1),
		name:        name,
		pumpStopped: make(chan struct{}),
		last:        false,
	}
	_, err := ret.w.Match(MatchNotification[NameAcquired]().ArgStr(0, name))
	if err != nil {
		ret.w.Close()
		return nil, err
	}
	_, err = ret.w.Match(MatchNotification[NameLost]().ArgStr(0, name))
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
	// TryReplace to take over ownership.
	AllowReplacement bool
	// TryReplace is whether to attempt to replace the current owner,
	// if the name already has an owner.
	//
	// Replacement is only permitted if the current owner made its
	// claim with the AllowReplacement option set. Otherwise, the
	// request for ownership joins the backup queue or returns an
	// error, depending on the NoQueue setting.
	//
	// TryReplace only takes effect at the moment the request is
	// made. If the replacement attempt fails and later on the owner
	// changes its settings to allow replacement, this queued claim
	// must explicitly request replacement again to take advantage of
	// the change.
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
	return c.c.bus.Interface(ifaceBus).Call(context.Background(), "RequestName", req, &resp)
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
	return c.c.bus.Interface(ifaceBus).Call(context.Background(), "ReleaseName", c.name, &ignore)
}

// Name returns the claim's bus name.
func (c *Claim) Name() string { return c.name }

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
			panic(fmt.Errorf("unexpected signal: %#v", sig))
		}
		c.send(c.last)
	}
}
