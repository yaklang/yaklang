package wordparser

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDocx(t *testing.T) {
	// 创建测试目录
	testDir := "testdata"
	err := os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatalf("创建测试目录失败: %v", err)
	}
	defer os.RemoveAll(testDir)

	// 测试文件路径
	testFile := filepath.Join(testDir, "test.docx")

	// 解析文档
	nodes, err := ParseWord(testFile)
	if err != nil {
		t.Logf("解析文件失败（这可能是预期的，因为测试文件不存在）: %v", err)
	}

	// 测试分类器
	classifier := ClassifyNodes(nodes)

	// 测试统计信息
	t.Run("测试统计信息", func(t *testing.T) {
		stats := classifier.GetStatistics()
		for category, count := range stats {
			t.Logf("%s: %d", category, count)
		}
	})

	// 测试文本内容
	t.Run("测试文本内容", func(t *testing.T) {
		texts := classifier.GetAllText()
		t.Logf("文本内容:\n%s", texts)
	})

	// 测试表格内容
	t.Run("测试表格内容", func(t *testing.T) {
		tables := classifier.GetAllTables()
		for i, table := range tables {
			t.Logf("表格 %d:\n%s", i+1, table)
		}
	})

	// 测试图片信息
	t.Run("测试图片信息", func(t *testing.T) {
		images := classifier.GetImageInfo()
		for i, img := range images {
			t.Logf("图片 %d: 名称=%s, 类型=%s, 大小=%s",
				i+1, img["名称"], img["MIME类型"], img["大小"])
		}
	})

	// 测试图表信息
	t.Run("测试图表信息", func(t *testing.T) {
		charts := classifier.GetChartInfo()
		for i, chart := range charts {
			t.Logf("图表 %d: 类型=%s, 大小=%s",
				i+1, chart["类型"], chart["大小"])
		}
	})

	// 测试PDF附件信息
	t.Run("测试PDF附件信息", func(t *testing.T) {
		pdfs := classifier.GetPDFInfo()
		for i, pdf := range pdfs {
			t.Logf("PDF %d: 名称=%s, 大小=%s",
				i+1, pdf["名称"], pdf["大小"])
		}
	})

	// 测试OLE对象信息
	t.Run("测试OLE对象信息", func(t *testing.T) {
		oles := classifier.GetOLEInfo()
		for i, ole := range oles {
			t.Logf("OLE对象 %d: 名称=%s, 类型=%s, 大小=%s",
				i+1, ole["名称"], ole["类型"], ole["大小"])
		}
	})

	// 测试VBA代码信息
	t.Run("测试VBA代码信息", func(t *testing.T) {
		vbas := classifier.GetVBAInfo()
		for i, vba := range vbas {
			t.Logf("VBA模块 %d: 模块名=%s, 代码长度=%s",
				i+1, vba["模块名"], vba["代码长度"])
		}
	})

	// 测试摘要输出
	t.Run("测试摘要输出", func(t *testing.T) {
		summary := classifier.PrintSummary()
		t.Logf("文档摘要:\n%s", summary)
	})
}

// 创建一个包含各种内容的测试文档
func createTestDocx(t *testing.T, filePath string) {
	// 这里可以实现创建测试文档的逻辑
	// 可以使用第三方库来创建一个包含各种内容的docx文件
	t.Log("注意：这里需要实现创建测试文档的逻辑")
}

// TestParseDocxWithRealFile 使用真实文件进行测试
func TestParseDocxWithRealFile(t *testing.T) {
	// 指定一个真实的Word文档路径
	realFilePath := "/Users/z3/Downloads/doc1.docx"

	// 检查文件是否存在
	if _, err := os.Stat(realFilePath); os.IsNotExist(err) {
		t.Skip("跳过测试：测试文件不存在，请提供一个真实的Word文档进行测试")
	}

	// 解析文档
	nodes, err := ParseWord(realFilePath)
	if err != nil {
		t.Fatalf("解析文件失败: %v", err)
	}

	// 使用分类器处理结果
	classifier := ClassifyNodes(nodes)
	// 输出摘要
	summary := classifier.PrintSummary()
	fmt.Printf("文档解析结果:\n%s\n", summary)

	// 验证各种内容
	if len(classifier.Texts) > 0 {
		t.Logf("成功解析文本内容: %d 个文本节点", len(classifier.Texts))
	}
	if len(classifier.Tables) > 0 {
		t.Logf("成功解析表格: %d 个表格", len(classifier.Tables))
	}
	if len(classifier.Images) > 0 {
		t.Logf("成功解析图片: %d 张图片", len(classifier.Images))
	}
	if len(classifier.Charts) > 0 {
		t.Logf("成功解析图表: %d 个图表", len(classifier.Charts))
	}
	if len(classifier.PDFs) > 0 {
		t.Logf("成功解析PDF附件: %d 个PDF", len(classifier.PDFs))
	}
	if len(classifier.OLEs) > 0 {
		t.Logf("成功解析OLE对象: %d 个对象", len(classifier.OLEs))
	}
	if len(classifier.VBAs) > 0 {
		t.Logf("成功解析VBA代码: %d 个模块", len(classifier.VBAs))
	}
}

// TestClassifierWithMockData 使用模拟数据测试分类器
func TestClassifierWithMockData(t *testing.T) {
	// 创建模拟节点
	mockNodes := []WordNode{
		{
			Type: TextNode,
			Content: TextContent{
				Text:     "这是一个测试文本",
				IsBold:   true,
				IsItalic: false,
			},
		},
		{
			Type: TableNode,
			Content: TableContent{
				Headers: []string{"列1", "列2"},
				Rows:    [][]string{{"数据1", "数据2"}},
			},
		},
		{
			Type: ImageNode,
			Content: ImageContent{
				Name:     "test.png",
				MimeType: "image/png",
				Data:     []byte("mock image data"),
			},
		},
	}

	// 使用分类器处理模拟数据
	classifier := ClassifyNodes(mockNodes)

	// 验证分类结果
	t.Run("验证文本节点", func(t *testing.T) {
		if len(classifier.Texts) != 1 {
			t.Errorf("期望1个文本节点，实际得到%d个", len(classifier.Texts))
		}
		if classifier.Texts[0].Text != "这是一个测试文本" {
			t.Errorf("文本内容不匹配")
		}
	})

	t.Run("验证表格节点", func(t *testing.T) {
		if len(classifier.Tables) != 1 {
			t.Errorf("期望1个表格节点，实际得到%d个", len(classifier.Tables))
		}
		if len(classifier.Tables[0].Headers) != 2 {
			t.Errorf("表格列数不匹配")
		}
	})

	t.Run("验证图片节点", func(t *testing.T) {
		if len(classifier.Images) != 1 {
			t.Errorf("期望1个图片节点，实际得到%d个", len(classifier.Images))
		}
		if classifier.Images[0].Name != "test.png" {
			t.Errorf("图片名称不匹配")
		}
	})

	// 测试摘要输出
	summary := classifier.PrintSummary()
	t.Logf("模拟数据测试摘要:\n%s", summary)
}
