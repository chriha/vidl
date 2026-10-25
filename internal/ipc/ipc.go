// Package ipc hands URLs to a running vidl over a local Unix domain socket.
// Each connection carries one JSON request ({"add":["url",...],"paused":true}) and one JSON
// reply ({"added":N} or {"error":"..."}), both bounded by a timeout.
package ipc

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// DefaultTimeout bounds one request/reply exchange on either side.
const DefaultTimeout = 2 * time.Second

// SocketName is the socket's file name inside the config dir.
const SocketName = "vidl.sock"

// maxSocketPath stays below macOS's 104-byte sun_path limit (Linux allows 108).
const maxSocketPath = 103

// maxRequest caps the size of one request.
const maxRequest = 1 << 20

// maxAcceptBackoff caps the pause between retries after a failed Accept.
const maxAcceptBackoff = time.Second

// wsaeConnRefused is Windows' raw WSAECONNREFUSED, which net does not map to
// syscall.ECONNREFUSED.
const wsaeConnRefused = syscall.Errno(10061)

var (
	// ErrNoServer means nothing listens on the socket: it is missing or stale.
	ErrNoServer = errors.New("no running vidl")
	// ErrRunning means another process already serves the socket.
	ErrRunning = errors.New("vidl is already running")
)

// Request is the client's message.
type Request struct {
	Add []string `json:"add"`
	// Paused asks to add the URLs paused instead of queued.
	Paused bool `json:"paused,omitempty"`
}

// Reply is the server's answer to one Request.
type Reply struct {
	Added int    `json:"added"`
	Error string `json:"error,omitempty"`
}

// Handler adds the URLs of one request and returns how many were added.
// It must return promptly; requests are handled one at a time.
type Handler func(req Request) (int, error)

// SocketPath returns the socket path for a config dir. When that path is too
// long for a Unix socket, it falls back to a private directory under
// os.TempDir() named after a hash of the config dir.
func SocketPath(configDir string) string {
	path := filepath.Join(configDir, SocketName)
	if len(path) <= maxSocketPath {
		return path
	}
	sum := sha256.Sum256([]byte(filepath.Clean(configDir)))
	return filepath.Join(os.TempDir(), "vidl-"+hex.EncodeToString(sum[:8]), SocketName)
}

// Server accepts add requests on a socket.
type Server struct {
	// OnError receives errors from individual connections that could not be
	// reported back to the client. Set it before Serve; nil ignores them.
	OnError func(error)

	ln      net.Listener
	handle  Handler
	timeout time.Duration
}

// Listen creates the socket at path. A stale socket file left by a crashed
// instance (connection refused) is replaced; a socket that still answers
// yields ErrRunning; any other dial error is returned, so the socket of a
// live but busy instance is never removed.
func Listen(path string, handle Handler) (*Server, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	if _, err := os.Lstat(path); err == nil {
		conn, err := net.DialTimeout("unix", path, DefaultTimeout)
		if err == nil {
			conn.Close()
			return nil, ErrRunning
		}
		if !noListener(err) {
			return nil, fmt.Errorf("check socket: %w", err)
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("remove stale socket: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("check socket: %w", err)
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("restrict socket: %w", err)
	}
	return &Server{ln: ln, handle: handle, timeout: DefaultTimeout}, nil
}

// Serve handles connections one at a time until Close and returns once the
// connection in progress is done. Accept errors (such as running out of file
// descriptors) go to OnError and are retried after a growing pause.
func (s *Server) Serve() {
	var backoff time.Duration
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if s.OnError != nil {
				s.OnError(fmt.Errorf("accept: %w", err))
			}
			backoff = min(max(2*backoff, 5*time.Millisecond), maxAcceptBackoff)
			time.Sleep(backoff)
			continue
		}
		backoff = 0
		if err := s.serveConn(conn); err != nil && s.OnError != nil {
			s.OnError(err)
		}
	}
}

// Close stops accepting connections and removes the socket file.
func (s *Server) Close() error {
	return s.ln.Close()
}

func (s *Server) serveConn(conn net.Conn) error {
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(s.timeout)); err != nil {
		return fmt.Errorf("set read deadline: %w", err)
	}
	var reply Reply
	var req Request
	if err := json.NewDecoder(io.LimitReader(conn, maxRequest)).Decode(&req); err != nil {
		reply.Error = "bad request: " + err.Error()
	} else if len(req.Add) == 0 {
		reply.Error = "no URLs to add"
	} else if n, err := s.handle(Request{Add: req.Add}); err != nil {
		reply.Error = err.Error()
	} else {
		reply.Added = n
	}

	if err := conn.SetWriteDeadline(time.Now().Add(s.timeout)); err != nil {
		return fmt.Errorf("set write deadline: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(reply); err != nil {
		return fmt.Errorf("write reply: %w", err)
	}
	return nil
}

// Send hands req to the vidl listening at path and returns how many it
// added. It fails with ErrNoServer when the socket is missing or stale;
// other dial errors (a timeout, no permission) are returned as they are.
func Send(path string, req Request, timeout time.Duration) (int, error) {
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		if noListener(err) {
			return 0, fmt.Errorf("%w: %v", ErrNoServer, err)
		}
		return 0, fmt.Errorf("connect to running vidl: %w", err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return 0, fmt.Errorf("set deadline: %w", err)
	}
	if err := json.NewEncoder(conn).Encode(Request{Add: req.Add}); err != nil {
		return 0, fmt.Errorf("send request: %w", err)
	}
	var reply Reply
	if err := json.NewDecoder(conn).Decode(&reply); err != nil {
		return 0, fmt.Errorf("read reply: %w", err)
	}
	if reply.Error != "" {
		return 0, fmt.Errorf("running vidl: %s", reply.Error)
	}
	return reply.Added, nil
}

// noListener reports whether a dial error means nobody listens on the socket:
// the file is missing or its owner is gone.
func noListener(err error) bool {
	return errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, wsaeConnRefused)
}
