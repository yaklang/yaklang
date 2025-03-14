package wordparser

import (
	"fmt"
	"strings"
)

// WordNodeClassifier 用于存储分类后的节点
type WordNodeClassifier struct {
	Texts  []TextContent  // 文本内容
	Tables []TableContent // Excel表格
	Images []ImageContent // 图片
	Charts []ChartContent // 图表
	PDFs   []PDFContent   // PDF附件
	OLEs   []OLEContent   // OLE对象（如PPT）
	VBAs   []VBAContent   // 宏代码（VBA）
}

// ClassifyNodes 将解析结果分类
func ClassifyNodes(nodes []WordNode) *WordNodeClassifier {
	classifier := &WordNodeClassifier{
		Texts:  make([]TextContent, 0),
		Tables: make([]TableContent, 0),
		Images: make([]ImageContent, 0),
		Charts: make([]ChartContent, 0),
		PDFs:   make([]PDFContent, 0),
		OLEs:   make([]OLEContent, 0),
		VBAs:   make([]VBAContent, 0),
	}

	// 递归处理所有节点
	var processNode func(node WordNode)
	processNode = func(node WordNode) {
		switch node.Type {
		case TextNode:
			if content, ok := node.Content.(TextContent); ok {
				// 直接添加到末尾，保持原始顺序
				classifier.Texts = append(classifier.Texts, content)
			}
		case TableNode:
			if content, ok := node.Content.(TableContent); ok {
				// 直接添加到末尾，保持原始顺序
				classifier.Tables = append(classifier.Tables, content)
			}
		case ImageNode:
			if content, ok := node.Content.(ImageContent); ok {
				// 直接添加到末尾，保持原始顺序
				classifier.Images = append(classifier.Images, content)
			}
		case ChartNode:
			if content, ok := node.Content.(ChartContent); ok {
				// 直接添加到末尾，保持原始顺序
				classifier.Charts = append(classifier.Charts, content)
			}
		case PDFNode:
			if content, ok := node.Content.(PDFContent); ok {
				// 直接添加到末尾，保持原始顺序
				classifier.PDFs = append(classifier.PDFs, content)
			}
		case OLENode:
			if content, ok := node.Content.(OLEContent); ok {
				// 直接添加到末尾，保持原始顺序
				classifier.OLEs = append(classifier.OLEs, content)
			}
		case VBANode:
			if content, ok := node.Content.(VBAContent); ok {
				// 直接添加到末尾，保持原始顺序
				classifier.VBAs = append(classifier.VBAs, content)
			}
		}
	}

	// 处理所有节点
	for _, node := range nodes {
		processNode(node)
	}

	return classifier
}

// GetStatistics 获取分类统计信息
func (c *WordNodeClassifier) GetStatistics() map[string]int {
	return map[string]int{
		"文本数量":    len(c.Texts),
		"表格数量":    len(c.Tables),
		"图片数量":    len(c.Images),
		"图表数量":    len(c.Charts),
		"PDF附件数量": len(c.PDFs),
		"OLE对象数量": len(c.OLEs),
		"VBA代码数量": len(c.VBAs),
	}
}

