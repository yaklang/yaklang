package wordparser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// NodeType 定义节点类型
type NodeType int

const (
	TextNode NodeType = iota
	TableNode
	ImageNode
	ChartNode
	PDFNode
	OLENode
	VBANode
)

// WordNode 定义通用节点结构
type WordNode struct {
	Type     NodeType
	Content  interface{}
	Position int
}

// TextContent 文本节点内容
type TextContent struct {
	Text     string
	IsBold   bool
	IsItalic bool
	IsStrike bool
}

// TableContent 表格节点内容
type TableContent struct {
	Rows    [][]string
	Headers []string
}

// ImageContent 图片节点内容
type ImageContent struct {
	Data     []byte
	MimeType string
	Name     string
}

// ChartContent 图表节点内容
type ChartContent struct {
	Type      string      // 图表类型
	Data      []byte      // 图表数据
	ChartData interface{} // 解析后的图表数据
}

// PDFContent PDF附件内容
type PDFContent struct {
	Data []byte
	Name string
}

// OLEContent OLE对象内容
type OLEContent struct {
	Type string // 如 "PowerPoint.Show"
	Data []byte
	Name string
}

// VBAContent VBA代码内容
type VBAContent struct {
	Code    string
	ModName string
}

// Relationship 定义文档关系
type Relationship struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

// Relationships 定义文档关系集合
type Relationships struct {
	XMLName      xml.Name       `xml:"Relationships"`
	Relationship []Relationship `xml:"Relationship"`
}

// ParseWord 解析Word文档，返回节点数组
func ParseWord(filePath string) ([]WordNode, error) {
	var nodes []WordNode

	// 打开docx文件
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开Word文件: %v", err)
	}
	defer r.Close()

	// 解析关系文件
	rels, err := parseRelationships(r)
	if err != nil {
		return nil, err
	}

	// 解析主文档
	doc, err := parseMainDocument(r)
	if err != nil {
		return nil, err
	}

	// 递归解析文档内容
	for _, part := range doc.Body.Elements {
		partNodes, err := parseElement(part, rels, r)
		if err != nil {
			continue
		}
		// 将新节点添加到开头，保持原始顺序
		nodes = append(nodes, partNodes...)
	}

	// 解析VBA代码
	vbaNodes, err := parseVBAContent(r)
	if err == nil {
		// 将VBA节点添加到开头，保持原始顺序
		nodes = append(nodes, vbaNodes...)
	}

	// 解析PDF附件
	pdfNodes, err := parsePDFAttachments(rels, r)
	if err == nil {
		// 将PDF节点添加到开头，保持原始顺序
		nodes = append(nodes, pdfNodes...)
	}

	return nodes, nil
}

// parseRelationships 解析文档关系
func parseRelationships(r *zip.ReadCloser) (*Relationships, error) {
	var rels Relationships
	relsFile := findFile(r.File, "word/_rels/document.xml.rels")
	if relsFile == nil {
		return nil, errors.New("找不到关系文件")
	}

	rc, err := relsFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	err = xml.Unmarshal(data, &rels)
	if err != nil {
		return nil, err
	}

	return &rels, nil
}

// findFile 在zip文件中查找指定文件
func findFile(files []*zip.File, target string) *zip.File {
	for _, f := range files {
		if ok, _ := path.Match(target, f.Name); ok {
			return f
		}
	}
	return nil
}

// parseMainDocument 解析主文档
func parseMainDocument(r *zip.ReadCloser) (*Document, error) {
	var doc Document
	docFile := findFile(r.File, "word/document.xml")
	if docFile == nil {
		return nil, errors.New("找不到主文档")
	}

	rc, err := docFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	err = xml.Unmarshal(data, &doc)
	if err != nil {
		return nil, err
	}

	return &doc, nil
}

// Document 定义文档结构
type Document struct {
	XMLName xml.Name `xml:"document"`
	Body    struct {
		Elements []Element `xml:",any"`
	} `xml:"body"`
}

// Element 定义文档元素
type Element struct {
	XMLName  xml.Name
	Content  string     `xml:",chardata"`
	Attrs    []xml.Attr `xml:",any,attr"`
	Children []Element  `xml:",any"`
}

// parseElement 递归解析文档元素
func parseElement(e Element, rels *Relationships, r *zip.ReadCloser) ([]WordNode, error) {
	var nodes []WordNode

	// 递归解析子元素
	for _, child := range e.Children {
		childNodes, err := parseElement(child, rels, r)
		if err == nil {
			nodes = append(nodes, childNodes...)
		}
	}

	// 根据元素类型解析
	switch e.XMLName.Local {
	case "p": // 段落
		node, err := parseTextNode(e)
		if err == nil {
			nodes = append(nodes, node)
		}

	case "tbl": // 表格
		node, err := parseTableNode(e)
		if err == nil {
			nodes = append(nodes, node)
		}

	case "drawing": // 图片
		node, err := parseImageNode(e, rels, r)
		if err == nil {
			nodes = append(nodes, node)
		}

	case "chart": // 图表
		node, err := parseChartNode(e, rels, r)
		if err == nil {
			nodes = append(nodes, node)
		}

	case "object": // OLE对象
		node, err := parseOLENode(e, rels, r)
		if err == nil {
			nodes = append(nodes, node)
		}
	}

	return nodes, nil
}

