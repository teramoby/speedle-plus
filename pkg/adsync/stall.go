// Licensed under the Universal Permissive License (UPL) Version 1.0.

package adsync

import (
	"io"
	"sync"
	"time"
)

// stallWatch cancels a request only when no bytes have arrived for timeout.
// The lock also prevents a timer callback that raced with progress or stop
// from cancelling an active or completed download.
type stallWatch struct {
	mu      sync.Mutex
	timer   *time.Timer
	last    time.Time
	timeout time.Duration
	stopped bool
	cancel  func()
}

func newStallWatch(timeout time.Duration, cancel func()) *stallWatch {
	w := &stallWatch{last: time.Now(), timeout: timeout, cancel: cancel}
	w.timer = time.AfterFunc(timeout, w.check)
	return w
}

func (w *stallWatch) check() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stopped {
		return
	}
	remaining := w.timeout - time.Since(w.last)
	if remaining > 0 {
		w.timer.Reset(remaining)
		return
	}
	w.stopped = true
	w.cancel()
}

func (w *stallWatch) progress() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.stopped {
		w.last = time.Now()
	}
}

func (w *stallWatch) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.stopped = true
	w.timer.Stop()
}

type progressReader struct {
	io.Reader
	watch   *stallWatch
	readErr error
}

func (r *progressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err != nil {
		r.readErr = err
	}
	if n > 0 {
		r.watch.progress()
	}
	return n, err
}
