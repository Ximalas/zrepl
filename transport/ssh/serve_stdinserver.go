package ssh

import (
	"github.com/problame/go-netssh"
	"github.com/zrepl/zrepl/config"
	"github.com/zrepl/zrepl/daemon/nethelpers"
	"github.com/zrepl/zrepl/transport"
	"io"
	"net"
	"path"
	"time"
	"context"
	"github.com/pkg/errors"
	"sync/atomic"
)

func MultiStdinserverListenerFactoryFromConfig(g *config.Global, in *config.StdinserverServer) (transport.AuthenticatedListenerFactory,error) {

	for _, ci := range in.ClientIdentities {
		if err := transport.ValidateClientIdentity(ci); err != nil {
			return nil, errors.Wrapf(err, "invalid client identity %q", ci)
		}
	}

	clientIdentities := in.ClientIdentities
	sockdir := g.Serve.StdinServer.SockDir

	lf := func() (transport.AuthenticatedListener,error) {
		return multiStdinserverListenerFromClientIdentities(sockdir, clientIdentities)
	}

	return lf, nil
}

type multiStdinserverAcceptRes struct {
	conn *transport.AuthConn
	err error
}

type MultiStdinserverListener struct {
	listeners []*stdinserverListener
	accepts chan multiStdinserverAcceptRes
	closed int32
}

// client identities must be validated
func multiStdinserverListenerFromClientIdentities(sockdir string, cis []string) (*MultiStdinserverListener, error) {
	listeners := make([]*stdinserverListener, 0, len(cis))
	var err error
	for _, ci := range cis {
		sockpath := path.Join(sockdir, ci)
		l  := &stdinserverListener{clientIdentity: ci}
		if err = nethelpers.PreparePrivateSockpath(sockpath); err != nil {
			break
		}
		if l.l, err = netssh.Listen(sockpath); err != nil {
			break
		}
		listeners = append(listeners, l)
	}
	if err != nil {
		for _, l := range listeners {
			l.Close() // FIXME error reporting?
		}
		return nil, err
	}
	return &MultiStdinserverListener{listeners: listeners}, nil
}

func (m *MultiStdinserverListener) Accept(ctx context.Context) (*transport.AuthConn, error){

	if m.accepts == nil {
		m.accepts = make(chan multiStdinserverAcceptRes, len(m.listeners))
		for i := range m.listeners {
			go func(i int) {
				for atomic.LoadInt32(&m.closed) == 0 {
					conn, err := m.listeners[i].Accept(context.TODO())
					m.accepts <- multiStdinserverAcceptRes{conn, err}
				}
			}(i)
		}
	}

	res := <- m.accepts
	return res.conn, res.err

}

func (m *MultiStdinserverListener) Addr() (net.Addr) {
	return netsshAddr{}
}

func (m *MultiStdinserverListener) Close() error {
	atomic.StoreInt32(&m.closed, 1)
	var oneErr error
	for _, l := range m.listeners {
		if err := l.Close(); err != nil && oneErr == nil {
			oneErr = err
		}
	}
	return oneErr
}

// a single stdinserverListener (part of multiStinserverListener)
type stdinserverListener struct {
	l *netssh.Listener
	clientIdentity string
}

func (l stdinserverListener) Addr() net.Addr {
	return netsshAddr{}
}

func (l stdinserverListener) Accept(ctx context.Context) (*transport.AuthConn, error) {
	c, err := l.l.Accept()
	if err != nil {
		return nil, err
	}
	return transport.NewAuthConn(netsshConnToNetConnAdatper{c}, l.clientIdentity), nil
}

func (l stdinserverListener) Close() (err error) {
	return l.l.Close()
}

type netsshAddr struct{}

func (netsshAddr) Network() string { return "netssh" }
func (netsshAddr) String() string  { return "???" }

// works for both netssh.SSHConn and netssh.ServeConn
type netsshConn interface {
	io.ReadWriteCloser
	CloseWrite() error
}

type netsshConnToNetConnAdatper struct {
	netsshConn
}

func (netsshConnToNetConnAdatper) LocalAddr() net.Addr { return netsshAddr{} }

func (netsshConnToNetConnAdatper) RemoteAddr() net.Addr { return netsshAddr{} }

// FIXME log warning once!
func (netsshConnToNetConnAdatper) SetDeadline(t time.Time) error { return nil }

func (netsshConnToNetConnAdatper) SetReadDeadline(t time.Time) error { return nil }

func (netsshConnToNetConnAdatper) SetWriteDeadline(t time.Time) error { return nil }
