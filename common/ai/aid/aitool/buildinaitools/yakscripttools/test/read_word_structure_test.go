package test

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	_ "github.com/yaklang/yaklang/common/yak"
	"gotest.tools/v3/assert"
)

const contentTypesXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`

const rootRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`

const docRelsXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
</Relationships>`

const documentXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p>
      <w:r>
        <w:rPr><w:b/></w:rPr>
        <w:t>Investigation Report Title</w:t>
      </w:r>
    </w:p>
    <w:p>
      <w:r>
        <w:t>This document contains the findings of the economic investigation conducted in Q1 2024.</w:t>
      </w:r>
    </w:p>
    <w:p>
      <w:r>
        <w:t>Multiple transactions were identified as suspicious and require further analysis.</w:t>
      </w:r>
    </w:p>
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>TransactionID</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Amount</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>Status</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>TXN-001</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>50000</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>flagged</w:t></w:r></w:p></w:tc>
      </w:tr>
      <w:tr>
        <w:tc><w:p><w:r><w:t>TXN-002</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>120000</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>confirmed</w:t></w:r></w:p></w:tc>
      </w:tr>
    </w:tbl>
    <w:p>
      <w:r>
        <w:t>End of report.</w:t>
      </w:r>
    </w:p>
  </w:body>
</w:document>`

func createTestDocxFile(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	docxPath := filepath.Join(tempDir, "test_report.docx")

	f, err := os.Create(docxPath)
	if err != nil {
		t.Fatalf("failed to create docx file: %v", err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	files := map[string]string{
		"[Content_Types].xml":          contentTypesXML,
		"_rels/.rels":                  rootRelsXML,
		"word/_rels/document.xml.rels": docRelsXML,
		"word/document.xml":            documentXML,
	}

	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry %s: %v", name, err)
		}
		_, err = fw.Write([]byte(content))
		if err != nil {
			t.Fatalf("failed to write zip entry %s: %v", name, err)
		}
	}

	return docxPath
}

func getReadWordStructureTool(t *testing.T) *aitool.Tool {
	t.Helper()
	embedFS := yakscripttools.GetEmbedFS()
	content, err := embedFS.ReadFile("yakscriptforai/doc/read_word_structure.yak")
	if err != nil {
		t.Fatalf("failed to read read_word_structure.yak: %v", err)
	}
	aiTool := yakscripttools.LoadYakScriptToAiTools("read_word_structure", string(content))
	if aiTool == nil {
		t.Fatalf("failed to parse read_word_structure.yak metadata")
	}
	tools := yakscripttools.ConvertTools([]*schema.AIYakTool{aiTool})
	if len(tools) == 0 {
		t.Fatalf("ConvertTools returned empty")
	}
	return tools[0]
}

func execReadWordStructureTool(t *testing.T, tool *aitool.Tool, params aitool.InvokeParams) (stdout, stderr string) {
	t.Helper()
	w1, w2 := bytes.NewBuffer(nil), bytes.NewBuffer(nil)
	_, err := tool.Callback(context.Background(), params, nil, w1, w2)
	if err != nil {
		t.Logf("tool execution error (may be expected): %v", err)
	}
	return w1.String(), w2.String()
}

func TestReadWordStructure_BasicParse(t *testing.T) {
	testFile := createTestDocxFile(t)

	tool := getReadWordStructureTool(t)
	stdout, _ := execReadWordStructureTool(t, tool, aitool.InvokeParams{
		"input": testFile,
	})

	output := stdout
	assert.Assert(t, strings.Contains(output, "Word Document"), "should contain Word Document header")
	assert.Assert(t, strings.Contains(output, "TEXT CONTENT"), "should contain TEXT CONTENT section")
	assert.Assert(t, strings.Contains(output, "Investigation Report Title"), "should contain document title text")
	assert.Assert(t, strings.Contains(output, "economic investigation"), "should contain body text")
	t.Logf("stdout:\n%s", stdout)
}

func TestReadWordStructure_TableExtraction(t *testing.T) {
	testFile := createTestDocxFile(t)

	tool := getReadWordStructureTool(t)
	stdout, _ := execReadWordStructureTool(t, tool, aitool.InvokeParams{
		"input": testFile,
	})

	output := stdout
	assert.Assert(t, strings.Contains(output, "TABLES"), "should contain TABLES section")
	assert.Assert(t, strings.Contains(output, "TransactionID"), "should contain table header TransactionID")
	assert.Assert(t, strings.Contains(output, "TXN-001"), "should contain first transaction ID")
	assert.Assert(t, strings.Contains(output, "TXN-002"), "should contain second transaction ID")
	assert.Assert(t, strings.Contains(output, "flagged"), "should contain status flagged")
	t.Logf("stdout:\n%s", stdout)
}

