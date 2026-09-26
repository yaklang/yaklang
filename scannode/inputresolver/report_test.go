package inputresolver

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
)

func TestReportWorkspaceReadsLinesAndExtractsXLSXWithProvenance(t *testing.T) {
	root := t.TempDir()
	inputDir := filepath.Join(root, "inputs", "field_report")
	if err := os.MkdirAll(inputDir, 0700); err != nil {
		t.Fatal(err)
	}
	textPath := filepath.Join(inputDir, "01-evidence.json")
	textContent := "{\"fact\":\"alpha\"}\n{\"fact\":\"beta\"}\n"
	if err := os.WriteFile(textPath, []byte(textContent), 0400); err != nil {
		t.Fatal(err)
	}

	workbook := excelize.NewFile()
	if err := workbook.SetCellValue("Sheet1", "A1", "host"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue("Sheet1", "B1", "severity"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue("Sheet1", "A2", "owned.example"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue("Sheet1", "B2", "high"); err != nil {
		t.Fatal(err)
	}
	xlsxPath := filepath.Join(inputDir, "02-evidence.xlsx")
	if err := workbook.SaveAs(xlsxPath); err != nil {
		t.Fatal(err)
	}
	if err := workbook.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(xlsxPath, 0400); err != nil {
		t.Fatal(err)
	}

	textInfo, err := os.Stat(textPath)
	if err != nil {
		t.Fatal(err)
	}
	xlsxInfo, err := os.Stat(xlsxPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	workspace := &Workspace{
		root: root, ctx: ctx, cancel: cancel,
		manifest: &aiv1.InputManifest{Resources: []*aiv1.InputResource{
			{ResourceId: "text-1", RelativePath: "inputs/field_report/01-evidence.json", SizeBytes: uint64(textInfo.Size())},
			{ResourceId: "xlsx-1", RelativePath: "inputs/field_report/02-evidence.xlsx", SizeBytes: uint64(xlsxInfo.Size())},
		}},
	}

	lines, err := workspace.ReadLines(ctx, "inputs/field_report/01-evidence.json", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := lines["lines"].([]string); len(got) != 1 || !strings.Contains(got[0], "beta") {
		t.Fatalf("unexpected bounded lines: %#v", lines)
	}
	extracted, err := workspace.ExtractOfficeText(ctx, "inputs/field_report/02-evidence.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	content, _ := extracted["content"].(string)
	if !strings.Contains(content, "owned.example") || !strings.Contains(content, "high") || extracted["format"] != "xlsx" {
		t.Fatalf("unexpected XLSX extraction: %#v", extracted)
	}
	refs := workspace.MaterialReferences()
	if len(refs) != 2 || refs[0].Operations[0] != "read_lines" || refs[1].Operations[0] != "extract_xlsx" {
		t.Fatalf("unexpected material provenance: %#v", refs)
	}
}

func TestValidateOfficeArchiveRejectsActiveContent(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("xl/vbaProject.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("macro")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := validateOfficeArchive(bytes.NewReader(buffer.Bytes()), int64(buffer.Len())); err == nil || !strings.Contains(err.Error(), "active") {
		t.Fatalf("active Office content error = %v", err)
	}
}
