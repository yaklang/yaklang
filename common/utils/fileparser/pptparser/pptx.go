package pptparser

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/yaklang/yaklang/common/log"
)

// PPTXParser is the parser for PPTX files using OPC (Open Packaging Convention)
type PPTXParser struct {
	content          []byte
	package_         *OpcPackage
	nodes            []PPTNode
	presentationPart *Part
}

// Parse parses the PPTX file
func (p *PPTXParser) Parse() ([]PPTNode, error) {
	var err error

	// Open the package using OPC
	p.package_, err = OpenOpcPackage(bytes.NewReader(p.content))
	if err != nil {
		return nil, fmt.Errorf("failed to open PPTX package: %v", err)
	}

	// Get the main document part (presentation part)
	presentationPart, err := p.package_.MainDocumentPart()
	if err != nil {
		return nil, fmt.Errorf("failed to get presentation part: %v", err)
	}
	err = p.parsePresentationAllNodes(presentationPart)
	if err != nil {
		return nil, fmt.Errorf("failed to parse presentation all nodes: %v", err)
	}
	return p.nodes, nil
}
func (p *PPTXParser) parsePresentationAllNodes(presentationPart *Part) error {
	sldLists, err := parseSldIdLst(presentationPart.Blob())
	if err != nil {
		return fmt.Errorf("failed to parse sldIdLst: %v", err)
	}
	partNameMap := map[string]struct{}{}
	for i, sldList := range sldLists {
		slidePart, err := presentationPart.RelatedPart(sldList.RID)
		if err != nil {
			return fmt.Errorf("failed to get slide part: %v", err)
		}
		p.nodes = append(p.nodes, PPTNode{
			Type: "slide",
			Content: SlideContent{
				SlideNumber: i,
				Title:       "Slide " + strconv.Itoa(i),
			},
		})
		root := etree.NewDocument()
		if err := root.ReadFromBytes(slidePart.Blob()); err != nil {
			return fmt.Errorf("failed to read slide part: %v", err)
		}
		nodes := processTable(root.Root())
		p.nodes = append(p.nodes, nodes...)
		txBodys := root.FindElements("//p:txBody")
		var getAllText func(node *etree.Element) string
		getAllText = func(node *etree.Element) string {
			text := ""
			for _, child := range node.Child {
				if data, ok := child.(*etree.CharData); ok {
					text += data.Data
				} else {
					text += getAllText(child.(*etree.Element))
				}
			}
			return text
		}
		for _, txBody := range txBodys {
			p.nodes = append(p.nodes, PPTNode{
				Type: TextNode,
				Content: TextContent{
					SlideNumber: i,
					Text:        getAllText(txBody),
				},
			})
		}
		rels := slidePart.Rels()
		for _, rel := range rels.Values() {
			if _, ok := partNameMap[rel.TargetRef()]; ok {
				continue
			}
			partNameMap[rel.TargetRef()] = struct{}{}
			if rel.targetMode == "External" {
				p.nodes = append(p.nodes, PPTNode{
					Type: ExternalNode,
					Content: URLContent{
						SlideNumber: i,
						URL:         rel.TargetRef(),
						DisplayText: rel.TargetRef(),
					},
				})
			} else {
				part, err := slidePart.RelatedPart(rel.RID())
				if err != nil {
					return fmt.Errorf("failed to get related part: %v", err)
				}
				switch part.ContentType() {
				case "image/png":
					p.nodes = append(p.nodes, PPTNode{
						Type: part.ContentType(),
						Content: ImageContent{
							SlideNumber: i,
							Content:     part.Blob(),
						},
					})
				case "video/mp4":
					p.nodes = append(p.nodes, PPTNode{
						Type: part.ContentType(),
						Content: VideoContent{
							SlideNumber: i,
							Content:     part.Blob(),
							Path:        rel.TargetRef(),
						},
					})
				case "audio/mpeg":
					p.nodes = append(p.nodes, PPTNode{
						Type: part.ContentType(),
						Content: AudioContent{
							SlideNumber: i,
							Content:     part.Blob(),
							Path:        rel.TargetRef(),
						},
					})
				case "application/vnd.openxmlformats-officedocument.presentationml.notesSlide+xml":
					notes, err := parseNotesXml(part.Blob())
					if err != nil {
						log.Warnf("failed to parse notes XML: %v", err)
						continue
					}
					noteText := strings.Join(notes, "\n")
					p.nodes = append(p.nodes, PPTNode{
						Type: NoteNode,
						Content: NoteContent{
							SlideNumber: i,
							Text:        noteText,
						},
					})
				case "application/vnd.ms-powerpoint.comments+xml":
					comments, err := parseCommentsXml(part.Blob())
					if err != nil {
						log.Warnf("failed to parse comments XML: %v", err)
						continue
					}
					commentText := strings.Join(comments, "\n")
					p.nodes = append(p.nodes, PPTNode{
						Type: CommentNode,
						Content: CommentContent{
							SlideNumber: i,
							Text:        commentText,
						},
					})
				}
			}
		}
	}
	return nil
}

