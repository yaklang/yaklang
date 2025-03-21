package excelparser

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/fileparser/types"
)

// ExcelNodeClassifier 用于存储分类后的节点
type ExcelNodeClassifier struct {
	Tables       []TableContent       // 表格内容
	Texts        []TextContent        // 文本内容
	URLs         []URLContent         // URL内容
	Formulas     []FormulaContent     // 公式内容
	Comments     []CommentContent     // 批注内容
	DataConns    []DataConnContent    // 外部数据连接
	PowerQueries []PowerQueryContent  // Power Query脚本
	VBAs         []VBAContent         // VBA宏
	HiddenSheets []HiddenSheetContent // 隐藏工作表
	NameDefs     []NameDefContent     // 名称管理器定义
	CondRules    []CondRuleContent    // 条件规则
}

// ClassifyNodes 将解析结果分类
func ClassifyNodes(nodes []ExcelNode) *ExcelNodeClassifier {
	classifier := &ExcelNodeClassifier{
		Tables:       make([]TableContent, 0),
		Texts:        make([]TextContent, 0),
		URLs:         make([]URLContent, 0),
		Formulas:     make([]FormulaContent, 0),
		Comments:     make([]CommentContent, 0),
		DataConns:    make([]DataConnContent, 0),
		PowerQueries: make([]PowerQueryContent, 0),
		VBAs:         make([]VBAContent, 0),
		HiddenSheets: make([]HiddenSheetContent, 0),
		NameDefs:     make([]NameDefContent, 0),
		CondRules:    make([]CondRuleContent, 0),
	}

	// 处理所有节点
	for _, node := range nodes {
		switch node.Type {
		case TableNode:
			if content, ok := node.Content.(TableContent); ok {
				classifier.Tables = append(classifier.Tables, content)
			}
		case TextNode:
			if content, ok := node.Content.(TextContent); ok {
				classifier.Texts = append(classifier.Texts, content)
			}
		case URLNode:
			if content, ok := node.Content.(URLContent); ok {
				classifier.URLs = append(classifier.URLs, content)
			}
		case FormulaNode:
			if content, ok := node.Content.(FormulaContent); ok {
				classifier.Formulas = append(classifier.Formulas, content)
			}
		case CommentNode:
			if content, ok := node.Content.(CommentContent); ok {
				classifier.Comments = append(classifier.Comments, content)
			}
		case DataConnNode:
			if content, ok := node.Content.(DataConnContent); ok {
				classifier.DataConns = append(classifier.DataConns, content)
			}
		case PowerQueryNode:
			if content, ok := node.Content.(PowerQueryContent); ok {
				classifier.PowerQueries = append(classifier.PowerQueries, content)
			}
		case VBANode:
			if content, ok := node.Content.(VBAContent); ok {
				classifier.VBAs = append(classifier.VBAs, content)
			}
		case HiddenSheetNode:
			if content, ok := node.Content.(HiddenSheetContent); ok {
				classifier.HiddenSheets = append(classifier.HiddenSheets, content)
			}
		case NameDefNode:
			if content, ok := node.Content.(NameDefContent); ok {
				classifier.NameDefs = append(classifier.NameDefs, content)
			}
		case CondRuleNode:
			if content, ok := node.Content.(CondRuleContent); ok {
				classifier.CondRules = append(classifier.CondRules, content)
			}
		}
	}

	return classifier
}

