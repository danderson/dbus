package dbus

import (
	"context"
	"errors"
	"maps"
	"net"
	"reflect"
	"sync"

	"github.com/creachadair/mds/mapset"
	"github.com/creachadair/mds/queue"
)

const maxWatcherQueue = 20

// A Watcher delivers notifications received from the bus that match
// its filters.
type Watcher struct {
	conn     *Conn
	wakePump chan struct{} // closed to halt the pump

	// owned by the pump goroutine.
	notifications chan *Notification
	pumpStopped   chan struct{}

	mu      sync.Mutex
	closed  bool
	queue   queue.Queue[*Notification]
	matches mapset.Set[*Match]
}

// Notification is a signal or property change received from a bus
// peer.
type Notification struct {
	// Sender is the originator of the notification.
	Sender Interface
	// Name is the name of the signal or changed property.
	Name string
	// Body is the signal payload or property value.
	//
	// For signals, Body a pointer to the struct type that was
	// associated with the signal name using RegisterSignalType, or
	// a pointer to an anonymous struct if no type was registered for
	// the signal.
	//
	// For property changes, Body is a pointer to the struct type that
	// was associated with the property using
	// RegisterPropertyChangeType, or a pointer to an anonymous struct
	// if no type was registered for the property.
	Body any
	// Overflow reports that the watcher discarded some notifications
	// that followed this one, due to the caller not processing
	// delivered notifications fast enough.
	Overflow bool
}

// Watch watches the bus for notifications from other bus
// participants.
//
// A newly created Watcher delivers no notifications. The caller must
// use [Watcher.Match] to specify which signals and property changes
// the Watcher should provide.
func (c *Conn) Watch() (*Watcher, error) {
	w := &Watcher{
		conn:          c,
		notifications: make(chan *Notification),
		wakePump:      make(chan struct{}, 1),
		pumpStopped:   make(chan struct{}),
		matches:       mapset.New[*Match](),
	}

	if err := c.addWatcher(w); err != nil {
		return nil, err
	}
	go w.pump()
	return w, nil
}

func (c *Conn) addWatcher(w *Watcher) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return net.ErrClosed
	}
	c.watchers.Add(w)
	return nil
}

func (c *Conn) removeWatcher(w *Watcher) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.watchers.Remove(w)
}

// Close shuts down the Watcher.
func (w *Watcher) Close() {
	ms, shouldClose := w.clearMatches()
	if !shouldClose {
		return
	}

	close(w.wakePump)
	<-w.pumpStopped

	w.conn.removeWatcher(w)
	for m := range ms {
		w.conn.removeMatch(context.Background(), m)
	}
}

func (w *Watcher) addMatch(m *Match) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return net.ErrClosed
	}
	w.matches.Add(m)
	return nil
}

func (w *Watcher) removeMatch(m *Match) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return false
	}
	delete(w.matches, m)
	return true
}

func (w *Watcher) clearMatches() (mapset.Set[*Match], bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil, false
	}

	ret := w.matches
	w.closed = true
	w.matches = nil
	w.queue.Clear()
	return ret, true
}

// Chan returns the channel on which notifications are delivered.
//
// The caller must drain this channel of new notifications promptly,
// to avoid overflowing the Watcher's receive queue and losing
// notifications. Missing notifications due to an overflow are
// indicated by the Overflow field of the [Notification] that
// immediately precedes the discarded signal(s).
func (w *Watcher) Chan() <-chan *Notification {
	return w.notifications
}

// Match requests delivery of notifications that match the
// specification m.
//
// Matches are additive: a notification is delivered if it matches any
// of the Watcher's match specifications.
//
// If the match is added successfully, the returned remove function
// may be used to remove thee match without affecting other
// matches. Use of remove is optional, and may be ignored if the set
// of matches doesn't need to change for the lifetime of the Watcher.
func (w *Watcher) Match(m *Match) (remove func() error, err error) {
	if err = w.conn.addMatch(context.Background(), m); err != nil {
		return nil, err
	}

	if err = w.addMatch(m); err != nil {
		rmErr := w.conn.removeMatch(context.Background(), m)
		return nil, errors.Join(err, rmErr)
	}

	return func() error {
		if !w.removeMatch(m) {
			return nil
		}
		return w.conn.removeMatch(context.Background(), m)
	}, nil
}

func (w *Watcher) enqueueLocked(n Notification) {
	if w.queue.Len() >= maxWatcherQueue {
		last, _ := w.queue.Peek(-1)
		last.Overflow = true
		return
	}

	w.queue.Add(&n)
	if w.queue.Len() == 1 {
		select {
		case w.wakePump <- struct{}{}:
		default:
		}
	}
}

func (w *Watcher) deliverSignal(sender Interface, hdr *header, body reflect.Value) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}

	want := func() bool {
		for m := range maps.Keys(w.matches) {
			if m.matchesSignal(hdr, body) {
				return true
			}
		}
		return false
	}()
	if !want {
		return
	}

	w.enqueueLocked(Notification{
		Sender: sender,
		Name:   hdr.Member,
		Body:   body.Interface(),
	})
}

func (w *Watcher) deliverProp(sender Interface, hdr *header, prop interfaceMember, value reflect.Value) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return
	}

	want := func() bool {
		for m := range maps.Keys(w.matches) {
			if m.matchesProperty(hdr, prop, value) {
				return true
			}
		}
		return false
	}()
	if !want {
		return
	}

	w.enqueueLocked(Notification{
		Sender: sender,
		Name:   prop.Member,
		Body:   value.Interface(),
	})
}

func (w *Watcher) popNotification() *Notification {
	w.mu.Lock()
	defer w.mu.Unlock()
	ret, _ := w.queue.Pop()
	return ret
}

func (w *Watcher) pump() {
	defer close(w.pumpStopped)
	defer close(w.notifications)
	for {
		n := w.popNotification()
		if n == nil {
			_, ok := <-w.wakePump
			if !ok {
				return
			}
		} else {
		deliver:
			for {
				select {
				case w.notifications <- n:
					break deliver
				case _, ok := <-w.wakePump:
					if !ok {
						return
					}
					continue
				}
			}
		}
	}
}
