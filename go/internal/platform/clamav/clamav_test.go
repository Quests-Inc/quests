package clamav

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
)

// fakeClamd speaks the server half of the protocol: read the command, drain
// length-prefixed chunks until the zero terminator, then answer with reply.
func fakeClamd(t *testing.T, reply string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				cmd, err := readUntilNUL(c)
				if err != nil {
					return
				}
				if strings.Contains(cmd, "INSTREAM") {
					if err := drainChunks(c); err != nil {
						return
					}
				}
				_, _ = c.Write([]byte(reply + "\x00"))
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func readUntilNUL(c net.Conn) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			if buf[0] == 0 {
				return sb.String(), nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			return sb.String(), err
		}
	}
}

func drainChunks(c net.Conn) error {
	var hdr [4]byte
	for {
		if _, err := io.ReadFull(c, hdr[:]); err != nil {
			return err
		}
		size := binary.BigEndian.Uint32(hdr[:])
		if size == 0 {
			return nil
		}
		if _, err := io.CopyN(io.Discard, c, int64(size)); err != nil {
			return err
		}
	}
}

func TestScanClean(t *testing.T) {
	c := New(fakeClamd(t, "stream: OK"))
	res, err := c.Scan(context.Background(), strings.NewReader("harmless content"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Infected {
		t.Error("expected a clean verdict")
	}
}

func TestScanInfected(t *testing.T) {
	c := New(fakeClamd(t, "stream: Eicar-Test-Signature FOUND"))
	res, err := c.Scan(context.Background(), strings.NewReader("whatever"))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if !res.Infected {
		t.Fatal("expected an infected verdict")
	}
	if res.Signature != "Eicar-Test-Signature" {
		t.Errorf("Signature = %q, want %q", res.Signature, "Eicar-Test-Signature")
	}
}

func TestScanErrorReplyIsNotClean(t *testing.T) {
	// The important property: a clamd-side failure must surface as an error,
	// never as Infected:false — otherwise an unscanned file reads as clean.
	c := New(fakeClamd(t, "INSTREAM size limit exceeded. ERROR"))
	_, err := c.Scan(context.Background(), strings.NewReader("too big"))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestScanLargeBodyIsChunked(t *testing.T) {
	// Larger than one chunk, so the loop's multi-chunk path is exercised.
	c := New(fakeClamd(t, "stream: OK"))
	body := strings.Repeat("a", chunkSize*2+123)
	res, err := c.Scan(context.Background(), strings.NewReader(body))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if res.Infected {
		t.Error("expected a clean verdict")
	}
}

func TestScanUnreachable(t *testing.T) {
	// Port 1 on loopback: nothing listens there.
	c := New("127.0.0.1:1")
	_, err := c.Scan(context.Background(), strings.NewReader("x"))
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestNotConfigured(t *testing.T) {
	c := New("")
	if c.Configured() {
		t.Fatal("empty address must not count as configured")
	}
	if _, err := c.Scan(context.Background(), strings.NewReader("x")); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestPing(t *testing.T) {
	c := New(fakeClamd(t, "PONG"))
	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestParseVerdict(t *testing.T) {
	tests := []struct {
		reply    string
		infected bool
		sig      string
		wantErr  bool
	}{
		{reply: "stream: OK"},
		{reply: "stream: Win.Test.EICAR_HDB-1 FOUND", infected: true, sig: "Win.Test.EICAR_HDB-1"},
		{reply: "some nonsense", wantErr: true},
		{reply: "", wantErr: true},
	}
	for _, tt := range tests {
		res, err := parseVerdict(tt.reply)
		if tt.wantErr {
			if err == nil {
				t.Errorf("%q: expected an error", tt.reply)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: unexpected error %v", tt.reply, err)
			continue
		}
		if res.Infected != tt.infected || res.Signature != tt.sig {
			t.Errorf("%q: got %+v, want infected=%v sig=%q", tt.reply, res, tt.infected, tt.sig)
		}
	}
}