// DumpToFiles 将分类后的节点转换为文件
func (c *ExcelNodeClassifier) DumpToFiles() map[string][]types.File {
	result := make(map[string][]types.File)

	// 处理表格内容
	if len(c.Tables) > 0 {
		var tableFiles []types.File
		for _, table := range c.Tables {
			markdown := c.convertTableToMarkdown(table)
			tableFiles = append(tableFiles, types.File{
				FileName:   "tables/" + table.SheetName + ".md",
				Type:       string(FileTypeTable),
				BinaryData: []byte(markdown),
				Metadata: map[string]string{
					"sheet_name": table.SheetName,
					"row_count":  fmt.Sprintf("%d", len(table.Rows)+1),
				},
			})
		}
		result[string(FileTypeTable)] = tableFiles
	}

	// // 处理文本内容
	// if len(c.Texts) > 0 {
	// 	var textBuffer strings.Builder
	// 	for _, text := range c.Texts {
	// 		textBuffer.WriteString(fmt.Sprintf("%s (%s): %s\n", text.SheetName, text.Cell, text.Text))
	// 	}

	// 	result[string(FileTypeText)] = []types.File{
	// 		{
	// 			FileName:   "text/text.txt",
	// 			Type:       string(FileTypeText),
	// 			BinaryData: []byte(textBuffer.String()),
	// 			Metadata: map[string]string{
	// 				"count": fmt.Sprintf("%d", len(c.Texts)),
	// 			},
	// 		},
	// 	}
	// }

	// 处理URL内容
	if len(c.URLs) > 0 {
		var urlBuffer strings.Builder
		for _, url := range c.URLs {
			urlBuffer.WriteString(fmt.Sprintf("- [%s (%s)](%s)\n", url.SheetName, url.Cell, url.URL))
		}

		result[string(FileTypeURL)] = []types.File{
			{
				FileName:   "urls/urls.txt",
				Type:       string(FileTypeURL),
				BinaryData: []byte(urlBuffer.String()),
				Metadata: map[string]string{
					"count": fmt.Sprintf("%d", len(c.URLs)),
				},
			},
		}
	}

	// 处理公式内容
	if len(c.Formulas) > 0 {
		var formulaBuffer strings.Builder
		formulaBuffer.WriteString("## 单元格公式\n\n")

		for _, formula := range c.Formulas {
			formulaBuffer.WriteString(fmt.Sprintf("### %s!%s\n", formula.SheetName, formula.Cell))
			formulaBuffer.WriteString(fmt.Sprintf("公式: `%s`\n", formula.Formula))
			formulaBuffer.WriteString(fmt.Sprintf("结果: %s\n\n", formula.Result))
		}

		result[string(FileTypeFormula)] = []types.File{
			{
				FileName:   "formulas/formulas.txt",
				Type:       string(FileTypeFormula),
				BinaryData: []byte(formulaBuffer.String()),
				Metadata: map[string]string{
					"count": fmt.Sprintf("%d", len(c.Formulas)),
				},
			},
		}
	}

	// 处理批注内容
	if len(c.Comments) > 0 {
		var commentBuffer strings.Builder
		commentBuffer.WriteString("## 单元格批注\n\n")

		for _, comment := range c.Comments {
			commentBuffer.WriteString(fmt.Sprintf("### %s!%s\n", comment.SheetName, comment.Cell))
			if comment.Author != "" {
				commentBuffer.WriteString(fmt.Sprintf("作者: %s\n", comment.Author))
			}
			commentBuffer.WriteString(fmt.Sprintf("内容: %s\n\n", comment.Text))
		}

		result[string(FileTypeComment)] = []types.File{
			{
				FileName:   "comments/comments.txt",
				Type:       string(FileTypeComment),
				BinaryData: []byte(commentBuffer.String()),
				Metadata: map[string]string{
					"count": fmt.Sprintf("%d", len(c.Comments)),
				},
			},
		}
	}

	// 处理外部数据连接
	if len(c.DataConns) > 0 {
		var connBuffer strings.Builder
		connBuffer.WriteString("## 外部数据连接\n\n")

		for i, conn := range c.DataConns {
			connBuffer.WriteString(fmt.Sprintf("### 连接 %d: %s\n", i+1, conn.Name))
			if conn.Description != "" {
				connBuffer.WriteString(fmt.Sprintf("描述: %s\n", conn.Description))
			}
			connBuffer.WriteString(fmt.Sprintf("类型: %s\n", conn.Type))
			connBuffer.WriteString(fmt.Sprintf("连接字符串: `%s`\n", conn.ConnectionString))
			if conn.Command != "" {
				connBuffer.WriteString(fmt.Sprintf("命令: ```\n%s\n```\n", conn.Command))
			}
			connBuffer.WriteString("\n")
		}

		result[string(FileTypeDataConn)] = []types.File{
			{
				FileName:   "data_conns/data_conns.txt",
				Type:       string(FileTypeDataConn),
				BinaryData: []byte(connBuffer.String()),
				Metadata: map[string]string{
					"count": fmt.Sprintf("%d", len(c.DataConns)),
				},
			},
		}
	}

	// 处理Power Query脚本
	if len(c.PowerQueries) > 0 {
		var pqFiles []types.File
		for i, pq := range c.PowerQueries {
			var pqBuffer strings.Builder
			pqBuffer.WriteString(fmt.Sprintf("# Power Query: %s\n\n", pq.Name))

			if pq.Source != "" {
				pqBuffer.WriteString(fmt.Sprintf("数据源: %s\n\n", pq.Source))
			}

			pqBuffer.WriteString("```\n")
			pqBuffer.WriteString(pq.Script)
			pqBuffer.WriteString("\n```\n")

			pqFiles = append(pqFiles, types.File{
				FileName:   "power_queries/" + pq.Name + ".txt",
				Type:       string(FileTypePowerQuery),
				BinaryData: []byte(pqBuffer.String()),
				Metadata: map[string]string{
					"name":       pq.Name,
					"index":      fmt.Sprintf("%d", i+1),
					"script_len": fmt.Sprintf("%d", len(pq.Script)),
				},
			})
		}
		result[string(FileTypePowerQuery)] = pqFiles
	}

	// 处理VBA宏
	if len(c.VBAs) > 0 {
		var vbaFiles []types.File
		for _, vba := range c.VBAs {
			vbaFiles = append(vbaFiles, types.File{
				FileName:   "vbas/" + fmt.Sprintf("%s.vba", vba.ModuleName),
				Type:       string(FileTypeVBA),
				BinaryData: []byte(vba.Code),
				Metadata: map[string]string{
					"module": vba.ModuleName,
					"type":   vba.Type,
					"size":   fmt.Sprintf("%d", len(vba.Code)),
					"name":   fmt.Sprintf("%s.vba", vba.ModuleName),
				},
			})
		}
		result[string(FileTypeVBA)] = vbaFiles
	}

	// 处理隐藏工作表
	if len(c.HiddenSheets) > 0 {
		var hsFiles []types.File
		for _, sheet := range c.HiddenSheets {
			var hsBuffer strings.Builder

			hsBuffer.WriteString(fmt.Sprintf("# 隐藏工作表: %s\n", sheet.SheetName))
			hsBuffer.WriteString(fmt.Sprintf("隐藏类型: %s\n\n", sheet.HideType))

			// 生成表格内容
			if len(sheet.Headers) > 0 {
				hsBuffer.WriteString("|")
				for _, header := range sheet.Headers {
					hsBuffer.WriteString(fmt.Sprintf(" %s |", header))
				}
				hsBuffer.WriteString("\n|")

				for range sheet.Headers {
					hsBuffer.WriteString(" --- |")
				}
				hsBuffer.WriteString("\n")

				for _, row := range sheet.Rows {
					hsBuffer.WriteString("|")
					for i := 0; i < len(sheet.Headers); i++ {
						cell := ""
						if i < len(row) {
							cell = row[i]
						}
						// 处理单元格中的换行符和竖线符号
						cell = strings.ReplaceAll(cell, "\n", "<br>")
						cell = strings.ReplaceAll(cell, "|", "\\|")
						hsBuffer.WriteString(fmt.Sprintf(" %s |", cell))
					}
					hsBuffer.WriteString("\n")
				}
			}

			hsFiles = append(hsFiles, types.File{
				FileName:   "hidden_sheets/" + sheet.SheetName + ".md",
				Type:       string(FileTypeHiddenSheet),
				BinaryData: []byte(hsBuffer.String()),
				Metadata: map[string]string{
					"sheet_name": sheet.SheetName,
					"hide_type":  sheet.HideType,
				},
			})
		}
		result[string(FileTypeHiddenSheet)] = hsFiles
	}

	// 处理名称管理器定义
	if len(c.NameDefs) > 0 {
		var ndBuffer strings.Builder
		ndBuffer.WriteString("## 名称管理器定义\n\n")

		for _, nd := range c.NameDefs {
			ndBuffer.WriteString(fmt.Sprintf("### %s\n", nd.Name))
			ndBuffer.WriteString(fmt.Sprintf("引用: `%s`\n", nd.RefersTo))
			if nd.Scope != "" {
				ndBuffer.WriteString(fmt.Sprintf("作用域: %s\n", nd.Scope))
			}
			if nd.Comment != "" {
				ndBuffer.WriteString(fmt.Sprintf("注释: %s\n", nd.Comment))
			}
			ndBuffer.WriteString("\n")
		}

		result[string(FileTypeNameDef)] = []types.File{
			{
				FileName:   "name_defs/name_defs.txt",
				Type:       string(FileTypeNameDef),
				BinaryData: []byte(ndBuffer.String()),
				Metadata: map[string]string{
					"count": fmt.Sprintf("%d", len(c.NameDefs)),
				},
			},
		}
	}

	// 处理条件规则
	if len(c.CondRules) > 0 {
		var crBuffer strings.Builder
		crBuffer.WriteString("## 条件格式规则\n\n")

		for i, rule := range c.CondRules {
			crBuffer.WriteString(fmt.Sprintf("### 规则 %d\n", i+1))
			crBuffer.WriteString(fmt.Sprintf("工作表: %s\n", rule.SheetName))
			crBuffer.WriteString(fmt.Sprintf("应用区域: %s\n", rule.Range))
			crBuffer.WriteString(fmt.Sprintf("规则类型: %s\n", rule.Type))
			if rule.Formula != "" {
				crBuffer.WriteString(fmt.Sprintf("公式: `%s`\n", rule.Formula))
			}
			if rule.FormatStyle != "" {
				crBuffer.WriteString(fmt.Sprintf("格式样式: %s\n", rule.FormatStyle))
			}
			crBuffer.WriteString("\n")
		}

		result[string(FileTypeCondRule)] = []types.File{
			{
				FileName:   "cond_rules/cond_rules.txt",
				Type:       string(FileTypeCondRule),
				BinaryData: []byte(crBuffer.String()),
				Metadata: map[string]string{
					"count": fmt.Sprintf("%d", len(c.CondRules)),
				},
			},
		}
	}

	return result
}

