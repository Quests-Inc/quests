package filepreview

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/go-pdf/fpdf"
)

func TestTextPreview(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{"<script>alert(1)</script>\nhello", "<script>alert(1)</script>\nhello"},
		{"\xef\xbb\xbfПривет", "Привет"},
		{"\xff\xfeH\x00i\x00", "Hi"},
	} {
		out, e := Extract(context.Background(), "text", strings.NewReader(tc.data))
		if e != nil || out != tc.want {
			t.Fatalf("%q %v", out, e)
		}
	}
	if _, e := Extract(context.Background(), "text", strings.NewReader("binary\x00bytes")); e == nil {
		t.Fatal("binary accepted")
	}
	out, e := Extract(context.Background(), "text", strings.NewReader(strings.Repeat("я", MaxTextRead)))
	if e != nil || len([]rune(out)) != MaxText+1 {
		t.Fatalf("not bounded: %d %v", len(out), e)
	}
}

func zipped(t *testing.T, name, data string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, e := w.Create(name)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write([]byte(data)); e != nil {
		t.Fatal(e)
	}
	if e = w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}

func TestArchives(t *testing.T) {
	data := zipped(t, "word/document.xml", `<document><p><t>Первый абзац</t></p><p><t>Второй</t></p></document>`)
	out, e := Extract(context.Background(), "office", bytes.NewReader(data))
	if e != nil || !strings.Contains(out, "Первый абзац Второй") {
		t.Fatalf("%q %v", out, e)
	}
	out, e = Extract(context.Background(), "zip", bytes.NewReader(data))
	if e != nil || out != "word/document.xml" {
		t.Fatalf("%q %v", out, e)
	}
	bomb := zipped(t, "word/document.xml", strings.Repeat("a", (1<<20)+1))
	if _, e = Extract(context.Background(), "office", bytes.NewReader(bomb)); e == nil {
		t.Fatal("inflated XML accepted")
	}
	if _, e = Extract(context.Background(), "zip", strings.NewReader("bad zip")); e == nil {
		t.Fatal("corrupt zip accepted")
	}
	if _, e = Extract(context.Background(), "zip", strings.NewReader(strings.Repeat("x", MaxFile+1))); e == nil {
		t.Fatal("oversized zip accepted")
	}
}

func TestPDFFirstPage(t *testing.T) {
	if _, e := exec.LookPath("pdftotext"); e != nil {
		t.Skip("install poppler-utils (poppler on Arch) for PDF previews")
	}
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetFont("Arial", "", 12)
	pdf.AddPage()
	pdf.Cell(40, 10, "FIRST_PAGE")
	pdf.AddPage()
	pdf.Cell(40, 10, "SECOND_PAGE")
	var b bytes.Buffer
	if e := pdf.Output(&b); e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	out, e := Extract(ctx, "pdf", bytes.NewReader(b.Bytes()))
	if e != nil || !strings.Contains(out, "FIRST_PAGE") || strings.Contains(out, "SECOND_PAGE") {
		t.Fatalf("%q %v", out, e)
	}
	if _, e = Extract(ctx, "pdf", strings.NewReader("broken pdf")); e == nil {
		t.Fatal("corrupt PDF accepted")
	}
}

func TestPDFOutputIsBounded(t *testing.T) {
	out := &cappedOutput{}
	if _, err := io.Copy(out, strings.NewReader(strings.Repeat("x", MaxTextRead*4))); err != nil {
		t.Fatal(err)
	}
	if len(out.String()) != MaxTextRead {
		t.Fatalf("output grew to %d bytes", len(out.String()))
	}
}
