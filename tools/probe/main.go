package main

import (
	"fmt"
	"os"

	"github.com/xuri/excelize/v2"
)

func main() {
	f, err := excelize.OpenFile(os.Args[1])
	if err != nil {
		fmt.Println("open err:", err)
		return
	}
	defer f.Close()
	for _, ref := range []string{"A55", "B55", "F55", "L55", "O55", "P55", "P56", "A58", "L58"} {
		raw, e1 := f.GetCellValue("MPS", ref, excelize.Options{RawCellValue: true})
		fmtd, e2 := f.GetCellValue("MPS", ref)
		fmt.Printf("%-5s raw=%-12q fmt=%-12q err=%v/%v\n", ref, raw, fmtd, e1, e2)
	}
	rows, err := f.GetRows("MPS", excelize.Options{RawCellValue: true})
	fmt.Println("GetRows len:", len(rows), "err:", err)
	if len(rows) > 57 {
		fmt.Printf("row58 first 15: %q\n", rows[57][:min(15, len(rows[57]))])
	}
}
func min(a, b int) int { if a < b { return a }; return b }