// convertTableToMarkdown 将表格内容转换为 Markdown 格式
func (c *ExcelNodeClassifier) convertTableToMarkdown(table TableContent) string {
	var buf strings.Builder

	// 添加表格标题
	buf.WriteString(fmt.Sprintf("### %s\n\n", table.SheetName))

	// 如果没有数据，返回空字符串
	if len(table.Headers) == 0 {
		return buf.String()
	}

	// 处理表头中的空值
	for i, header := range table.Headers {
		if header == "" {
			table.Headers[i] = fmt.Sprintf("列%d", i+1)
		}
	}

	// 写入表头
	buf.WriteString("|")
	for _, header := range table.Headers {
		buf.WriteString(fmt.Sprintf(" %s |", header))
	}
	buf.WriteString("\n")

	// 写入分隔行
	buf.WriteString("|")
	for range table.Headers {
		buf.WriteString(" --- |")
	}
	buf.WriteString("\n")

	// 写入数据行
	for _, row := range table.Rows {
		buf.WriteString("|")
		// 确保行的长度与表头一致
		for i := 0; i < len(table.Headers); i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			// 处理单元格中的换行符和竖线符号
			cell = strings.ReplaceAll(cell, "\n", "<br>")
			cell = strings.ReplaceAll(cell, "|", "\\|")
			buf.WriteString(fmt.Sprintf(" %s |", cell))
		}
		buf.WriteString("\n")
	}

	buf.WriteString("\n")
	return buf.String()
}

