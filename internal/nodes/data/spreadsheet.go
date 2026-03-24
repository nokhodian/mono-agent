package data

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	"github.com/monoes/monoes-agent/internal/workflow"
	"github.com/xuri/excelize/v2"
)

// SpreadsheetNode reads or writes CSV/XLSX files.
// Type: "data.spreadsheet"
type SpreadsheetNode struct{}

func (n *SpreadsheetNode) Type() string { return "data.spreadsheet" }

func (n *SpreadsheetNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	operation, _ := config["operation"].(string)
	filePath, _ := config["file_path"].(string)
	sheet, _ := config["sheet"].(string)

	if sheet == "" {
		sheet = "Sheet1"
	}

	// has_header defaults to true
	hasHeader := true
	if v, ok := config["has_header"].(bool); ok {
		hasHeader = v
	}

	switch operation {
	case "read_csv":
		return n.readCSV(filePath, hasHeader)
	case "write_csv":
		return n.writeCSV(filePath, input.Items, hasHeader)
	case "read_xlsx":
		return n.readXLSX(filePath, sheet, hasHeader)
	case "write_xlsx":
		return n.writeXLSX(filePath, sheet, input.Items, hasHeader)
	default:
		return nil, fmt.Errorf("data.spreadsheet: unknown operation %q", operation)
	}
}

func (n *SpreadsheetNode) readCSV(filePath string, hasHeader bool) ([]workflow.NodeOutput, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("data.spreadsheet read_csv: open %q: %w", filePath, err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("data.spreadsheet read_csv: read: %w", err)
	}

	items := rowsToItems(rows, hasHeader)
	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

func (n *SpreadsheetNode) writeCSV(filePath string, items []workflow.Item, hasHeader bool) ([]workflow.NodeOutput, error) {
	f, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("data.spreadsheet write_csv: create %q: %w", filePath, err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	rows, headers := itemsToRows(items, hasHeader)
	if hasHeader && len(headers) > 0 {
		if err := writer.Write(headers); err != nil {
			return nil, fmt.Errorf("data.spreadsheet write_csv: write header: %w", err)
		}
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("data.spreadsheet write_csv: write row: %w", err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("data.spreadsheet write_csv: flush: %w", err)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{
		workflow.NewItem(map[string]interface{}{"rows_written": len(rows)}),
	}}}, nil
}

func (n *SpreadsheetNode) readXLSX(filePath, sheet string, hasHeader bool) ([]workflow.NodeOutput, error) {
	xl, err := excelize.OpenFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("data.spreadsheet read_xlsx: open %q: %w", filePath, err)
	}
	defer xl.Close()

	rows, err := xl.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("data.spreadsheet read_xlsx: get rows from sheet %q: %w", sheet, err)
	}

	items := rowsToItems(rows, hasHeader)
	return []workflow.NodeOutput{{Handle: "main", Items: items}}, nil
}

func (n *SpreadsheetNode) writeXLSX(filePath, sheet string, items []workflow.Item, hasHeader bool) ([]workflow.NodeOutput, error) {
	xl := excelize.NewFile()
	defer xl.Close()

	// Rename default sheet or create new
	defaultSheet := xl.GetSheetName(0)
	if defaultSheet != sheet {
		xl.SetSheetName(defaultSheet, sheet)
	}

	rows, headers := itemsToRows(items, hasHeader)
	rowNum := 1

	if hasHeader && len(headers) > 0 {
		for col, h := range headers {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowNum)
			xl.SetCellValue(sheet, cell, h)
		}
		rowNum++
	}

	for _, row := range rows {
		for col, val := range row {
			cell, _ := excelize.CoordinatesToCellName(col+1, rowNum)
			xl.SetCellValue(sheet, cell, val)
		}
		rowNum++
	}

	if err := xl.SaveAs(filePath); err != nil {
		return nil, fmt.Errorf("data.spreadsheet write_xlsx: save %q: %w", filePath, err)
	}

	return []workflow.NodeOutput{{Handle: "main", Items: []workflow.Item{
		workflow.NewItem(map[string]interface{}{"rows_written": len(rows)}),
	}}}, nil
}

// rowsToItems converts a 2D slice of strings into workflow Items.
// If hasHeader is true, the first row is used as field names.
func rowsToItems(rows [][]string, hasHeader bool) []workflow.Item {
	if len(rows) == 0 {
		return nil
	}

	var headers []string
	startRow := 0

	if hasHeader {
		headers = rows[0]
		startRow = 1
	}

	items := make([]workflow.Item, 0, len(rows)-startRow)
	for i := startRow; i < len(rows); i++ {
		row := rows[i]
		data := make(map[string]interface{})
		if hasHeader {
			for j, h := range headers {
				if j < len(row) {
					data[h] = row[j]
				} else {
					data[h] = ""
				}
			}
		} else {
			for j, val := range row {
				data[strconv.Itoa(j)] = val
			}
		}
		items = append(items, workflow.NewItem(data))
	}
	return items
}

// itemsToRows converts workflow Items to a 2D string slice and header slice.
func itemsToRows(items []workflow.Item, hasHeader bool) ([][]string, []string) {
	if len(items) == 0 {
		return nil, nil
	}

	// Collect all unique keys to form stable headers
	keyOrder := make([]string, 0)
	keySet := make(map[string]bool)
	for _, item := range items {
		for k := range item.JSON {
			if !keySet[k] {
				keySet[k] = true
				keyOrder = append(keyOrder, k)
			}
		}
	}

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		row := make([]string, len(keyOrder))
		for i, k := range keyOrder {
			if v, ok := item.JSON[k]; ok {
				row[i] = fmt.Sprintf("%v", v)
			}
		}
		rows = append(rows, row)
	}

	return rows, keyOrder
}
