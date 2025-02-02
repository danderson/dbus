package dbus

import (
	"context"
	"fmt"
	"net"
	"sync"
)

// ClaimOptions are the options for a [Claim] to a bus name.
type ClaimOptions struct {
	// AllowReplacement is whether to allow another request that sets
	// TryReplace to take over ownership.
	//
	// A claim that gets replaced as the current owner gets moved to
	// the head of the backup queue, or gets dropped from the line of
	// succession entirely if NoQueue is set.
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
	// If ownership of the name cannot be secured when the initial
	// claim is made, or if ownership is later lost due to the effect
	// of AllowReplacement/TryReplace, the claim becomes inactive
	// until a new request is explicitly made with Claim.Request.
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
	conn  *Conn
	watch *Watcher
	name  string

	stop        func() error
	pumpStopped chan struct{}

	// owned by pump goroutine
	owner chan bool
	last  bool
}

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
	w, err := c.Watch()
	if err != nil {
		return nil, err
	}

	_, err = w.Match(MatchNotification[NameAcquired]().ArgStr(0, name))
	if err != nil {
		w.Close()
		return nil, err
	}
	_, err = w.Match(MatchNotification[NameLost]().ArgStr(0, name))
	if err != nil {
		w.Close()
		return nil, err
	}

	ret := &Claim{
		conn:        c,
		watch:       w,
		name:        name,
		pumpStopped: make(chan struct{}),
		owner:       make(chan bool, 1),
		last:        false,
	}
	ret.stop = sync.OnceValue(ret.close)

	ret.send(false)
	if err := ret.Request(opts); err != nil {
		w.Close()
		return nil, err
	}

	if err := c.addClaim(ret); err != nil {
		w.Close()
		return nil, err
	}

	go ret.pump()
	return ret, nil
}

func (c *Conn) addClaim(cl *Claim) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	c.claims.Add(cl)
	return nil
}

func (c *Conn) removeClaim(cl *Claim) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.claims.Remove(cl)
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
//
// Request only returns a non-nil error if sending the updated claim
// request fails. Failure to acquire ownership is not an error.
func (c *Claim) Request(opts ClaimOptions) error {
	var req struct {
		Name  string
		Flags uint32
	}
	req.Name = c.name
	if opts.AllowReplacement {
		req.Flags |= 0x1
	}
	if opts.TryReplace {
		req.Flags |= 0x2
	}
	if opts.NoQueue {
		req.Flags |= 0x4
	}

	var resp uint32
	return c.conn.bus.Interface(ifaceBus).Call(context.Background(), "RequestName", req, &resp)
}

// Close abandons the claim.
//
// If the claim is the current owner of the bus name, ownership is
// lost and may be passed on to another claimant.
func (c *Claim) Close() error {
	return c.stop()
}

func (c *Claim) close() error {
	c.conn.removeClaim(c)

	c.watch.Close()
	<-c.pumpStopped

	var ignore uint32
	return c.conn.bus.Interface(ifaceBus).Call(context.Background(), "ReleaseName", c.name, &ignore)
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
	defer func() {
		if c.last {
			// One final send to report loss of ownership.
			c.send(false)
		}
		close(c.owner)
		close(c.pumpStopped)
	}()
	for sig := range c.watch.Chan() {
		notify := false
		switch v := sig.Body.(type) {
		case *NameAcquired:
			if v.Name != c.name {
				continue
			}
			notify = !c.last
			c.last = true
		case *NameLost:
			if v.Name != c.name {
				continue
			}
			notify = c.last
			c.last = false
		default:
			panic(fmt.Errorf("unexpected signal: %#v", sig))
		}
		if notify {
			c.send(c.last)
		}
	}
}
