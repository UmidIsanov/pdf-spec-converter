package main

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/xuri/excelize/v2"
)

const maxColWidth = 60.0
const minColWidth = 10.0

var reInvalidSheet = regexp.MustCompile(`[\[\]:*?/\\]`)

// safeSheetName — валидное уникальное имя листа (макс. 31 символ)
func safeSheetName(filename string, used map[string]bool) string {
	name := fullShifr(filename)
	name = reInvalidSheet.ReplaceAllString(name, "_")
	if len(name) > 31 {
		name = name[:31]
	}
	orig := name
	for i := 1; used[strings.ToLower(name)]; i++ {
		suffix := fmt.Sprintf("_%d", i)
		cut := 31 - len(suffix)
		if cut > len(orig) {
			cut = len(orig)
		}
		name = orig[:cut] + suffix
	}
	used[strings.ToLower(name)] = true
	return name
}

// buildWorkbook собирает xlsx: лист «Итого по наименованиям», сводный лист и по листу на файл.
func buildWorkbook(files []FileResult) ([]byte, error) {
	// строки по файлам + все вместе
	var allRows [][]string
	type fileSheet struct {
		filename string
		rows     [][]string
	}
	var perFile []fileSheet
	for _, fr := range files {
		if fr.Error != nil {
			continue
		}
		rows := rowsForFile(fr)
		if len(rows) == 0 {
			continue
		}
		allRows = append(allRows, rows...)
		perFile = append(perFile, fileSheet{fr.Filename, rows})
	}
	if len(allRows) == 0 {
		return nil, fmt.Errorf("нет данных для записи в Excel")
	}

	f := excelize.NewFile()
	defer f.Close()

	headerStyle, _ := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"4472C4"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center", WrapText: true},
	})
	dataStyle, _ := f.NewStyle(&excelize.Style{
		Alignment: &excelize.Alignment{Vertical: "top", WrapText: true},
	})

	writeSheet := func(sheet string, headers []string, rows [][]string) {
		// шапка
		for ci, h := range headers {
			col, _ := excelize.ColumnNumberToName(ci + 1)
			f.SetCellValue(sheet, col+"1", h)
		}
		// данные
		for ri, row := range rows {
			for ci, val := range row {
				col, _ := excelize.ColumnNumberToName(ci + 1)
				cell := fmt.Sprintf("%s%d", col, ri+2)
				f.SetCellValue(sheet, cell, val)
			}
		}
		lastCol, _ := excelize.ColumnNumberToName(len(headers))
		nRows := len(rows) + 1
		// стиль шапки
		f.SetCellStyle(sheet, "A1", lastCol+"1", headerStyle)
		// стиль данных (перенос текста)
		if len(rows) > 0 {
			f.SetCellStyle(sheet, "A2", fmt.Sprintf("%s%d", lastCol, nRows), dataStyle)
		}
		// ширина колонок
		for ci, h := range headers {
			maxLen := len([]rune(h))
			for _, row := range rows {
				if ci < len(row) {
					if l := len([]rune(row[ci])); l > maxLen {
						maxLen = l
					}
				}
			}
			w := float64(maxLen + 2)
			if w < minColWidth {
				w = minColWidth
			}
			if w > maxColWidth {
				w = maxColWidth
			}
			col, _ := excelize.ColumnNumberToName(ci + 1)
			f.SetColWidth(sheet, col, col, w)
		}
		// закрепить шапку
		f.SetPanes(sheet, &excelize.Panes{
			Freeze:      true,
			YSplit:      1,
			TopLeftCell: "A2",
			ActivePane:  "bottomLeft",
		})
		// автофильтр
		f.AutoFilter(sheet, fmt.Sprintf("A1:%s%d", lastCol, nRows), []excelize.AutoFilterOptions{})
	}

	headers := allHeaders()

	// 1) Итого по наименованиям
	totalSheet := "Итого по наименованиям"
	f.SetSheetName("Sheet1", totalSheet)
	writeSheet(totalSheet, summaryColumns, summarizeByName(files))

	// 2) Сводная спецификация
	summarySheet := "Сводная спецификация"
	f.NewSheet(summarySheet)
	writeSheet(summarySheet, headers, allRows)

	// 3) По листу на файл
	used := map[string]bool{
		strings.ToLower(totalSheet):   true,
		strings.ToLower(summarySheet): true,
	}
	for _, fs := range perFile {
		name := safeSheetName(fs.filename, used)
		f.NewSheet(name)
		writeSheet(name, headers, fs.rows)
	}

	f.SetActiveSheet(0)

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
