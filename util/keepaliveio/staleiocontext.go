package keepaliveio

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

type readWork struct {
	p []byte
	n int
	err error
}

type KeepaliveReadCloser struct {
	rc        io.ReadCloser
	onClose    context.CancelFunc

	timedOutMutex sync.Mutex
	timedOut bool

	// signal from timer goroutine to current Read
	timeoutSig chan struct{}
	lastReadComplete atomic.Value

	// currentRead serializes Read operations because we only have one currentReadbuf back-buffer
	currentRead sync.Mutex
	readWork chan *readWork
	// signal from underlying Read goroutine to the goroutine that called Read
	currentReadComplete chan struct{} // here to avoid allocation on every Read
	currentReadbuf      [1<<16]byte
}

var KeepaliveReadTimeout = fmt.Errorf("keepaliveio: reader timed out, nooping all calls")

type ReadCloserConstructor func(ctx context.Context) (io.ReadCloser, error)

// the returned error can only be from constructor
func NewKeepaliveReadCloser(ctx context.Context, timeout time.Duration, constructor ReadCloserConstructor) (context.Context, context.CancelFunc, *KeepaliveReadCloser, error) {
	ctx, cancel := context.WithCancel(ctx)
	rc, err := constructor(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	karc := &KeepaliveReadCloser{
		onClose: cancel,
		rc: rc,
		currentReadComplete: make(chan struct{}, 1),
		timeoutSig: make(chan struct{}, 1),
		readWork: make(chan *readWork, 1),
	}
	go karc.readWorker(ctx)
	go func() {
		t := time.NewTicker(timeout)
		defer t.Stop()

		for {
			select {
				case <-ctx.Done():
					return
				case <-t.C:
					lastReadComplete, ok := karc.lastReadComplete.Load().(time.Time)
					if ok && time.Now().Add(-timeout).Before(lastReadComplete) {
						continue
					}
					karc.timedOutMutex.Lock()
					karc.timedOut = true
					close(karc.timeoutSig)
					cancel()
					karc.timedOutMutex.Unlock()
					return
			}
		}
	}()
	return ctx, cancel, karc, nil
}

func DidTimeOut(r io.ReadCloser) (didTimeOut bool, ok bool) {
	karc, ok := r.(*KeepaliveReadCloser)
	if !ok {
		return false, false
	}
	return karc.TimedOut(), true
}

func (r *KeepaliveReadCloser) TimedOut() bool {
	r.timedOutMutex.Lock()
	defer r.timedOutMutex.Unlock()
	return r.timedOut
}

func (r *KeepaliveReadCloser) readWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.timeoutSig:
			return
		case w := <-r.readWork:
			w.n, w.err = r.rc.Read(w.p)
			r.lastReadComplete.Store(time.Now())
			r.currentReadComplete <- struct{}{}
		}
	}
}

func (r *KeepaliveReadCloser) Read(p []byte) (n int, err error) {

	// make sure we can only enter Read while not timed out
	if r.TimedOut() {
		return 0, KeepaliveReadTimeout
	}

	r.currentRead.Lock()

	// read into a back-buffer
	// if we time out before the read completes, the read goes to the back buffer
	// but we will return with 0, fmt.Errorf("timeout")
	w := readWork{r.currentReadbuf[:len(p)], 0, nil}
	r.readWork <- &w
	select {
		case <-r.timeoutSig:
			// let the current read go into the back-buffer
			return 0, KeepaliveReadTimeout
		case <-r.currentReadComplete:
			n, err = w.n, w.err
			copy(p, r.currentReadbuf[:n])
			r.currentRead.Unlock()
			return // n and err set in goroutine
	}
}

func (r *KeepaliveReadCloser) Close() error {
	if r.TimedOut() {
		r.onClose()
		return KeepaliveReadTimeout
	}
	r.onClose()
	return r.rc.Close()
}