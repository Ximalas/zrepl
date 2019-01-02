// Package transport defines a common interface for
// network connections that have an associated client identity.
package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/zrepl/zrepl/logger"
	"github.com/zrepl/zrepl/rpc/dataconn/timeoutconn"
	"github.com/zrepl/zrepl/zfs"
)

type AuthConn struct {
	Wire
	clientIdentity string
}

var _ timeoutconn.SyscallConner = AuthConn{}

var errAuthConnNoSyscallConn = fmt.Errorf("underlying conn is not a SyscallConn")

func (a AuthConn) SyscallConn() (rawConn syscall.RawConn, err error) {
	scc, ok := a.Wire.(timeoutconn.SyscallConner)
	if !ok {
		return nil, errAuthConnNoSyscallConn
	}
	return scc.SyscallConn()
}

func NewAuthConn(conn Wire, clientIdentity string) *AuthConn {
	return &AuthConn{conn, clientIdentity}
}

func (c *AuthConn) ClientIdentity() string {
	if err := ValidateClientIdentity(c.clientIdentity); err != nil {
		panic(err)
	}
	return c.clientIdentity
}

// like net.Listener, but with an AuthenticatedConn instead of net.Conn
type AuthenticatedListener interface {
	Addr() net.Addr
	Accept(ctx context.Context) (*AuthConn, error)
	Close() error
}

type AuthenticatedListenerFactory func() (AuthenticatedListener, error)

type Wire interface {
       net.Conn
       // A call to CloseWrite indicates that no further Write calls will be made to Wire.
       // The implementation must return an error in case of Write calls after CloseWrite.
       // On the peer's side, after it read all data written to Wire prior to the call to
       // CloseWrite on our side, the peer's Read calls must return io.EOF.
       // CloseWrite must not affect the read-direction of Wire: specifically, the
       // peer must continue to be able to send, and our side must continue be
       // able to receive data over Wire.
       //
       // Note that CloseWrite may (and most likely will) return sooner than the
       // peer having received all data written to Wire prior to CloseWrite.
       // Note further that buffering happening in the network stacks on either side
       // mandates an explicit acknowledgement from the peer that the connection may
       // be fully shut down: If we call Close without such acknowledgement, any data
       // from peer to us that was already in flight may cause connection resets to
       // be sent from us to the peer via the specific transport protocol. Those
       // resets (e.g. RST frames) may erase all connection context on the peer,
       // including data in its receive buffers. Thus, those resets are in race with
       // a) transmission of data written prior to CloseWrite and
       // b) the peer application reading from those buffers.
       //
       // The WaitForPeerClose method can be used to wait for connection termination,
       // iff the implementation supports it. If it does not, the only reliable way
       // to wait for a peer to have read all data from Wire (until io.EOF), is to
       // expect it to close the wire at that point as well, and to drain Wire until
       // we also read io.EOF.
       CloseWrite() error

       // Wait for the peer to close the connection.
       // No data that could otherwise be Read is lost as a consequence of this call.
       // The use case for this API is abortive connection shutdown.
       // To provide any value over draining Wire using io.Read, an implementation
       // will likely use out-of-bounds messaging mechanisms.
       // TODO WaitForPeerClose() (supported bool, err error)
}

type Connecter interface {
	Connect(ctx context.Context) (Wire, error)
}

// A client identity must be a single component in a ZFS filesystem path
func ValidateClientIdentity(in string) (err error) {
	path, err := zfs.NewDatasetPath(in)
	if err != nil {
		return err
	}
	if path.Length() != 1 {
		return errors.New("client identity must be a single path comonent (not empty, no '/')")
	}
	return nil
}

type contextKey int

const contextKeyLog contextKey = 0

type Logger = logger.Logger

func WithLogger(ctx context.Context, log Logger) context.Context {
	return context.WithValue(ctx, contextKeyLog, log)
}

func GetLogger(ctx context.Context) Logger {
	if log, ok := ctx.Value(contextKeyLog).(Logger); ok {
		return log
	}
	return logger.NewNullLogger()
}
