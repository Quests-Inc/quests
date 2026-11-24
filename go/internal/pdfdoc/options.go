package pdfdoc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
)

// Options are document-local; omitted options preserve the historical export.
type Options struct {
	Width             float64 `json:"width"`
	Height            float64 `json:"height"`
	Landscape         bool    `json:"landscape"`
	Top               float64 `json:"top"`
	Right             float64 `json:"right"`
	Bottom            float64 `json:"bottom"`
	Left              float64 `json:"left"`
	Font              string  `json:"font"`
	FontSize          float64 `json:"font_size"`
	LineSpacing       float64 `json:"line_spacing"`
	TextColor         string  `json:"text_color"`
	BackgroundColor   string  `json:"background_color"`
	BackgroundImage   int64   `json:"background_image"`
	BackgroundOpacity float64 `json:"background_opacity"`
	BackgroundFit     string  `json:"background_fit"`
	NumberFormat      string  `json:"number_format"`
	NumberPosition    string  `json:"number_position"`
	NumberAlign       string  `json:"number_align"`
	ChapterNewPage    bool    `json:"chapter_new_page"`
	MirrorMargins     bool    `json:"mirror_margins"`
	Binding           float64 `json:"binding"`
}

func DefaultOptions() Options {
	return Options{Width: 210, Height: 297, Top: 16, Right: 16, Bottom: 18, Left: 16, Font: "serif", FontSize: 10, LineSpacing: 1.45, TextColor: "#1a1a1a", BackgroundColor: "#ffffff", BackgroundOpacity: 1, BackgroundFit: "contain", NumberFormat: "total", NumberPosition: "bottom", NumberAlign: "center", ChapterNewPage: true}
}

func (o *Options) UnmarshalJSON(data []byte) error {
	type plain Options
	value := plain(DefaultOptions())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*o = Options(value)
	return nil
}

type InvalidOptionsError struct{ Message string }

func (e *InvalidOptionsError) Error() string { return "Настройки PDF: " + e.Message }

var colorHex = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (o Options) validate() error {
	bad := func(message string) error { return &InvalidOptionsError{message} }
	between := func(v, lo, hi float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= lo && v <= hi }
	if !between(o.Width, 80, 600) || !between(o.Height, 80, 600) {
		return bad("ширина и высота должны быть от 80 до 600 мм")
	}
	for _, v := range []float64{o.Top, o.Right, o.Bottom, o.Left, o.Binding} {
		if !between(v, 0, 100) {
			return bad("поля и переплёт должны быть от 0 до 100 мм")
		}
	}
	w, h := o.dimensions()
	if w-o.Left-o.Right-o.Binding < 40 || h-o.Top-o.Bottom < 40 {
		return bad("для содержимого должно оставаться не менее 40 × 40 мм")
	}
	if !between(o.FontSize, 6, 24) || !between(o.LineSpacing, 1, 2.5) {
		return bad("размер шрифта 6–24 пт, межстрочный интервал 1–2,5")
	}
	if !colorHex.MatchString(o.TextColor) || !colorHex.MatchString(o.BackgroundColor) {
		return bad("цвет должен иметь формат #rrggbb")
	}
	if o.Font != "serif" && o.Font != "sans" && o.Font != "mono" && o.Font != "gothic" {
		return bad("неизвестный шрифт")
	}
	if o.NumberFormat != "none" && o.NumberFormat != "page" && o.NumberFormat != "total" {
		return bad("неизвестный формат нумерации")
	}
	if o.NumberPosition != "top" && o.NumberPosition != "bottom" {
		return bad("неизвестное положение нумерации")
	}
	if o.NumberAlign != "left" && o.NumberAlign != "center" && o.NumberAlign != "right" {
		return bad("неизвестное выравнивание нумерации")
	}
	if o.NumberFormat != "none" && ((o.NumberPosition == "top" && o.Top < 10) || (o.NumberPosition == "bottom" && o.Bottom < 10)) {
		return bad("со стороны номера страницы нужно поле не менее 10 мм")
	}
	if o.BackgroundImage < 0 || !between(o.BackgroundOpacity, 0, 1) || (o.BackgroundFit != "contain" && o.BackgroundFit != "cover") {
		return bad("неверные параметры фонового изображения")
	}
	return nil
}
func (o Options) dimensions() (float64, float64) {
	w, h := o.Width, o.Height
	if o.Landscape && h > w || !o.Landscape && w > h {
		w, h = h, w
	}
	return w, h
}
func (r *Renderer) pageWidth() float64 {
	if r.options.Width == 0 {
		return pageW
	}
	w, _ := r.options.dimensions()
	return w
}
func (r *Renderer) pageHeight() float64 {
	if r.options.Height == 0 {
		return pageH
	}
	_, h := r.options.dimensions()
	return h
}
func (r *Renderer) topMargin() float64 {
	if r.options.Width == 0 {
		return marginTop
	}
	return r.options.Top
}
func (r *Renderer) bottomMargin() float64 {
	if r.options.Width == 0 {
		return marginBottom
	}
	return r.options.Bottom
}
func (r *Renderer) leftMargin(page int) float64 {
	if r.options.Width == 0 {
		return marginX
	}
	if r.options.MirrorMargins && page%2 == 0 {
		return r.options.Right
	}
	return r.options.Left + r.options.Binding
}
func (r *Renderer) contentWidth() float64 {
	if r.options.Width == 0 {
		return pageW - 2*marginX
	}
	return r.pageWidth() - r.options.Left - r.options.Right - r.options.Binding
}
func (r *Renderer) fontSize(size float64) float64 {
	if r.options.FontSize == 0 {
		return size
	}
	return size * r.options.FontSize / 10
}
func (r *Renderer) textColor() rgb {
	if r.options.TextColor == "" {
		return colText
	}
	return hexColor(r.options.TextColor, colText)
}
func (r *Renderer) bodyStyle() style {
	st := bodyStyle()
	st.size = r.fontSize(10)
	st.color = r.textColor()
	switch r.options.Font {
	case "sans":
		st.fam = famSans
	case "mono":
		st.fam = famMono
	case "gothic":
		st.fam = famGothic
	}
	return st
}