// parsePresentationPart parses the presentation part
func (p *PPTXParser) parsePresentationPart(presentationPart *Part) error {
	p.presentationPart = presentationPart
	// Parse the presentation XML
	sldIdLst, err := parseSldIdLst(presentationPart.Blob())
	if err != nil {
		return err
	}

	// Create a file reader for additional content
	reader := bytes.NewReader(p.content)

	// Process each slide
	for _, sldId := range sldIdLst {
		// Get the slide part by relationship ID
		slidePart, err := presentationPart.RelatedPart(sldId.RID)
		if err != nil {
			log.Warnf("Failed to get slide part for rId %s: %v", sldId.RID, err)
			continue
		}

		// Parse the slide
		slideNumber, err := getSlideNumberFromId(sldId.ID)
		if err != nil {
			log.Warnf("Failed to parse slide number from id %s: %v", sldId.ID, err)
			slideNumber = len(p.nodes) + 1 // Fallback to node count + 1
		}

		// Parse the slide using the existing code
		if err := p.parseSlide(slidePart, slideNumber, reader); err != nil {
			log.Warnf("Failed to parse slide %d: %v", slideNumber, err)
			continue
		}
	}

	return nil
}

func parseSldIdLst(data []byte) ([]*SldId, error) {
	doc := etree.NewDocument()
	err := doc.ReadFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read sldIdLst XML: %v", err)
	}

	root := doc.Root()
	if root == nil {
		return nil, fmt.Errorf("failed to get root element from sldIdLst XML")
	}

	sldIdLst := root.FindElements("//p:sldIdLst/p:sldId")
	if len(sldIdLst) == 0 {
		return nil, fmt.Errorf("failed to find sldIdLst element in sldIdLst XML")
	}

	var sldIdList []*SldId
	for _, sldId := range sldIdLst {
		id := sldId.SelectAttr("id")
		if id == nil {
			return nil, fmt.Errorf("failed to find id element in sldId element")
		}

		rid := sldId.SelectAttr("r:id")
		if rid == nil {
			return nil, fmt.Errorf("failed to find rid element in sldId element")
		}
		sldIdList = append(sldIdList, &SldId{
			ID:  id.Value,
			RID: rid.Value,
		})
	}
	return sldIdList, nil
}

// getSlideNumberFromId extracts the slide number from the slide ID
func getSlideNumberFromId(id string) (int, error) {
	// Slide IDs are typically in the format "256", "257", etc.
	// We convert this to 1-based slide numbers
	slideID := 0
	_, err := fmt.Sscanf(id, "%d", &slideID)
	if err != nil {
		return 0, err
	}

	// Convert to 1-based index (typically slide IDs start at 256)
	if slideID >= 256 {
		return slideID - 255, nil
	}
	return slideID, nil
}

