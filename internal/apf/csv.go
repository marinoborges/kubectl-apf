package apf

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

func parseRecords(data []byte) ([]map[string]string, error) {
	reader := csv.NewReader(bytes.NewReader(data))
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true

	rows, err := reader.ReadAll()
	if err != nil && err != io.EOF {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty debug response")
	}

	header := make([]string, len(rows[0]))
	for i, column := range rows[0] {
		header[i] = strings.TrimSpace(column)
	}

	records := make([]map[string]string, 0, len(rows)-1)
	for _, row := range rows[1:] {
		if len(strings.TrimSpace(strings.Join(row, ""))) == 0 {
			continue
		}
		record := make(map[string]string, len(header))
		for i, column := range header {
			if column == "" || i >= len(row) {
				continue
			}
			record[column] = strings.TrimSpace(row[i])
		}
		records = append(records, record)
	}
	return records, nil
}

func field(record map[string]string, names ...string) string {
	for _, name := range names {
		if value, ok := record[name]; ok {
			return value
		}
	}
	return ""
}
