package pdfdoc

import (
	"embed"

	"github.com/go-pdf/fpdf"
)

// DejaVu (Bitstream Vera licence, fonts/LICENSE): the same family the previous WeasyPrint image shipped,
// with Latin, Cyrillic and the check/circle marks used in step lists.
//
//go:embed fonts/*.ttf
var fontFS embed.FS

const (
	famSerif = "dvserif"
	famSans  = "dvsans"
	famMono  = "dvmono"
)

func registerFonts(pdf *fpdf.Fpdf) error {
	for _, f := range []struct{ fam, style, file string }{
		{famSerif, "", "DejaVuSerif.ttf"},
		{famSerif, "B", "DejaVuSerif-Bold.ttf"},
		{famSerif, "I", "DejaVuSerif-Italic.ttf"},
		{famSerif, "BI", "DejaVuSerif-BoldItalic.ttf"},
		{famSans, "", "DejaVuSans.ttf"},
		{famSans, "B", "DejaVuSans-Bold.ttf"},
		{famMono, "", "DejaVuSansMono.ttf"},
		{famMono, "B", "DejaVuSansMono-Bold.ttf"},
	} {
		b, err := fontFS.ReadFile("fonts/" + f.file)
		if err != nil {
			return err
		}
		pdf.AddUTF8FontFromBytes(f.fam, f.style, b)
	}
	return pdf.Error()
}
