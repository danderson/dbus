package transport

import (
	"bufio"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/creachadair/mds/queue"
	"golang.org/x/sys/unix"
)

// Transport is a raw DBus connection.
type Transport interface {
	io.ReadWriteCloser

	// GetFiles returns n received files that were attached to
	// previously read bytes as ancillary data.
	GetFiles(n int) ([]*os.File, error)
	// WriteWithFiles is like Transport.Write, but additionally sends
	// the given files as ancillary data.
	WriteWithFiles(bs []byte, fds []*os.File) (int, error)
}

// DialUnix connects to the bus at the given path.
func DialUnix(ctx context.Context, path string) (Transport, error) {
	addr := &net.UnixAddr{
		Net:  "unix",
		Name: path,
	}

	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return nil, err
	}

	ret := &unixTransport{
		conn: conn,
		fds:  queue.New[*os.File](),
	}
	ret.buf = bufio.NewReader(funcReader(ret.readToBuf))

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Time{}
	}

	if err := ret.conn.SetDeadline(deadline); err != nil {
		ret.Close()
		return nil, err
	}
	if err := ret.auth(); err != nil {
		ret.Close()
		return nil, err
	}
	if err := ret.conn.SetDeadline(time.Time{}); err != nil {
		ret.Close()
		return nil, err
	}

	return ret, nil
}

// unixTransport is a Transport that runs over a Unix domain socket.
type unixTransport struct {
	conn *net.UnixConn
	oob  [512]byte
	buf  *bufio.Reader
	fds  *queue.Queue[*os.File]
}

func (u *unixTransport) Read(bs []byte) (int, error) {
	return u.buf.Read(bs)
}

func (u *unixTransport) Write(bs []byte) (int, error) {
	return u.conn.Write(bs)
}

func (u *unixTransport) Close() error {
	u.fds.Each(func(f *os.File) bool {
		f.Close()
		return true
	})
	u.fds.Clear()
	u.buf.Discard(u.buf.Buffered())
	return u.conn.Close()
}

func (u *unixTransport) WriteWithFiles(bs []byte, fs []*os.File) (int, error) {
	if len(fs) == 0 {
		return u.Write(bs)
	}

	fds := make([]int, len(fs))
	for _, f := range fs {
		fds = append(fds, int(f.Fd()))
	}
	scm := unix.UnixRights(fds...)
	n, oobn, err := u.conn.WriteMsgUnix(bs, scm, nil)
	if err != nil {
		u.Close()
		return n, err
	}
	if oobn != len(scm) {
		u.Close()
		return n, io.ErrShortWrite
	}
	return n, nil
}

func (u *unixTransport) GetFiles(n int) ([]*os.File, error) {
	ret := make([]*os.File, 0, n)
	for range n {
		f, ok := u.fds.Pop()
		if !ok {
			for _, f := range ret {
				f.Close()
			}
			return nil, errors.New("requested file not available")
		}
		ret = append(ret, f)
	}
	return ret, nil
}

func (u *unixTransport) auth() error {
	// In theory, we're supposed to speak SASL now and carefully
	// negotiate an authentication with the bus. However, in practice,
	// when you talk to busses over a unix socket, the bus
	// authenticates you with the peer credentials that it can pull
	// from the socket without the client's help.
	//
	// So, the auth handshake boils down to a preamble string we can
	// blast out in one block, and see if the response has the
	// expected happy path shape. If it doesn't, we're just going to
	// hang up anyway so no point in sequencing the messages cleanly.
	uid := os.Getuid()
	uidBs := hex.EncodeToString([]byte(strconv.Itoa(uid)))
	if _, err := u.conn.Write([]byte("\x00AUTH EXTERNAL ")); err != nil {
		return err
	}
	if _, err := io.WriteString(u.conn, uidBs); err != nil {
		return err
	}
	if _, err := u.conn.Write([]byte("\r\nNEGOTIATE_UNIX_FD\r\nBEGIN\r\n")); err != nil {
		return err
	}

	resp, err := u.buf.ReadString('\n')
	if err != nil {
		return err
	}
	if !strings.HasPrefix(resp, "OK ") {
		return fmt.Errorf("AUTH EXTERNAL failed, server said %q", strings.TrimSpace(resp))
	}

	resp, err = u.buf.ReadString('\n')
	if err != nil {
		return err
	}
	if resp != "AGREE_UNIX_FD\r\n" {
		return fmt.Errorf("NEGOTIATE_UNIX_FD failed, server said %q", strings.TrimSpace(resp))
	}

	return nil
}

func (u *unixTransport) readToBuf(bs []byte) (int, error) {
	n, oobn, flags, _, err := u.conn.ReadMsgUnix(bs, u.oob[:])
	if flags&unix.MSG_CTRUNC != 0 {
		u.Close()
		return 0, errors.New("control message truncated")
	}
	if oobn > 0 {
		if oobErr := u.parseFDs(u.oob[:oobn]); err != nil {
			u.Close()
			return 0, oobErr
		}
	}
	if err != nil {
		u.Close()
		return 0, err
	}

	return n, nil
}

func (u *unixTransport) parseFDs(oob []byte) error {
	scms, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return err
	}
	// Accumulate errors and keep parsing on errors. We want to
	// extract all provided file descriptors from the message, so that
	// we can correctly close all of them on error. If we bailed on
	// first error, we'd leave dangling fds in the process, and allow
	// for a DoS.
	var errs []error
	for _, scm := range scms {
		if scm.Header.Level != unix.SOL_SOCKET || scm.Header.Type != unix.SCM_RIGHTS {
			continue
		}
		var fds []int
		fds, err = unix.ParseUnixRights(&scm)
		if err != nil {
			errs = append(errs, fmt.Errorf("parsing unix rights: %w", err))
			continue
		}
		for _, fd := range fds {
			f := os.NewFile(uintptr(fd), "")
			if f == nil {
				errs = append(errs, fmt.Errorf("invalid file descriptor %d received on dbus socket", fd))
			} else {
				u.fds.Add(f)
			}
		}
	}

	if len(errs) != 0 {
		return errors.Join(errs...)
	}
	return nil
}

type funcReader func([]byte) (int, error)

func (f funcReader) Read(bs []byte) (int, error) {
	return f(bs)
}