// GetStatistics 获取分类统计信息
func (c *ExcelNodeClassifier) GetStatistics() map[string]int {
	return map[string]int{
		"表格数量":         len(c.Tables),
		"文本数量":         len(c.Texts),
		"URL数量":        len(c.URLs),
		"公式数量":         len(c.Formulas),
		"批注数量":         len(c.Comments),
		"外部数据连接数量":     len(c.DataConns),
		"PowerQuery数量": len(c.PowerQueries),
		"VBA宏数量":       len(c.VBAs),
		"隐藏工作表数量":      len(c.HiddenSheets),
		"名称定义数量":       len(c.NameDefs),
		"条件规则数量":       len(c.CondRules),
	}
}

// PrintSummary 打印文档内容摘要
func (c *ExcelNodeClassifier) PrintSummary() string {
	var summary strings.Builder

	// 打印统计信息
	stats := c.GetStatistics()
	summary.WriteString("Excel文档统计信息:\n")
	for k, v := range stats {
		summary.WriteString(fmt.Sprintf("%s: %d\n", k, v))
	}
	summary.WriteString("\n")

	// 打印工作表信息
	if len(c.Tables) > 0 {
		summary.WriteString("工作表信息:\n")
		for i, table := range c.Tables {
			summary.WriteString(fmt.Sprintf("表 %d: %s (%d行x%d列)\n",
				i+1,
				table.SheetName,
				len(table.Rows)+1, // 包括表头
				len(table.Headers)))
		}
		summary.WriteString("\n")
	}

	// 打印隐藏工作表信息
	if len(c.HiddenSheets) > 0 {
		summary.WriteString("隐藏工作表信息:\n")
		for i, sheet := range c.HiddenSheets {
			summary.WriteString(fmt.Sprintf("表 %d: %s (%s)\n",
				i+1,
				sheet.SheetName,
				sheet.HideType))
		}
		summary.WriteString("\n")
	}

	// 打印外部数据连接信息
	if len(c.DataConns) > 0 {
		summary.WriteString("外部数据连接:\n")
		for i, conn := range c.DataConns {
			summary.WriteString(fmt.Sprintf("连接 %d: %s (%s)\n",
				i+1,
				conn.Name,
				conn.Type))
		}
		summary.WriteString("\n")
	}

	// 打印VBA宏信息
	if len(c.VBAs) > 0 {
		summary.WriteString("VBA宏信息:\n")
		for i, vba := range c.VBAs {
			summary.WriteString(fmt.Sprintf("模块 %d: %s (%s)\n",
				i+1,
				vba.ModuleName,
				vba.Type))
		}
		summary.WriteString("\n")
	}

	return summary.String()
}
