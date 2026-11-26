// Package pdfdoc renders Quests documents (a note, a quest) to PDF in pure Go.
//
// The client sends a Document: a title, a few meta chips and blocks of markdown,
// steps, facts and lists. Markdown is parsed with goldmark and laid out here on
// top of go-pdf/fpdf with embedded DejaVu fonts (Cyrillic included); code is
// highlighted with chroma. Nothing is fetched from the network or the disk:
// images come only through the ImageLoader the caller supplies.
package pdfdoc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Document is what one PDF is made of.
type Document struct {
	Options  *Options   `json:"options,omitempty"`
	Title    string     `json:"title"`
	Book     bool       `json:"book,omitempty"`
	Chapters []Chapter  `json:"chapters,omitempty"`
	Meta     []MetaChip `json:"meta,omitempty"`
	// Footnote is a small gray line under the title ("Выгрузка: …").
	Footnote string  `json:"footnote,omitempty"`
	Blocks   []Block `json:"blocks"`
	// Labels turn journal refs in markdown (`note=12`) into titles: "note:12" → "Деплой".
	Labels        map[string]string `json:"labels,omitempty"`
	ReferenceBase string            `json:"reference_base,omitempty"`
	// Diagrams are client-rendered mermaid diagrams: the browser draws each
	// ```mermaid block and sends the PNG keyed by the trimmed source. The renderer
	// embeds the raster where the source matches; without it (server-side export,
	// failed render) the PDF keeps the diagram source with the usual marker.
	Diagrams []Diagram `json:"diagrams,omitempty"`
}

// Diagram is one client-rendered mermaid diagram: the exact trimmed source of the
// ```mermaid block and the PNG raster the browser drew from it.
type Diagram struct {
	Source string `json:"source"`
	PNG    []byte `json:"png"`
}

// diagramIndex keys the PNGs by the sha256 of the trimmed source. Oversized
// entries are dropped (those diagrams fall back to source); a total over the cap
// is a request error, like the other document limits.
func (doc Document) diagramIndex() (map[string][]byte, error) {
	if len(doc.Diagrams) == 0 {
		return nil, nil
	}
	out := make(map[string][]byte, len(doc.Diagrams))
	total := 0
	for _, d := range doc.Diagrams {
		if len(out) >= MaxDiagrams {
			break
		}
		src := strings.TrimSpace(d.Source)
		if src == "" || len(d.PNG) == 0 || len(d.PNG) > MaxDiagramBytes {
			continue
		}
		sum := sha256.Sum256([]byte(src))
		out[hex.EncodeToString(sum[:])] = d.PNG
		total += len(d.PNG)
		if total > MaxDiagramsBytes {
			return nil, fmt.Errorf("pdfdoc: diagrams exceed %d MB total", MaxDiagramsBytes>>20)
		}
	}
	return out, nil
}

// MetaChip is one fragment of the line under the title; Color is #rrggbb or empty.
type MetaChip struct {
	Text  string `json:"text"`
	Color string `json:"color,omitempty"`
}

// Block types.
const (
	BlockMarkdown = "markdown"
	BlockSteps    = "steps"
	BlockFacts    = "facts"
	BlockList     = "list"
)

// Block is one part of a document. Label, when set, is printed above it as a section heading.
type Block struct {
	Type     string   `json:"type"`
	Label    string   `json:"label,omitempty"`
	Markdown string   `json:"markdown,omitempty"`
	Steps    []Step   `json:"steps,omitempty"`
	Facts    []Fact   `json:"facts,omitempty"`
	Items    []string `json:"items,omitempty"`
	// Empty is shown instead of the block when it has no content ("Шагов нет").
	Empty string `json:"empty,omitempty"`
}

// Step is one row of a quest's step list.
type Step struct {
	Title    string `json:"title"`
	Progress string `json:"progress,omitempty"`
	Done     bool   `json:"done,omitempty"`
	Markdown string `json:"markdown,omitempty"`
}

// Fact is a label/value pair ("Создан" → "10 окт. 2026").
type Fact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ImageLoader returns the bytes of attachment id (the caller checks access and
// the type). An error renders a "not available" placeholder instead of the image.
type ImageLoader func(id int64) ([]byte, error)

// Limits bound what one request may cost.
const (
	MaxMarkdownBytes = 4 << 20
	MaxPages         = 400
	MaxImages        = 60
	MaxImageBytes    = 12 << 20
	maxImagePixels   = 40_000_000

	MaxDiagrams      = 40
	MaxDiagramBytes  = 2 << 20
	MaxDiagramsBytes = 8 << 20
)

// Chapter retains the source note's identity and position in its tree.
type Chapter struct {
	NoteID   int64  `json:"note_id"`
	Title    string `json:"title"`
	Depth    int    `json:"depth"`
	Markdown string `json:"markdown"`
}
