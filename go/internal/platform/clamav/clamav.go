// Package clamav speaks just enough of clamd's protocol to scan one upload.
//
// This is INSTREAM by hand rather than a library on purpose: the wire format
// is a stable, documented ~50 lines (length-prefixed chunks, zero-length
// terminator, one-line verdict), while the Go clamd libraries are long
// unmaintained — and an abandoned dependency sitting in the malware-scanning
// path is its own liability.
package clamav

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// ErrUnavailable means clamd could not be reached or spoke unexpectedly.
// Callers must treat it as "not scanned", never as "clean".
var ErrUnavailable = errors.New("clamav: unavailable")

// chunkSize stays well under clamd's default StreamMaxLength per-chunk limits.
const chunkSize = 64 << 10

type Client struct {
	Addr    string
	Timeout time.Duration
}

func New(addr string) *Client {
	return &Client{Addr: addr, Timeout: 60 * time.Second}
}

func (c *Client) Configured() bool { return c != nil && c.Addr != "" }

type Result struct {
	// Infected is only meaningful when the scan actually completed.
	Infected bool
	// Signature is clamd's name for the threat, when infected.
	Signature string
}

// Ping verifies clamd is answering, for the health endpoint.
func (c *Client) Ping(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("zPING\x00")); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	resp, err := readResponse(conn)
	if err != nil {
		return err
	}
	if !strings.Contains(resp, "PONG") {
		return fmt.Errorf("%w: unexpected ping reply %q", ErrUnavailable, resp)
	}
	return nil
}

// Scan streams r to clamd and reports the verdict. An error means the file was
// not scanned — the caller decides what to do, but "unknown" is not "clean".
func (c *Client) Scan(ctx context.Context, r io.Reader) (Result, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	buf := make([]byte, chunkSize)
	var hdr [4]byte
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			binary.BigEndian.PutUint32(hdr[:], uint32(n))
			if _, err := conn.Write(hdr[:]); err != nil {
				return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return Result{}, readErr
		}
	}
	// Zero-length chunk ends the stream.
	binary.BigEndian.PutUint32(hdr[:], 0)
	if _, err := conn.Write(hdr[:]); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}

	resp, err := readResponse(conn)
	if err != nil {
		return Result{}, err
	}
	return parseVerdict(resp)
}

func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("%w: no address configured", ErrUnavailable)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	return conn, nil
}

// readResponse reads clamd's NUL-terminated reply.
func readResponse(conn net.Conn) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for sb.Len() < 4096 {
		n, err := conn.Read(buf)
		if n > 0 {
			if buf[0] == 0 {
				return strings.TrimSpace(sb.String()), nil
			}
			sb.WriteByte(buf[0])
		}
		if err == io.EOF {
			// Some builds close without the NUL; what we have is the reply.
			return strings.TrimSpace(sb.String()), nil
		}
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	return "", fmt.Errorf("%w: reply too long", ErrUnavailable)
}

// parseVerdict maps clamd's one-liner onto a result.
//
//	stream: OK
//	stream: Eicar-Test-Signature FOUND
//	INSTREAM size limit exceeded. ERROR
func parseVerdict(resp string) (Result, error) {
	switch {
	case resp == "":
		return Result{}, fmt.Errorf("%w: empty reply", ErrUnavailable)
	case strings.HasSuffix(resp, "ERROR"):
		return Result{}, fmt.Errorf("%w: %s", ErrUnavailable, resp)
	case strings.HasSuffix(resp, "OK"):
		return Result{Infected: false}, nil
	case strings.HasSuffix(resp, "FOUND"):
		sig := strings.TrimSpace(strings.TrimSuffix(resp, "FOUND"))
		if i := strings.Index(sig, ":"); i >= 0 {
			sig = strings.TrimSpace(sig[i+1:])
		}
		return Result{Infected: true, Signature: sig}, nil
	}
	return Result{}, fmt.Errorf("%w: unexpected reply %q", ErrUnavailable, resp)
}
