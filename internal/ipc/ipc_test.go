package ipc

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// shortDir returns a fresh directory with a short path: macOS limits socket
// paths to 104 bytes, and t.TempDir() there is too long once the test name
// is appended.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "ipc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

// startServer listens on a socket in a short dir and serves until the test ends.
// configure runs before Serve starts.
func startServer(t *testing.T, handle Handler, configure ...func(*Server)) (*Server, string) {
	t.Helper()
	path := filepath.Join(shortDir(t), "vidl.sock")
	s, err := Listen(path, handle)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	for _, c := range configure {
		c(s)
	}
	done := make(chan struct{})
	go func() { s.Serve(); close(done) }()
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		<-done
	})
	return s, path
}

func TestSendRoundTrip(t *testing.T) {
	var (
		mu  sync.Mutex
		got []string
	)
	_, path := startServer(t, func(urls []string) (int, error) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, urls...)
		return len(urls), nil
	})

	urls := []string{"https://example.com/a", "https://example.com/b?x=1&y=2"}
	n, err := Send(path, urls, time.Second)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if n != 2 {
		t.Errorf("added = %d, want 2", n)
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(got, urls) {
		t.Errorf("handler got %q, want %q", got, urls)
	}
}

func TestSendReturnsHandlerError(t *testing.T) {
	_, path := startServer(t, func([]string) (int, error) {
		return 0, errors.New("queue is full")
	})

	_, err := Send(path, []string{"https://example.com/a"}, time.Second)
	if err == nil || !strings.Contains(err.Error(), "queue is full") {
		t.Fatalf("err = %v, want the handler's error", err)
	}
	if errors.Is(err, ErrNoServer) {
		t.Errorf("err = %v, must not be ErrNoServer when the server answered", err)
	}
}

func TestServerRejectsBadRequests(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		wantErr string
	}{
		{"no urls", `{"add":[]}`, "no URLs"},
		{"missing add", `{}`, "no URLs"},
		{"malformed json", `{"add":`, "bad request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called := false
			_, path := startServer(t, func(urls []string) (int, error) {
				called = true
				return len(urls), nil
			})

			conn, err := net.Dial("unix", path)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := conn.SetDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Write([]byte(tt.payload)); err != nil {
				t.Fatal(err)
			}
			if err := conn.(*net.UnixConn).CloseWrite(); err != nil {
				t.Fatal(err)
			}
			var r Reply
			if err := json.NewDecoder(conn).Decode(&r); err != nil {
				t.Fatalf("decode reply: %v", err)
			}
			if !strings.Contains(r.Error, tt.wantErr) {
				t.Errorf("reply error = %q, want it to contain %q", r.Error, tt.wantErr)
			}
			if called {
				t.Error("handler called for a bad request")
			}
		})
	}
}

func TestSendNoServer(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{"missing socket", func(*testing.T, string) {}},
		{"stale socket", makeStaleSocket},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(shortDir(t), "vidl.sock")
			tt.setup(t, path)
			_, err := Send(path, []string{"https://example.com/a"}, time.Second)
			if !errors.Is(err, ErrNoServer) {
				t.Fatalf("err = %v, want ErrNoServer", err)
			}
		})
	}
}

func TestSendTimesOutOnSilentServer(t *testing.T) {
	path := filepath.Join(shortDir(t), "vidl.sock")
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			accepted <- conn
		}
	}()

	start := time.Now()
	_, err = Send(path, []string{"https://example.com/a"}, 50*time.Millisecond)
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("err = %v, want a deadline error", err)
	}
	if errors.Is(err, ErrNoServer) {
		t.Errorf("err = %v, must not be ErrNoServer: the socket answered the dial", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Send took %v, want it bounded by the timeout", elapsed)
	}
	select {
	case conn := <-accepted:
		conn.Close()
	default:
	}
}

func TestServerDropsSilentClient(t *testing.T) {
	_, path := startServer(t, func(urls []string) (int, error) { return len(urls), nil },
		func(s *Server) { s.timeout = 50 * time.Millisecond })

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	// Send nothing: the server must give up on its own and reply with an error.
	var r Reply
	if err := json.NewDecoder(conn).Decode(&r); err != nil {
		t.Fatalf("decode reply: %v", err)
	}
	if r.Error == "" {
		t.Error("reply has no error for a client that never sent a request")
	}
}

