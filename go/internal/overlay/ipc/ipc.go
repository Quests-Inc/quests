// Package ipc is the unix-socket control channel (niri binds → toggle/monitor/…).
package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SocketPath is $QUESTS_OVERLAY_SOCK or $XDG_RUNTIME_DIR/quests-overlay.sock.
func SocketPath() string {
	if p := strings.TrimSpace(os.Getenv("QUESTS_OVERLAY_SOCK")); p != "" {
		return p
	}
	runtime := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if runtime == "" {
		runtime = os.TempDir()
	}
	return filepath.Join(runtime, "quests-overlay.sock")
}

// Handler turns one command line into a reply string. Called on the UI thread
// when Dispatch is set; otherwise on the accept goroutine.
type Handler func(cmd string) string

// Dispatch marshals work onto the GTK main loop (e.g. glib.IdleAdd).
type Dispatch func(fn func())

// Server listens for one-line commands.
type Server struct {
	ln       net.Listener
	stop     chan struct{}
	handler  Handler
	dispatch Dispatch
	wg       sync.WaitGroup
}

// Start binds the socket and serves until Stop.
func Start(handler Handler, dispatch Dispatch) (*Server, error) {
	path := SocketPath()
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	s := &Server{ln: ln, stop: make(chan struct{}), handler: handler, dispatch: dispatch}
	s.wg.Add(1)
	go s.loop()
	return s, nil
}

// Stop closes the listener and removes the socket file.
func (s *Server) Stop() {
	if s == nil {
		return
	}
	close(s.stop)
	_ = s.ln.Close()
	s.wg.Wait()
	_ = os.Remove(SocketPath())
}

func (s *Server) loop() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			select {
			case <-s.stop:
				return
			default:
				return
			}
		}
		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, 256)
			n, err := c.Read(buf)
			cmd := ""
			if err == nil && n > 0 {
				cmd = strings.TrimSpace(strings.Split(string(buf[:n]), "\n")[0])
			}
			reply := s.invoke(cmd)
			_, _ = c.Write([]byte(reply))
		}(conn)
	}
}

func (s *Server) invoke(cmd string) string {
	if s.handler == nil {
		return "error: no handler"
	}
	if s.dispatch == nil {
		return s.handler(cmd)
	}
	type box struct {
		reply string
		done  chan struct{}
	}
	b := &box{done: make(chan struct{})}
	s.dispatch(func() {
		defer close(b.done)
		defer func() {
			if r := recover(); r != nil {
				b.reply = fmt.Sprintf("error: %v", r)
			}
		}()
		b.reply = s.handler(cmd)
	})
	select {
	case <-b.done:
		if b.reply == "" {
			return "ok"
		}
		return b.reply
	case <-time.After(2 * time.Second):
		return "error: timeout"
	}
}

// Send talks to a running daemon.
func Send(command string) (string, error) {
	path := SocketPath()
	conn, err := net.DialTimeout("unix", path, 1500*time.Millisecond)
	if err != nil {
		return "", fmt.Errorf("overlay socket not found: %s (is the overlay running?)", path)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(1500 * time.Millisecond))
	if _, err := conn.Write([]byte(strings.TrimSpace(command) + "\n")); err != nil {
		return "", err
	}
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(buf[:n])), nil
}