// GetAllText 获取所有文本内容
func (c *WordNodeClassifier) GetAllText() string {
	var texts []string
	for _, text := range c.Texts {
		if text.Text != "" {
			texts = append(texts, text.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// GetAllTables 获取所有表格内容的字符串表示
func (c *WordNodeClassifier) GetAllTables() []string {
	var tables []string
	for i, table := range c.Tables {
		var tableStr strings.Builder
		tableStr.WriteString(fmt.Sprintf("表格 %d:\n", i+1))

		// 写入表头
		tableStr.WriteString("表头: ")
		tableStr.WriteString(strings.Join(table.Headers, " | "))
		tableStr.WriteString("\n")

		// 写入数据行
		for rowNum, row := range table.Rows {
			tableStr.WriteString(fmt.Sprintf("行 %d: ", rowNum+1))
			tableStr.WriteString(strings.Join(row, " | "))
			tableStr.WriteString("\n")
		}

		tables = append(tables, tableStr.String())
	}
	return tables
}

// GetImageInfo 获取所有图片信息
func (c *WordNodeClassifier) GetImageInfo() []map[string]string {
	var images []map[string]string
	for _, img := range c.Images {
		images = append(images, map[string]string{
			"名称":     img.Name,
			"MIME类型": img.MimeType,
			"大小":     fmt.Sprintf("%d bytes", len(img.Data)),
		})
	}
	return images
}

// GetChartInfo 获取所有图表信息
func (c *WordNodeClassifier) GetChartInfo() []map[string]string {
	var charts []map[string]string
	for _, chart := range c.Charts {
		charts = append(charts, map[string]string{
			"类型": chart.Type,
			"大小": fmt.Sprintf("%d bytes", len(chart.Data)),
		})
	}
	return charts
}

// GetPDFInfo 获取所有PDF附件信息
func (c *WordNodeClassifier) GetPDFInfo() []map[string]string {
	var pdfs []map[string]string
	for _, pdf := range c.PDFs {
		pdfs = append(pdfs, map[string]string{
			"名称": pdf.Name,
			"大小": fmt.Sprintf("%d bytes", len(pdf.Data)),
		})
	}
	return pdfs
}

// GetOLEInfo 获取所有OLE对象信息
func (c *WordNodeClassifier) GetOLEInfo() []map[string]string {
	var oles []map[string]string
	for _, ole := range c.OLEs {
		oles = append(oles, map[string]string{
			"名称": ole.Name,
			"类型": ole.Type,
			"大小": fmt.Sprintf("%d bytes", len(ole.Data)),
		})
	}
	return oles
}

// GetVBAInfo 获取所有VBA代码信息
func (c *WordNodeClassifier) GetVBAInfo() []map[string]string {
	var vbas []map[string]string
	for _, vba := range c.VBAs {
		vbas = append(vbas, map[string]string{
			"模块名":  vba.ModName,
			"代码长度": fmt.Sprintf("%d bytes", len(vba.Code)),
		})
	}
	return vbas
}

// PrintSummary 打印文档内容摘要
func (c *WordNodeClassifier) PrintSummary() string {
	var summary strings.Builder

	// 打印统计信息
	stats := c.GetStatistics()
	summary.WriteString("文档统计信息:\n")
	for k, v := range stats {
		summary.WriteString(fmt.Sprintf("%s: %d\n", k, v))
	}
	summary.WriteString("\n")

	// 打印文本预览
	if len(c.Texts) > 0 {
		summary.WriteString("文本内容预览:\n")
		text := c.GetAllText()
		if len(text) > 200 {
			text = text[:200] + "..."
		}
		summary.WriteString(text)
		summary.WriteString("\n\n")
	}

	// 打印表格信息
	if len(c.Tables) > 0 {
		summary.WriteString(fmt.Sprintf("表格数量: %d\n", len(c.Tables)))
		for i, table := range c.Tables {
			summary.WriteString(fmt.Sprintf("表格 %d: %d 行 x %d 列\n",
				i+1, len(table.Rows)+1, len(table.Headers)))
		}
		summary.WriteString("\n")
	}

	// 打印图片信息
	if len(c.Images) > 0 {
		summary.WriteString("图片信息:\n")
		for i, img := range c.Images {
			summary.WriteString(fmt.Sprintf("图片 %d: %s (%s)\n",
				i+1, img.Name, img.MimeType))
		}
		summary.WriteString("\n")
	}

	// 打印其他资源信息
	if len(c.Charts) > 0 {
		summary.WriteString(fmt.Sprintf("图表数量: %d\n", len(c.Charts)))
	}
	if len(c.PDFs) > 0 {
		summary.WriteString(fmt.Sprintf("PDF附件数量: %d\n", len(c.PDFs)))
	}
	if len(c.OLEs) > 0 {
		summary.WriteString(fmt.Sprintf("OLE对象数量: %d\n", len(c.OLEs)))
	}
	if len(c.VBAs) > 0 {
		summary.WriteString(fmt.Sprintf("VBA代码模块数量: %d\n", len(c.VBAs)))
	}

	return summary.String()
}
