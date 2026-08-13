package ipc

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/foisal/linboard/internal/config"
)

// SocketPath is where a running LinBoard instance listens for commands.
func SocketPath() (string, error) {
	dir, err := config.DataDir()
	if err != nil {
		return "", err
	}
	runDir := filepath.Join(dir, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(runDir, "linboard.sock"), nil
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
		return fmt.Errorf("LinBoard is not running (start ./linboard first): %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err = io.WriteString(conn, "toggle\n"); err != nil {
		return err
	}
	buf := make([]byte, 8)
	_, _ = conn.Read(buf) // ack from current servers; older ones may close
	return nil
}

// ToggleWithRetry retries Toggle while the main instance is still starting its IPC server.
func ToggleWithRetry(attempts int, delay time.Duration) error {
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
	onToggle func()
	wg       sync.WaitGroup
}

func StartServer(onToggle func()) (*Server, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}
	_ = os.Remove(path)

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}

	s := &Server{ln: ln, onToggle: onToggle}
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
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	switch strings.TrimSpace(strings.ToLower(line)) {
	case "toggle":
		_, _ = io.WriteString(conn, "ok\n")
		if s.onToggle != nil {
			s.onToggle()
		}
	}
}

func (s *Server) Close() {
	if s.ln != nil {
		_ = s.ln.Close()
	}
	s.wg.Wait()
	path, _ := SocketPath()
	_ = os.Remove(path)
}
