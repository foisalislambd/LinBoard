package ipc

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/foisal/linboard/internal/config"
)

// SocketPath is where a running LinBoard instance listens for commands.
// The socket lives in the XDG runtime directory and is mode 0600.
func SocketPath() (string, error) {
	dir, err := config.RuntimeDir()
	if err != nil {
		return "", err
	}
	return socketPathIn(dir)
}

func socketPathIn(dir string) (string, error) {
	p := filepath.Join(dir, "linboard.sock")
	if unixPathOK(p) {
		return p, nil
	}
	// Abstract sun_path is 108 bytes including the NUL. A very long home
	// directory can exceed that, so fall back to a short directory in /tmp.
	short := filepath.Join(os.TempDir(), fmt.Sprintf("linboard-%d", os.Getuid()))
	if err := os.MkdirAll(short, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(short, 0o700); err != nil {
		return "", err
	}
	p = filepath.Join(short, "linboard.sock")
	if !unixPathOK(p) {
		return "", fmt.Errorf("socket path too long")
	}
	return p, nil
}

func unixPathOK(p string) bool {
	// Linux sockaddr_un.sun_path is 108 bytes, including the terminating NUL.
	return len(p) < 108
}

// Toggle asks a running LinBoard instance to show/hide history.
// Use this as a GNOME/KDE custom keyboard shortcut command.
func Toggle() error {
	path, err := SocketPath()
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", path, 2*time.Second)
	if err != nil {
		return fmt.Errorf("LinBoard is not running (start linboard first): %w", err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = io.WriteString(conn, "toggle\n"); err != nil {
		return err
	}
	buf := make([]byte, 8)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(buf[:n])) != "ok" {
		return fmt.Errorf("unexpected ipc reply %q", strings.TrimSpace(string(buf[:n])))
	}
	return nil
}

// ToggleWithRetry retries Toggle while the main instance is still starting its IPC server.
func ToggleWithRetry(attempts int, delay time.Duration) error {
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := Toggle(); err == nil {
			return nil
		} else {
			lastErr = err
			if i+1 < attempts {
				time.Sleep(delay)
			}
		}
	}
	return lastErr
}

// Server handles local IPC while LinBoard is running.
type Server struct {
	ln       net.Listener
	path     string
	onToggle func()
	wg       sync.WaitGroup
	connWg   sync.WaitGroup
}

func StartServer(onToggle func()) (*Server, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, err
	}

	s := &Server{ln: ln, path: path, onToggle: onToggle}
	s.wg.Add(1)
	go s.serve()
	return s, nil
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.connWg.Add(1)
		go func() {
			defer s.connWg.Done()
			s.handle(conn)
		}()
	}
}

func (s *Server) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "toggle":
		// Run the handler before acking so a client only hears success after
		// the window toggle has been handed to the UI thread.
		if s.onToggle != nil {
			s.onToggle()
		}
		_, _ = io.WriteString(conn, "ok\n")
	default:
		_, _ = io.WriteString(conn, "err\n")
	}
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	if s.ln != nil {
		_ = s.ln.Close()
	}
	s.wg.Wait()
	done := make(chan struct{})
	go func() {
		s.connWg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		// A handler blocked in the UI must not keep the process alive.
		log.Printf("ipc: timed out waiting for a connection to finish")
	}
	if s.path != "" {
		_ = os.Remove(s.path)
	}
}