func TestListenRecoversStaleSocket(t *testing.T) {
	path := filepath.Join(shortDir(t), "vidl.sock")
	makeStaleSocket(t, path)

	s, err := Listen(path, func(urls []string) (int, error) { return len(urls), nil })
	if err != nil {
		t.Fatalf("Listen over a stale socket: %v", err)
	}
	done := make(chan struct{})
	go func() { s.Serve(); close(done) }()

	if n, err := Send(path, []string{"https://example.com/a"}, time.Second); err != nil || n != 1 {
		t.Errorf("Send = %d, %v; want 1, nil", n, err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	<-done
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("socket file still exists after Close (err = %v)", err)
	}
}

func TestListenRefusesLiveSocket(t *testing.T) {
	_, path := startServer(t, func(urls []string) (int, error) { return len(urls), nil })

	_, err := Listen(path, func(urls []string) (int, error) { return len(urls), nil })
	if !errors.Is(err, ErrRunning) {
		t.Fatalf("err = %v, want ErrRunning", err)
	}
	// The live server must still answer.
	if _, err := Send(path, []string{"https://example.com/a"}, time.Second); err != nil {
		t.Errorf("Send after refused Listen: %v", err)
	}
}

func TestSocketPath(t *testing.T) {
	short := "/home/u/.config/vidl"
	if got, want := SocketPath(short), filepath.Join(short, "vidl.sock"); got != want {
		t.Errorf("SocketPath(%q) = %q, want %q", short, got, want)
	}

	long := filepath.Join("/", strings.Repeat("d", 120), "vidl")
	got := SocketPath(long)
	if len(got) > maxSocketPath {
		t.Errorf("SocketPath(long) = %q (%d bytes), want at most %d", got, len(got), maxSocketPath)
	}
	if filepath.Dir(filepath.Dir(got)) != filepath.Clean(os.TempDir()) {
		t.Errorf("SocketPath(long) = %q, want it under %q", got, os.TempDir())
	}
	other := filepath.Join("/", strings.Repeat("e", 120), "vidl")
	if SocketPath(other) == got {
		t.Error("different long config dirs share one fallback socket path")
	}
	if SocketPath(long) != got {
		t.Error("SocketPath is not stable for the same dir")
	}
}

// makeStaleSocket leaves a socket file at path with no listener behind it,
// like a crashed vidl does.
func makeStaleSocket(t *testing.T, path string) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	ln.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("stale socket file missing: %v", err)
	}
}

func TestDialErrorOtherThanRefusedKeepsSocket(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs Unix socket permissions enforced for a non-root user")
	}
	_, path := startServer(t, func(urls []string) (int, error) { return len(urls), nil })
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o600) })

	_, err := Send(path, []string{"https://example.com/a"}, time.Second)
	if err == nil || errors.Is(err, ErrNoServer) {
		t.Errorf("Send err = %v, want a non-ErrNoServer error", err)
	}
	_, err = Listen(path, func(urls []string) (int, error) { return len(urls), nil })
	if err == nil || errors.Is(err, ErrRunning) {
		t.Errorf("Listen err = %v, want a dial error", err)
	}

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Send(path, []string{"https://example.com/a"}, time.Second); err != nil {
		t.Errorf("Send after restoring permissions: %v (socket was removed?)", err)
	}
}

// flakyListener fails Accept with errs, then blocks until closed.
type flakyListener struct {
	net.Listener
	errs   chan error
	closed chan struct{}
}

func (l *flakyListener) Accept() (net.Conn, error) {
	select {
	case err := <-l.errs:
		return nil, err
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *flakyListener) Close() error {
	close(l.closed)
	return nil
}

func TestServeRetriesAcceptErrors(t *testing.T) {
	acceptErr := errors.New("too many open files")
	ln := &flakyListener{errs: make(chan error, 2), closed: make(chan struct{})}
	ln.errs <- acceptErr
	ln.errs <- acceptErr
	reported := make(chan error, 2)
	s := &Server{ln: ln, timeout: time.Second, OnError: func(err error) { reported <- err }}

	done := make(chan struct{})
	go func() { s.Serve(); close(done) }()
	for range 2 {
		select {
		case err := <-reported:
			if !errors.Is(err, acceptErr) {
				t.Errorf("OnError got %v, want the accept error", err)
			}
		case <-done:
			t.Fatal("Serve returned after an accept error")
		case <-time.After(2 * time.Second):
			t.Fatal("accept error not reported")
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
}
