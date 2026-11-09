// Package poller polls the Cursor usage API on a fixed interval with
// exponential backoff on failure.
package poller

import (
	"context"
	"errors"
	"sync"
	"time"

	"cursor-tray/internal/cursorapi"
)

// Event is delivered to the UI layer after each poll attempt.
type Event struct {
	Usage        *cursorapi.PlanUsage
	Err          error
	Unauthorized bool
}

// Poller periodically fetches usage and pushes events to Out.
type Poller struct {
	Out chan<- Event

	// Cookie returns the current cookie header value; may be empty.
	Cookie func() string

	MinInterval time.Duration // normal interval (default 10s)
	MaxInterval time.Duration // backoff ceiling (default 5m)

	mu       sync.Mutex
	interval time.Duration
	wake     chan struct{}
	stop     chan struct{}
	stopped  bool
	gen      uint64 // bumped by RefreshNow; stale in-flight polls are dropped
}

func (p *Poller) min() time.Duration {
	if p.MinInterval <= 0 {
		return 10 * time.Second
	}
	return p.MinInterval
}

func (p *Poller) max() time.Duration {
	if p.MaxInterval <= 0 {
		return 5 * time.Minute
	}
	return p.MaxInterval
}

func (p *Poller) ensureChans() {
	if p.wake == nil {
		p.wake = make(chan struct{}, 1)
	}
	if p.stop == nil {
		p.stop = make(chan struct{})
	}
}

// Run blocks until Stop is called.
func (p *Poller) Run(ctx context.Context) {
	p.mu.Lock()
	p.ensureChans()
	p.interval = p.min()
	p.mu.Unlock()

	for {
		select {
		case <-p.stop:
			return
		case <-ctx.Done():
			return
		default:
		}

		p.mu.Lock()
		gen := p.gen
		p.mu.Unlock()

		cookie := ""
		if p.Cookie != nil {
			cookie = p.Cookie()
		}

		var usage *cursorapi.PlanUsage
		var err error
		if cookie == "" {
			err = cursorapi.ErrUnauthorized
		} else {
			usage, err = cursorapi.Fetch(ctx, cookie)
		}

		p.mu.Lock()
		stale := gen != p.gen
		p.mu.Unlock()
		if stale {
			// Cookie/refresh changed while this poll was in flight — discard
			// and loop immediately so the tray never sticks on a pre-login 401.
			continue
		}

		unauthorized := errors.Is(err, cursorapi.ErrUnauthorized)
		if p.Out != nil {
			select {
			case p.Out <- Event{Usage: usage, Err: err, Unauthorized: unauthorized}:
			case <-p.stop:
				return
			}
		}

		p.mu.Lock()
		if err == nil {
			p.interval = p.min()
		} else {
			p.interval *= 2
			if p.interval < p.min() {
				p.interval = p.min()
			}
			if p.interval > p.max() {
				p.interval = p.max()
			}
		}
		wait := p.interval
		wake := p.wake
		p.mu.Unlock()

		select {
		case <-time.After(wait):
		case <-wake:
			p.mu.Lock()
			p.interval = p.min()
			p.mu.Unlock()
		case <-p.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// RefreshNow invalidates any in-flight poll and wakes the wait loop.
func (p *Poller) RefreshNow() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureChans()
	p.gen++
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// SetMinInterval updates the normal polling interval at runtime.
func (p *Poller) SetMinInterval(d time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.MinInterval = d
}

// Stop terminates the poll loop.
func (p *Poller) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ensureChans()
	if p.stopped {
		return
	}
	p.stopped = true
	close(p.stop)
}
