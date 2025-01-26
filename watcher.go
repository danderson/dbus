package dbus

import (
	"context"
	"maps"
	"reflect"
	"sync"

	"github.com/creachadair/mds/mapset"
	"github.com/creachadair/mds/queue"
)

const maxWatcherQueue = 20

// Watch watches the bus for notifications from other bus
// participants.
//
// A newly created Watcher delivers no notifications. The caller must
// use [Watcher.Match] to specify which signals and property changes
// the Watcher should provide.
func (c *Conn) Watch() *Watcher {
	w := &Watcher{
		conn:        c,
		signals:     make(chan *Notification),
		wakePump:    make(chan struct{}, 1),
		stopPump:    make(chan struct{}),
		pumpStopped: make(chan struct{}),
		matches:     mapset.New[*Match](),
	}
	go w.pump()

	c.mu.Lock()
	defer c.mu.Unlock()
	c.watchers.Add(w)
	return w
}

// A Watcher delivers signals received from the bus that match its
// filters.
type Watcher struct {
	conn     *Conn
	signals  chan *Notification
	wakePump chan struct{}

	stopPump    chan struct{}
	pumpStopped chan struct{}

	mu      sync.Mutex
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

// Close shuts down the Watcher.
func (w *Watcher) Close() {
	select {
	case <-w.pumpStopped:
		return
	default:
	}

	close(w.stopPump)
	close(w.wakePump)
	<-w.pumpStopped

	w.mu.Lock()
	defer w.mu.Unlock()
	for m := range w.matches {
		w.conn.removeMatch(context.Background(), m)
	}
	w.queue.Clear()
}

// Chan returns the channel on which signals are delivered.
//
// The caller must drain this channel of new signals promptly, to
// avoid overflowing the Watcher's receive queue and losing Notifications of
// interest. Missing signals due to an overflow are indicated by the
// Overflow field of the [Notification] that immediately precedes the
// discarded signal(s).
func (w *Watcher) Chan() <-chan *Notification {
	return w.signals
}

// Match requests delivery of signals that match the specification m.
//
// Matches are additive: a signal is delivered if it matches any of
// the Watcher's match specifications.
//
// If the match is added successfully, the returned remove function
// may be used to remove thee match without affecting other
// matches. Use of remove is optional, and may be ignored if the set
// of matches doesn't need to change for the lifetime of the Watcher.
func (w *Watcher) Match(m *Match) (remove func(), err error) {
	if err = w.conn.addMatch(context.Background(), m); err != nil {
		return nil, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	w.matches.Add(m)
	return func() {
		w.conn.removeMatch(context.Background(), m)
		w.mu.Lock()
		defer w.mu.Unlock()
		delete(w.matches, m)
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

	select {
	case <-w.pumpStopped:
		// raced with a Close, this watcher is done.
		return
	default:
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

	select {
	case <-w.pumpStopped:
		// raced with a Close, this watcher is done.
		return
	default:
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

func (w *Watcher) pump() {
	defer close(w.pumpStopped)
	defer close(w.signals)
	for {
		sig := func() *Notification {
			w.mu.Lock()
			defer w.mu.Unlock()
			ret, _ := w.queue.Pop()
			return ret
		}()
		if sig == nil {
			select {
			case <-w.stopPump:
				return
			case <-w.wakePump:
				continue
			}
		}
		select {
		case w.signals <- sig:
		case <-w.stopPump:
			return
		}
	}
}
