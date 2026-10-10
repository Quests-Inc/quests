// Package pdfdoc renders Quests documents (a note, a quest) to PDF in pure Go.
//
// The client sends a Document: a title, a few meta chips and blocks of markdown,
// steps, facts and lists. Markdown is parsed with goldmark and laid out here on
// top of go-pdf/fpdf with embedded DejaVu fonts (Cyrillic included); code is
// highlighted with chroma. Nothing is fetched from the network or the disk:
// images come only through the ImageLoader the caller supplies.
package pdfdoc

// Document is what one PDF is made of.
type Document struct {
	Title string     `json:"title"`
	Meta  []MetaChip `json:"meta,omitempty"`
	// Footnote is a small gray line under the title ("Выгрузка: …").
	Footnote string  `json:"footnote,omitempty"`
	Blocks   []Block `json:"blocks"`
	// Labels turn journal refs in markdown (`note=12`) into titles: "note:12" → "Деплой".
	Labels map[string]string `json:"labels,omitempty"`
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
)