// parseTextNode 解析文本节点
func parseTextNode(e Element) (WordNode, error) {
	content := TextContent{
		Text: "",
	}

	// 递归解析文本样式和内容
	var texts []string
	var processElement func(e Element)
	processElement = func(e Element) {
		switch e.XMLName.Local {
		case "t": // 文本内容
			if text := strings.TrimSpace(e.Content); text != "" {
				texts = append(texts, text)
			}
		case "rPr": // 文本样式
			for _, style := range e.Children {
				switch style.XMLName.Local {
				case "b":
					content.IsBold = true
				case "i":
					content.IsItalic = true
				case "strike":
					content.IsStrike = true
				}
			}
		}

		// 递归处理子元素
		for _, child := range e.Children {
			processElement(child)
		}
	}

	// 只处理当前段落的内容，不处理子段落
	if e.XMLName.Local == "p" {
		// 清除原有内容，避免重复
		content.Text = ""
		texts = []string{}

		processElement(e)
		content.Text = strings.Join(texts, " ")
	} else {
		// 非段落元素，使用原始内容
		if trimmed := strings.TrimSpace(e.Content); trimmed != "" {
			content.Text = trimmed
		}
	}

	// 如果文本为空，则返回错误
	if content.Text == "" {
		return WordNode{}, errors.New("空文本节点")
	}

	return WordNode{
		Type:    TextNode,
		Content: content,
	}, nil
}

// parseTableNode 解析表格节点
func parseTableNode(e Element) (WordNode, error) {
	var content TableContent

	// 递归解析表格内容
	var processRow func(e Element) []string
	processRow = func(e Element) []string {
		var rowData []string
		switch e.XMLName.Local {
		case "tc": // 表格单元格
			var cellTexts []string
			for _, child := range e.Children {
				if child.XMLName.Local == "p" {
					node, err := parseTextNode(child)
					if err == nil {
						text := node.Content.(TextContent).Text
						if text != "" {
							cellTexts = append(cellTexts, text)
						}
					}
				}
			}
			rowData = append(rowData, strings.Join(cellTexts, " "))
		default:
			for _, child := range e.Children {
				rowData = append(rowData, processRow(child)...)
			}
		}
		return rowData
	}

	// 处理表格行
	for _, row := range e.Children {
		if row.XMLName.Local != "tr" {
			continue
		}

		rowData := processRow(row)
		if len(rowData) > 0 {
			if len(content.Rows) == 0 {
				content.Headers = rowData
			} else {
				content.Rows = append(content.Rows, rowData)
			}
		}
	}

	return WordNode{
		Type:    TableNode,
		Content: content,
	}, nil
}

// parseImageNode 解析图片节点
func parseImageNode(e Element, rels *Relationships, r *zip.ReadCloser) (WordNode, error) {
	var content ImageContent

	// 打印当前元素的XML结构
	log.Debugf("开始解析图片节点，元素类型: %s", e.XMLName.Local)

	// 递归查找blip元素及其embed属性
	var findBlipEmbed func(e Element) string
	findBlipEmbed = func(e Element) string {
		log.Debugf("检查元素: %s", e.XMLName.Local)

		// 检查当前元素是否为blip
		if e.XMLName.Local == "blip" {
			log.Debugf("找到blip元素，属性: %+v", e.Attrs)
			for _, attr := range e.Attrs {
				// embed属性可能有命名空间前缀，只需要检查Local部分
				if attr.Name.Local == "embed" {
					log.Debugf("找到图片ID: %s", attr.Value)
					return attr.Value
				}
			}
		}

		// 检查所有子元素
		for _, child := range e.Children {
			// 按照文档结构查找：inline -> graphic -> graphicData -> pic -> blipFill -> blip
			switch child.XMLName.Local {
			case "inline", "graphic", "graphicData", "pic", "blipFill", "blip":
				if id := findBlipEmbed(child); id != "" {
					return id
				}
			}
		}
		return ""
	}

	// 查找图片关系ID
	imageID := findBlipEmbed(e)

	if imageID == "" {
		return WordNode{}, errors.New("未找到图片ID")
	}

	// 查找图片关系
	var imagePath string
	log.Debugf("开始查找图片关系，ID: %s", imageID)
	for _, rel := range rels.Relationship {
		log.Debugf("检查关系: ID=%s, Type=%s, Target=%s", rel.ID, rel.Type, rel.Target)
		if rel.ID == imageID {
			// 检查关系类型是否为图片
			if strings.Contains(rel.Type, "image") {
				imagePath = "word/" + rel.Target
				log.Debugf("找到图片路径: %s", imagePath)
				break
			}
		}
	}

	if imagePath == "" {
		return WordNode{}, errors.New("未找到图片关系")
	}

	// 读取图片数据
	imageFile := findFile(r.File, imagePath)
	if imageFile == nil {
		return WordNode{}, fmt.Errorf("找不到图片文件: %s", imagePath)
	}

	rc, err := imageFile.Open()
	if err != nil {
		return WordNode{}, fmt.Errorf("打开图片文件失败: %v", err)
	}
	defer rc.Close()

	content.Data, err = io.ReadAll(rc)
	if err != nil {
		return WordNode{}, fmt.Errorf("读取图片数据失败: %v", err)
	}

	content.Name = path.Base(imagePath)
	content.MimeType = detectMimeType(content.Data)
	log.Infof("成功解析图片: %s, 类型: %s, 大小: %d bytes", content.Name, content.MimeType, len(content.Data))

	return WordNode{
		Type:    ImageNode,
		Content: content,
	}, nil
}