func (r *Renderer) prepareBackground() error {
	if r.options.BackgroundImage == 0 || r.options.BackgroundOpacity == 0 {
		return nil
	}
	if r.loader == nil {
		return &InvalidOptionsError{"фоновое вложение недоступно"}
	}
	raw, err := r.loader(r.options.BackgroundImage)
	if err != nil {
		return &InvalidOptionsError{"фоновое вложение недоступно"}
	}
	data, typ, w, h, err := prepareImage(raw)
	if err != nil {
		return &InvalidOptionsError{"фоновое вложение не является доступным изображением"}
	}
	r.pdf.RegisterImageOptionsReader("background", imageOptions(typ), bytes.NewReader(data))
	r.backgroundW, r.backgroundH = float64(w), float64(h)
	return r.pdf.Error()
}
func (r *Renderer) paintBackground() {
	if r.options.Width == 0 {
		return
	}
	c := hexColor(r.options.BackgroundColor, rgb{255, 255, 255})
	r.pdf.SetFillColor(c[0], c[1], c[2])
	r.pdf.Rect(0, 0, r.pageWidth(), r.pageHeight(), "F")
	if r.backgroundW == 0 {
		return
	}
	scale := math.Min(r.pageWidth()/r.backgroundW, r.pageHeight()/r.backgroundH)
	if r.options.BackgroundFit == "cover" {
		scale = math.Max(r.pageWidth()/r.backgroundW, r.pageHeight()/r.backgroundH)
	}
	w, h := r.backgroundW*scale, r.backgroundH*scale
	r.pdf.ClipRect(0, 0, r.pageWidth(), r.pageHeight(), false)
	r.pdf.SetAlpha(r.options.BackgroundOpacity, "Normal")
	r.pdf.ImageOptions("background", (r.pageWidth()-w)/2, (r.pageHeight()-h)/2, w, h, false, imageOptions(""), 0, "")
	r.pdf.SetAlpha(1, "Normal")
	r.pdf.ClipEnd()
}
func (r *Renderer) pageNumber() {
	o := r.options
	if o.NumberFormat == "none" {
		return
	}
	page := r.pdf.PageNo()
	label := fmt.Sprintf("%d", page)
	if o.NumberFormat == "total" {
		label += " / {nb}"
	}
	y := r.pageHeight() - math.Max(7, o.Bottom/2) - 2
	if o.NumberPosition == "top" {
		y = math.Max(3, o.Top/2-2)
	}
	r.pdf.SetXY(r.leftMargin(page), y)
	r.pdf.SetFont(famSans, "", 7.5)
	r.pdf.SetTextColor(colMuted[0], colMuted[1], colMuted[2])
	align := map[string]string{"left": "L", "center": "C", "right": "R"}[o.NumberAlign]
	r.pdf.CellFormat(r.contentWidth(), 4, label, "", 0, align, false, 0, "")
}

func (r *Renderer) headingFamily() string {
	if r.options.Font == "gothic" {
		return famGothic
	}
	return famSans
}
