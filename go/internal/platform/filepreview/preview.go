// Package filepreview extracts bounded, inert text for attachment cards.
package filepreview

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

const MaxFile = 8 << 20
const MaxTextRead = 64 << 10
const MaxText = 2000

// Kind never decides whether bytes may execute: every result is plain text.
func Kind(name, mime string) string {
	ext := strings.ToLower(filepath.Ext(name))
	mime = strings.ToLower(strings.TrimSpace(strings.Split(mime, ";")[0]))
	if ext == ".pdf" || mime == "application/pdf" {
		return "pdf"
	}
	if slices.Contains([]string{".docx", ".xlsx", ".pptx", ".odt", ".ods", ".odp"}, ext) {
		return "office"
	}
	if ext == ".zip" || mime == "application/zip" {
		return "zip"
	}
	if strings.HasPrefix(mime, "text/") || slices.Contains([]string{".txt", ".md", ".csv", ".tsv", ".json", ".jsonl", ".xml", ".html", ".htm", ".svg", ".yaml", ".yml", ".toml", ".ini", ".conf", ".log", ".sql", ".py", ".js", ".ts", ".css", ".go", ".rs", ".sh", ".c", ".cpp", ".h", ".svelte"}, ext) {
		return "text"
	}
	return ""
}

func clip(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > MaxText {
		return string(r[:MaxText]) + "…"
	}
	return s
}

func text(data []byte) (string, error) {
	if len(data) >= 2 && ((data[0] == 0xff && data[1] == 0xfe) || (data[0] == 0xfe && data[1] == 0xff)) {
		var order binary.ByteOrder = binary.LittleEndian
		if data[0] == 0xfe {
			order = binary.BigEndian
		}
		units := make([]uint16, 0, (len(data)-2)/2)
		for i := 2; i+1 < len(data); i += 2 {
			units = append(units, order.Uint16(data[i:i+2]))
		}
		return clip(string(utf16.Decode(units))), nil
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return "", errors.New("binary file")
	}
	// A bounded read may stop in the middle of the last UTF-8 rune.
	for n := 0; n < 3 && len(data) > 0 && !utf8.Valid(data); n++ {
		data = data[:len(data)-1]
	}
	if !utf8.Valid(data) {
		return "", errors.New("unsupported text encoding")
	}
	return clip(strings.TrimPrefix(string(data), "\ufeff")), nil
}

// Extract reads no more than the limit for its format. ctx also bounds the PDF subprocess.
func Extract(ctx context.Context, kind string, input io.Reader) (string, error) {
	limit := int64(MaxFile)
	if kind == "text" {
		limit = MaxTextRead
	}
	data, err := io.ReadAll(io.LimitReader(input, limit+1))
	if err != nil {
		return "", err
	}
	if kind == "text" {
		if len(data) > MaxTextRead {
			data = data[:MaxTextRead]
		}
		return text(data)
	}
	if len(data) > MaxFile {
		return "", errors.New("file too large")
	}
	switch kind {
	case "pdf":
		out := &cappedOutput{}
		cmd := exec.CommandContext(ctx, "pdftotext", "-f", "1", "-l", "1", "-layout", "-enc", "UTF-8", "-", "-")
		cmd.Stdin = bytes.NewReader(data)
		cmd.Stdout = out
		if err := cmd.Run(); err != nil {
			return "", err
		}
		return clip(out.String()), nil
	case "office", "zip":
		return archiveText(ctx, data, kind)
	default:
		return "", errors.New("unsupported format")
	}
}

type cappedOutput struct{ buffer bytes.Buffer }

func (b *cappedOutput) String() string { return b.buffer.String() }

func (b *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	left := MaxTextRead - b.buffer.Len()
	if left > 0 {
		_, _ = b.buffer.Write(p[:min(left, n)])
	}
	return n, nil
}

func archiveText(ctx context.Context, data []byte, kind string) (string, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var out strings.Builder
	budget := int64(1 << 20)
	entries := 0
	for _, f := range z.File {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		entries++
		if entries > 200 {
			break
		}
		if kind == "zip" {
			out.WriteString(f.Name + "\n")
			if out.Len() > MaxTextRead {
				break
			}
			continue
		}
		n := f.Name
		selected := n == "word/document.xml" || n == "xl/sharedStrings.xml" || n == "content.xml" || strings.HasPrefix(n, "ppt/slides/slide") && strings.HasSuffix(n, ".xml") || strings.HasPrefix(n, "xl/worksheets/sheet") && strings.HasSuffix(n, ".xml")
		if !selected {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		raw, err := io.ReadAll(io.LimitReader(rc, budget+1))
		_ = rc.Close()
		if err != nil {
			return "", err
		}
		budget -= int64(len(raw))
		if budget < 0 {
			return "", errors.New("document too large")
		}
		d := xml.NewDecoder(bytes.NewReader(raw))
		for {
			token, err := d.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			if t, ok := token.(xml.CharData); ok {
				s := strings.TrimSpace(string(t))
				if s != "" {
					out.WriteString(s + " ")
				}
			}
			if out.Len() > MaxTextRead {
				return clip(out.String()), nil
			}
		}
		out.WriteByte('\n')
	}
	return clip(out.String()), nil
}