// parseChartNode 解析图表节点
func parseChartNode(e Element, rels *Relationships, r *zip.ReadCloser) (WordNode, error) {
	var content ChartContent

	// 获取图表ID
	var chartID string
	for _, attr := range e.Attrs {
		if attr.Name.Local == "id" {
			chartID = attr.Value
			break
		}
	}

	// 查找图表关系
	var chartPath string
	for _, rel := range rels.Relationship {
		if rel.ID == chartID {
			chartPath = "word/" + rel.Target
			break
		}
	}

	// 读取图表数据
	chartFile := findFile(r.File, chartPath)
	if chartFile == nil {
		return WordNode{}, errors.New("找不到图表文件")
	}

	rc, err := chartFile.Open()
	if err != nil {
		return WordNode{}, err
	}
	defer rc.Close()

	content.Data, err = io.ReadAll(rc)
	if err != nil {
		return WordNode{}, err
	}

	// TODO: 解析图表数据为结构化数据
	content.Type = "Unknown"
	content.ChartData = nil

	return WordNode{
		Type:    ChartNode,
		Content: content,
	}, nil
}

// parseOLENode 解析OLE对象节点
func parseOLENode(e Element, rels *Relationships, r *zip.ReadCloser) (WordNode, error) {
	var content OLEContent

	// 获取OLE对象ID
	var oleID string
	for _, attr := range e.Attrs {
		if attr.Name.Local == "id" {
			oleID = attr.Value
			break
		}
	}

	// 查找OLE对象关系
	var olePath string
	for _, rel := range rels.Relationship {
		if rel.ID == oleID {
			olePath = "word/" + rel.Target
			break
		}
	}

	// 读取OLE对象数据
	oleFile := findFile(r.File, olePath)
	if oleFile == nil {
		return WordNode{}, errors.New("找不到OLE对象文件")
	}

	rc, err := oleFile.Open()
	if err != nil {
		return WordNode{}, err
	}
	defer rc.Close()

	content.Data, err = io.ReadAll(rc)
	if err != nil {
		return WordNode{}, err
	}

	content.Name = path.Base(olePath)
	content.Type = detectOLEType(content.Data)

	return WordNode{
		Type:    OLENode,
		Content: content,
	}, nil
}

// parseVBAContent 解析VBA代码
func parseVBAContent(r *zip.ReadCloser) ([]WordNode, error) {
	var nodes []WordNode

	vbaFile := findFile(r.File, "word/vbaProject.bin")
	if vbaFile == nil {
		return nil, errors.New("找不到VBA项目文件")
	}

	rc, err := vbaFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	// TODO: 解析VBA二进制格式
	content := VBAContent{
		Code:    string(data), // 这里需要实现真正的VBA解析
		ModName: "Unknown",
	}

	nodes = append(nodes, WordNode{
		Type:    VBANode,
		Content: content,
	})

	return nodes, nil
}

// parsePDFAttachments 解析PDF附件
func parsePDFAttachments(rels *Relationships, r *zip.ReadCloser) ([]WordNode, error) {
	var nodes []WordNode

	for _, rel := range rels.Relationship {
		if strings.HasSuffix(rel.Target, ".pdf") {
			pdfPath := "word/" + rel.Target
			pdfFile := findFile(r.File, pdfPath)
			if pdfFile == nil {
				continue
			}

			rc, err := pdfFile.Open()
			if err != nil {
				continue
			}

			data, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				continue
			}

			content := PDFContent{
				Data: data,
				Name: path.Base(rel.Target),
			}

			nodes = append(nodes, WordNode{
				Type:    PDFNode,
				Content: content,
			})
		}
	}

	return nodes, nil
}

// detectMimeType 检测文件MIME类型
func detectMimeType(data []byte) string {
	// 这里可以实现更复杂的MIME类型检测
	if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G'}) {
		return "image/png"
	}
	if bytes.HasPrefix(data, []byte{0xFF, 0xD8}) {
		return "image/jpeg"
	}
	return "application/octet-stream"
}

// detectOLEType 检测OLE对象类型
func detectOLEType(data []byte) string {
	// 这里需要实现OLE对象类型检测
	// 可以通过分析OLE头部或其他特征来判断
	return "Unknown"
}
