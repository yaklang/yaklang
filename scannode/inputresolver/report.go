package inputresolver

import (
	"archive/zip"
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	excelize "github.com/xuri/excelize/v2"
)

const (
	maxReportLineCount      = 500
	maxReportLineScanBytes  = 2 << 20
	maxOfficeArchiveEntries = 2048
	maxOfficeExpandedBytes  = 32 << 20
	maxOfficeSheets         = 32
	maxOfficeRows           = 2000
	maxOfficeCells          = 50000
	maxOfficeExtractedBytes = 1 << 20
)

// ReadLines exposes a bounded, one-based line window without revealing the
// resolver's host path. It is intentionally separate from byte-offset reads.
func (w *Workspace) ReadLines(ctx context.Context, name string, startLine, endLine int) (map[string]any, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	resource, err := w.resource(name)
	if err != nil {
		return nil, err
	}
	if startLine <= 0 {
		startLine = 1
	}
	if endLine <= 0 {
		endLine = startLine + 199
	}
	if endLine < startLine || endLine-startLine+1 > maxReportLineCount {
		return nil, fail("input_range_invalid", resource.ResourceId)
	}
	file, err := w.open(resource)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReader(io.LimitReader(file, maxReportLineScanBytes+1))
	lines := make([]string, 0, endLine-startLine+1)
	lineNumber := 0
	bytesRead := 0
	for {
		if err := w.check(ctx); err != nil {
			return nil, err
		}
		line, readErr := reader.ReadString('\n')
		bytesRead += len(line)
		if bytesRead > maxReportLineScanBytes {
			return nil, fail("input_range_invalid", resource.ResourceId)
		}
		if line != "" {
			if !utf8.ValidString(line) || strings.ContainsRune(line, '\x00') {
				return nil, fail("input_text_required", resource.ResourceId)
			}
			lineNumber++
			if lineNumber >= startLine && lineNumber <= endLine {
				lines = append(lines, strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"))
			}
		}
		if lineNumber >= endLine || readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fail("input_read_failed", resource.ResourceId)
		}
	}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	w.event("input.file.access", Event{ResourceID: resource.ResourceId, Path: name, Operation: "read_lines", BytesRead: int64(bytesRead)})
	return map[string]any{
		"path": name, "resource_id": resource.ResourceId, "start_line": startLine,
		"end_line": startLine + len(lines) - 1, "lines": lines,
		"truncated": lineNumber >= endLine && uint64(bytesRead) < resource.SizeBytes,
	}, nil
}

// ExtractOfficeText accepts the explicitly supported report inputs. XLSX is
// parsed in-process only after archive limits and active-content exclusions;
// text and JSON use the same UTF-8 bounded reader as ordinary input access.
func (w *Workspace) ExtractOfficeText(ctx context.Context, name string) (map[string]any, error) {
	extension := strings.ToLower(filepath.Ext(name))
	if extension != ".xlsx" {
		if extension != ".txt" && extension != ".json" && extension != ".md" && extension != ".csv" && extension != ".log" {
			return nil, fail("input_text_required", "")
		}
		result, err := w.Read(ctx, name, 0, MaxReadBytes)
		if err != nil {
			return nil, err
		}
		result["format"] = strings.TrimPrefix(extension, ".")
		return result, nil
	}

	w.mu.RLock()
	defer w.mu.RUnlock()
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	resource, err := w.resource(name)
	if err != nil {
		return nil, err
	}
	file, err := w.open(resource)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := validateOfficeArchive(file, int64(resource.SizeBytes)); err != nil {
		return nil, fail("input_text_required", resource.ResourceId)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, fail("input_read_failed", resource.ResourceId)
	}
	workbook, err := excelize.OpenReader(file, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, fail("input_text_required", resource.ResourceId)
	}
	defer workbook.Close()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 || len(sheets) > maxOfficeSheets {
		return nil, fail("input_text_required", resource.ResourceId)
	}
	var output strings.Builder
	rowsRead, cellsRead := 0, 0
	truncated := false
	for _, sheet := range sheets {
		if ctx.Err() != nil || w.ctx.Err() != nil {
			return nil, fail("input_cancelled", resource.ResourceId)
		}
		if output.Len() > 0 {
			output.WriteString("\n")
		}
		output.WriteString("## Sheet: ")
		output.WriteString(sheet)
		output.WriteString("\n")
		rows, rowsErr := workbook.Rows(sheet)
		if rowsErr != nil {
			return nil, fail("input_read_failed", resource.ResourceId)
		}
		for rows.Next() {
			if rowsRead >= maxOfficeRows || cellsRead >= maxOfficeCells || output.Len() >= maxOfficeExtractedBytes {
				truncated = true
				break
			}
			columns, columnsErr := rows.Columns()
			if columnsErr != nil {
				rows.Close()
				return nil, fail("input_read_failed", resource.ResourceId)
			}
			for index := range columns {
				columns[index] = strings.ReplaceAll(strings.ReplaceAll(columns[index], "\t", " "), "\n", " ")
			}
			line := strings.Join(columns, "\t") + "\n"
			if output.Len()+len(line) > maxOfficeExtractedBytes {
				truncated = true
				break
			}
			output.WriteString(line)
			rowsRead++
			cellsRead += len(columns)
		}
		if closeErr := rows.Close(); closeErr != nil {
			return nil, fail("input_read_failed", resource.ResourceId)
		}
		if truncated {
			break
		}
	}
	w.event("input.file.access", Event{ResourceID: resource.ResourceId, Path: name, Operation: "extract_xlsx", BytesRead: int64(resource.SizeBytes)})
	return map[string]any{
		"path": name, "resource_id": resource.ResourceId, "format": "xlsx",
		"content": output.String(), "sheets": sheets, "rows": rowsRead,
		"cells": cellsRead, "truncated": truncated,
	}, nil
}

func validateOfficeArchive(reader io.ReaderAt, size int64) error {
	archive, err := zip.NewReader(reader, size)
	if err != nil || len(archive.File) == 0 || len(archive.File) > maxOfficeArchiveEntries {
		return fmt.Errorf("invalid Office archive")
	}
	var expanded uint64
	for _, file := range archive.File {
		name := strings.ToLower(strings.ReplaceAll(file.Name, "\\", "/"))
		if strings.HasPrefix(name, "xl/externallinks/") || strings.Contains(name, "vbaproject") || strings.HasSuffix(name, ".bin") {
			return fmt.Errorf("active or external Office content is not allowed")
		}
		if file.UncompressedSize64 > maxOfficeExpandedBytes || expanded+file.UncompressedSize64 > maxOfficeExpandedBytes {
			return fmt.Errorf("Office archive exceeds extraction limits")
		}
		expanded += file.UncompressedSize64
	}
	return nil
}
