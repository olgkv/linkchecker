package pdf

import (
	"bytes"
	"fmt"

	"github.com/olgkv/linkchecker/internal/domain"

	"github.com/jung-kurt/gofpdf"
)

func BuildLinksReport(tasks []*domain.Task) ([]byte, error) {
	p := gofpdf.New("P", "mm", "A4", "")
	p.AddPage()
	p.SetFont("Arial", "", 12)

	p.Cell(40, 10, "Links report")
	p.Ln(12)

	for _, t := range tasks {
		p.Cell(40, 10, fmt.Sprintf("Task #%d", t.ID))
		p.Ln(8)
		for _, link := range t.Links {
			status := t.Result[link]
			if status == "" {
				status = string(domain.StatusNotAvailable)
			}
			p.Cell(40, 8, fmt.Sprintf("%s - %s", link, status))
			p.Ln(8)
		}
		p.Ln(4)
	}

	var buf bytes.Buffer
	if err := p.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