// parseSlide parses a slide part
func (p *PPTXParser) parseSlide(slidePart *Part, slideNumber int, reader *bytes.Reader) error {

	// Add slide node

	// Parse the slide XML
	// slideContent, err := parseSlideXml(slidePart.Blob())
	// if err != nil {
	// 	return err
	// }
	// slideContent.ElementMap = map[string][][]byte{}
	// for _, rid := range slideContent.Rids {
	// 	part, err := slidePart.RelatedPart(rid)
	// 	if err != nil {
	// 		log.Warnf("Failed to get related part for rid %s: %v", rid, err)
	// 		continue
	// 	}
	// 	blob := part.Blob()
	// 	contentType := part.ContentType()
	// 	slideContent.ElementMap[contentType] = append(slideContent.ElementMap[contentType], blob)
	// }
	// p.nodes = append(p.nodes, PPTNode{
	// 	Type:    SlideNode,
	// 	Content: slideContent,
	// })
	return nil
}

// Parse slide XML - using the slideXML type from ppt.go
func parseSlideXml(data []byte) (*SlideContent, error) {
	var slide SlideContent
	// doc := etree.NewDocument()
	// err := doc.ReadFromBytes(data)
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to read slide XML: %v", err)
	// }

	// root := doc.Root()
	// if root == nil {
	// 	return nil, fmt.Errorf("failed to get root element from slide XML")
	// }

	// txBodys := root.FindElements("//p:txBody")
	// blips := root.FindElements("//a:blip")
	// videoFiles := root.FindElements("//a:videoFile")
	// videoRids := []string{}
	// for _, videoFile := range videoFiles {
	// 	videoRids = append(videoRids, videoFile.SelectAttr("r:link").Value)
	// }
	// slide.Rids = append(slide.Rids, videoRids...)
	// var getAllText func(node *etree.Element) string
	// getAllText = func(node *etree.Element) string {
	// 	text := ""
	// 	for _, child := range node.Child {
	// 		if data, ok := child.(*etree.CharData); ok {
	// 			text += data.Data
	// 		} else {
	// 			text += getAllText(child.(*etree.Element))
	// 		}
	// 	}
	// 	return text
	// }
	// for _, txBody := range txBodys {
	// 	slide.Elements = append(slide.Elements, getAllText(txBody))
	// }
	// for _, blip := range blips {
	// 	var rid string
	// 	for _, attr := range blip.Attr {
	// 		if attr.Key == "embed" {
	// 			rid = attr.Value
	// 			break
	// 		}
	// 	}
	// 	slide.Rids = append(slide.Rids, rid)
	// }
	// nodes := processTable(root)
	// slide.Nodes = append(slide.Nodes, nodes...)
	return &slide, nil
}

