package output

import (
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

// Sheet is a single worksheet.
type Sheet struct {
	Name    string
	Headers []string
	Rows    [][]any // strings and numbers
	// Highlight marks rows (by index) to fill in light red.
	Highlight []bool
}

// XLSX writes the sheet as an Excel workbook. The header row is bold on a
// light blue background and frozen.
func XLSX(w io.Writer, s Sheet) error {
	f := excelize.NewFile()
	defer f.Close()

	if err := f.SetSheetName("Sheet1", s.Name); err != nil {
		return err
	}
	header, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#DDEBF7"}},
	})
	if err != nil {
		return err
	}
	highlight, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Color: "#9C0006"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"#FFC7CE"}},
	})
	if err != nil {
		return err
	}

	headers := make([]any, len(s.Headers))
	for i, h := range s.Headers {
		headers[i] = h
	}
	if err := f.SetSheetRow(s.Name, "A1", &headers); err != nil {
		return err
	}
	last, err := excelize.CoordinatesToCellName(len(s.Headers), 1)
	if err != nil {
		return err
	}
	if err := f.SetCellStyle(s.Name, "A1", last, header); err != nil {
		return err
	}

	widths := make([]int, len(s.Headers))
	for i, h := range s.Headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for i, row := range s.Rows {
		start := fmt.Sprintf("A%d", i+2)
		if err := f.SetSheetRow(s.Name, start, &row); err != nil {
			return err
		}
		if i < len(s.Highlight) && s.Highlight[i] {
			end, err := excelize.CoordinatesToCellName(len(s.Headers), i+2)
			if err != nil {
				return err
			}
			if err := f.SetCellStyle(s.Name, start, end, highlight); err != nil {
				return err
			}
		}
		for j, v := range row {
			if j < len(widths) {
				widths[j] = max(widths[j], utf8.RuneCountInString(fmt.Sprint(v)))
			}
		}
	}
	for i, wd := range widths {
		col, err := excelize.ColumnNumberToName(i + 1)
		if err != nil {
			return err
		}
		if err := f.SetColWidth(s.Name, col, col, float64(min(wd, 80)+2)); err != nil {
			return err
		}
	}

	if err := f.SetPanes(s.Name, &excelize.Panes{
		Freeze:      true,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return err
	}
	_, err = f.WriteTo(w)
	return err
}