func TestReadWordStructure_TextOnly(t *testing.T) {
	testFile := createTestDocxFile(t)

	tool := getReadWordStructureTool(t)
	stdout, _ := execReadWordStructureTool(t, tool, aitool.InvokeParams{
		"input":     testFile,
		"text_only": true,
	})

	output := stdout
	assert.Assert(t, strings.Contains(output, "TEXT CONTENT"), "should contain TEXT CONTENT")
	assert.Assert(t, strings.Contains(output, "Investigation Report Title"), "should contain title")
	assert.Assert(t, !strings.Contains(output, "## TABLES"), "text_only should NOT contain TABLES section header")
	t.Logf("stdout:\n%s", stdout)
}

func TestReadWordStructure_TablesOnly(t *testing.T) {
	testFile := createTestDocxFile(t)

	tool := getReadWordStructureTool(t)
	stdout, _ := execReadWordStructureTool(t, tool, aitool.InvokeParams{
		"input":       testFile,
		"tables_only": true,
	})

	output := stdout
	assert.Assert(t, strings.Contains(output, "TABLES"), "should contain TABLES")
	assert.Assert(t, strings.Contains(output, "TransactionID"), "should contain table header")
	assert.Assert(t, !strings.Contains(output, "## TEXT CONTENT"), "tables_only should NOT contain TEXT CONTENT section header")
	t.Logf("stdout:\n%s", stdout)
}

func TestReadWordStructure_OutputToFile(t *testing.T) {
	testFile := createTestDocxFile(t)
	outputFile := filepath.Join(t.TempDir(), "word_output.txt")

	tool := getReadWordStructureTool(t)
	stdout, _ := execReadWordStructureTool(t, tool, aitool.InvokeParams{
		"input":  testFile,
		"output": outputFile,
	})

	assert.Assert(t, strings.Contains(stdout, "saved to"), "should confirm file saved")

	content, err := os.ReadFile(outputFile)
	assert.NilError(t, err, "output file should exist")
	assert.Assert(t, strings.Contains(string(content), "Word Document"), "file should contain document info")
	assert.Assert(t, strings.Contains(string(content), "Investigation Report Title"), "file should contain title text")
	t.Logf("output file content:\n%s", string(content))
}

func TestReadWordStructure_NonExistentFile(t *testing.T) {
	tool := getReadWordStructureTool(t)
	stdout, stderr := execReadWordStructureTool(t, tool, aitool.InvokeParams{
		"input": "/nonexistent/path/doc.docx",
	})

	output := stdout + stderr
	assert.Assert(t, strings.Contains(output, "does not exist"), "should report file not found")
}

func TestReadWordStructure_UnsupportedFormat(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "test.doc")
	os.WriteFile(tmpFile, []byte("fake doc"), 0644)

	tool := getReadWordStructureTool(t)
	stdout, stderr := execReadWordStructureTool(t, tool, aitool.InvokeParams{
		"input": tmpFile,
	})

	output := stdout + stderr
	assert.Assert(t, strings.Contains(output, "Unsupported") || strings.Contains(output, ".doc"),
		"should report unsupported format for .doc")
}

func TestReadWordStructure_AttachmentsSection(t *testing.T) {
	testFile := createTestDocxFile(t)
	outputFile := filepath.Join(t.TempDir(), "attach_check.txt")

	tool := getReadWordStructureTool(t)
	execReadWordStructureTool(t, tool, aitool.InvokeParams{
		"input":       testFile,
		"output":      outputFile,
		"attachments": true,
	})

	content, err := os.ReadFile(outputFile)
	assert.NilError(t, err, "output file should exist")
	output := string(content)
	t.Logf("output file content:\n%s", output)
	assert.Assert(t, strings.Contains(output, "TEXT CONTENT"), "should contain text section")
	assert.Assert(t, strings.Contains(output, "TABLES"), "should contain tables section")
	assert.Assert(t, strings.Contains(output, "ATTACHMENTS") ||
		strings.Contains(output, "No images, charts, or embedded objects found"),
		"should show attachments section or indicate no attachments")
}