func processTable(rootNode *etree.Element) []PPTNode {
	tableNodes := rootNode.FindElements("//a:tbl")
	if len(tableNodes) == 0 {
		return []PPTNode{}
	}
	nodes := []PPTNode{}
	// 处理每个表格
	for _, tableNode := range tableNodes {
		// 查找表格ID和名称
		var tableID, tableName string

		// 查找父节点中的id属性
		parent := tableNode.Parent()
		if parent != nil && parent.Tag == "p:graphicFrame" {
			nvGraphicFramePr := parent.FindElement("p:nvGraphicFramePr")
			if nvGraphicFramePr != nil {
				cNvPr := nvGraphicFramePr.FindElement("p:cNvPr")
				if cNvPr != nil {
					tableID = cNvPr.SelectAttrValue("id", "")
					tableName = cNvPr.SelectAttrValue("name", "")
				}
			}
		}

		// 提取表头
		headers := []string{}
		firstRow := tableNode.FindElement("a:tr")
		if firstRow != nil {
			// 表格第一行作为表头
			for _, cell := range firstRow.FindElements("a:tc") {
				textBody := cell.FindElement("a:txBody")
				if textBody != nil {
					var cellText string
					// 提取段落中的文本
					for _, p := range textBody.FindElements("a:p") {
						for _, r := range p.FindElements("a:r") {
							t := r.FindElement("a:t")
							if t != nil {
								cellText += t.Text()
							}
						}
					}
					headers = append(headers, cellText)
				} else {
					headers = append(headers, "")
				}
			}
		}

		// 提取数据行
		rows := [][]string{}
		// 从第二行开始提取数据行
		tableRows := tableNode.FindElements("a:tr")
		for i := 1; i < len(tableRows); i++ { // 跳过第一行（表头）
			row := tableRows[i]
			rowData := []string{}

			for _, cell := range row.FindElements("a:tc") {
				textBody := cell.FindElement("a:txBody")
				if textBody != nil {
					var cellText string
					// 提取段落中的文本
					for _, p := range textBody.FindElements("a:p") {
						for _, r := range p.FindElements("a:r") {
							t := r.FindElement("a:t")
							if t != nil {
								cellText += t.Text()
							}
						}
					}
					rowData = append(rowData, cellText)
				} else {
					rowData = append(rowData, "")
				}
			}

			rows = append(rows, rowData)
		}

		// 添加表格节点
		nodes = append(nodes, PPTNode{
			Type: TableNode,
			Content: TableContent{
				Headers:  headers,
				Rows:     rows,
				Position: fmt.Sprintf("表格ID_%s", tableID),
				Style:    "基本样式", // 默认样式
				Metadata: map[string]string{
					"name": tableName,
					"id":   tableID,
				},
			},
		})
	}
	return nodes
}

// ParsePPTX parses a PPTX file and returns the nodes
func ParsePPTX(content []byte) ([]PPTNode, error) {
	// Check if file is a PPTX file
	parser := &PPTXParser{
		content: content,
	}
	// Parse the file
	return parser.Parse()
}

// HandlePPTFile is a wrapper function to ensure compatibility with the existing code
// Renamed from ParsePPTFile to avoid conflict
// func HandlePPTFile(filePath string) ([]PPTNode, error) {
// 	// Check file extension
// 	ext := filepath.Ext(filePath)
// 	if strings.EqualFold(ext, ".pptx") {
// 		// Use the new PPTX parser for .pptx files
// 		return ParsePPTXFile(filePath)
// 	}

// 	// For .ppt files, continue using the existing code
// 	// The existing implementation should be in ppt.go
// 	log.Infof("File is a .ppt file, using legacy parser: %s", filePath)

// 	// Call the existing implementation
// 	// This should be implemented already in ppt.go
// 	reader, err := utils.NewFileReader(filePath)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to create file reader: %v", err)
// 	}

// 	var nodes []PPTNode
// 	// // Call the existing implementation that processes slides
// 	// // Note: processSlideDirectly doesn't return an error
// 	// processSlideDirectly(reader, "", 1, &nodes)

// 	return nodes, nil
// }

// Helper functions for PPTX parsing
// These are just minimal implementations to reference existing functions

// Helper for parsing notes XML
func parseNotesXml(data []byte) ([]string, error) {
	doc := etree.NewDocument()
	err := doc.ReadFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read notes XML: %v", err)
	}
	root := doc.Root()
	if root == nil {
		return nil, fmt.Errorf("failed to get root element from notes XML")
	}
	res := []string{}
	notes := root.FindElements("//a:t")
	for _, note := range notes {
		res = append(res, note.Text())
	}
	return res, nil
}

// Helper for parsing comments XML
func parseCommentsXml(data []byte) ([]string, error) {
	doc := etree.NewDocument()
	err := doc.ReadFromBytes(data)
	if err != nil {
		return nil, fmt.Errorf("failed to read comments XML: %v", err)
	}
	root := doc.Root()
	if root == nil {
		return nil, fmt.Errorf("failed to get root element from comments XML")
	}
	res := []string{}
	comments := root.FindElements("//a:t")
	for _, comment := range comments {
		res = append(res, comment.Text())
	}
	return res, nil
}
