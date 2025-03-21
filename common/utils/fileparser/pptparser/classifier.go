package pptparser

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/fileparser/types"
)

// PPTClassifier 用于分类PPT节点
type PPTClassifier struct {
	Images       []*ImageContent       // 所有图像内容
	Urls         []*URLContent         // 所有URL内容
	Videos       []*VideoContent       // 所有视频内容
	Slides       []*SlideContent       // 所有幻灯片内容
	Tables       []*TableContent       // 所有表格内容
	Audios       []*AudioContent       // 所有音频内容
	Notes        []*NoteContent        // 所有备注内容
	Texts        []*TextContent        // 所有文本内容
	Comments     []*CommentContent     // 所有批注内容
	MasterSlides []*MasterSlideContent // 所有母版内容
	Themes       []*ThemeContent       // 所有主题内容
	Animations   []*AnimationContent   // 所有动画内容
	Transitions  []*TransitionContent  // 所有转场效果内容
	Macros       []*MacroContent       // 所有宏内容
}

// ClassifyNodes 对PPT节点进行分类
func ClassifyNodes(nodes []PPTNode) *PPTClassifier {
	classifier := &PPTClassifier{}

	for _, node := range nodes {
		switch node.Type {
		case ExternalNode:
			content, ok := node.Content.(URLContent)
			if ok {
				classifier.Urls = append(classifier.Urls, &content)
			}
		case ImageNode:
			content, ok := node.Content.(ImageContent)
			if ok {
				classifier.Images = append(classifier.Images, &content)
			}
		case TableNode:
			content, ok := node.Content.(TableContent)
			if ok {
				classifier.Tables = append(classifier.Tables, &content)
			}
		case VideoNode:
			content, ok := node.Content.(VideoContent)
			if ok {
				classifier.Videos = append(classifier.Videos, &content)
			}
		case AudioNode:
			content, ok := node.Content.(AudioContent)
			if ok {
				classifier.Audios = append(classifier.Audios, &content)
			}
		case TextNode:
			content, ok := node.Content.(TextContent)
			if ok {
				classifier.Texts = append(classifier.Texts, &content)
			}
		case CommentNode:
			content, ok := node.Content.(CommentContent)
			if ok {
				classifier.Comments = append(classifier.Comments, &content)
			}
		case SlideNode:
			content, ok := node.Content.(SlideContent)
			if ok {
				classifier.Slides = append(classifier.Slides, &content)
			}
		case NoteNode:
			content, ok := node.Content.(NoteContent)
			if ok {
				classifier.Notes = append(classifier.Notes, &content)
			}
		default:
			log.Warnf("未知的节点类型: %s", node.Type)
		}
	}

	return classifier
}

// DumpToFiles 将分类后的节点转换为文件
func (c *PPTClassifier) DumpToFiles() map[string][]types.File {
	result := make(map[string][]types.File)

	// // 处理幻灯片
	// if len(c.Slides) > 0 {
	// 	var slideFiles []File
	// 	for _, slide := range c.Slides {
	// 		// 创建幻灯片概要
	// 		content := fmt.Sprintf("# 幻灯片 %d: %s\n\n", slide.SlideNumber, slide.Title)
	// 		content += fmt.Sprintf("布局: %s\n\n", slide.Layout)

	// 		// 添加元数据
	// 		if len(slide.Metadata) > 0 {
	// 			content += "## 元数据\n\n"
	// 			for key, value := range slide.Metadata {
	// 				content += fmt.Sprintf("- %s: %s\n", key, value)
	// 			}
	// 			content += "\n"
	// 		}

	// 		slideFiles = append(slideFiles, File{
	// 			Type:       SlideNode,
	// 			BinaryData: []byte(content),
	// 			Metadata: map[string]string{
	// 				"slide_number": strconv.Itoa(slide.SlideNumber),
	// 				"title":        slide.Title,
	// 			},
	// 		})
	// 	}
	// 	result[SlideNode] = slideFiles
	// }

	// 处理文本
	if len(c.Texts) > 0 {
		var textFiles []types.File
		var slideTexts = make(map[int]string)

		// 首先按幻灯片分组收集文本
		for _, text := range c.Texts {
			slideNum := text.SlideNumber

			// 获取当前幻灯片的文本，如果没有则初始化
			content, exists := slideTexts[slideNum]
			if !exists {
				// // 找到对应的幻灯片标题
				// var slideTitle string
				// for _, slide := range c.Slides {
				// 	if slide.SlideNumber == slideNum {
				// 		slideTitle = slide.Title
				// 		break
				// 	}
				// }

				content = ""
			}

			// 添加当前文本
			if text.IsTitle {
				content += fmt.Sprintf("## %s\n\n", text.Text)
			} else {
				// 根据文本级别添加缩进
				indent := strings.Repeat("  ", text.Level)
				if text.Level > 0 {
					content += fmt.Sprintf("%s- %s\n", indent, text.Text)
				} else {
					content += fmt.Sprintf("%s\n\n", text.Text)
				}
			}

			slideTexts[slideNum] = content
		}

		// 然后为每个幻灯片创建一个文本文件
		for slideNum, content := range slideTexts {
			textFiles = append(textFiles, types.File{
				Type:       TextNode,
				BinaryData: []byte(content),
				FileName:   fmt.Sprintf("text/slide_%d_text.txt", slideNum),
				Metadata: map[string]string{
					"slide_number": strconv.Itoa(slideNum),
				},
			})
		}

		result[TextNode] = textFiles
	}

	// 处理表格
	if len(c.Tables) > 0 {
		var tableFiles []types.File
		for _, table := range c.Tables {
			content := ""
			// 表格头部
			content += "| " + strings.Join(table.Headers, " | ") + " |\n"
			content += "| " + strings.Repeat("--- | ", len(table.Headers)) + "\n"

			// 表格内容
			for _, row := range table.Rows {
				content += "| " + strings.Join(row, " | ") + " |\n"
			}

			tableFiles = append(tableFiles, types.File{
				Type:       TableNode,
				BinaryData: []byte(content),
				FileName:   fmt.Sprintf("table/slide_%d_table.md", table.SlideNumber),
				Metadata: map[string]string{
					"slide_number": strconv.Itoa(table.SlideNumber),
					"position":     table.Position,
				},
			})
		}
		result[TableNode] = tableFiles
	}

	// 处理URL
	if len(c.Urls) > 0 {
		var urlFiles []types.File
		var allURLs strings.Builder

		for _, url := range c.Urls {
			allURLs.WriteString(url.URL + "\n")
		}

		urlFiles = append(urlFiles, types.File{
			Type:       ExternalNode,
			BinaryData: []byte(allURLs.String()),
			FileName:   "url/all_urls.txt",
			Metadata: map[string]string{
				"count": strconv.Itoa(len(c.Urls)),
			},
		})

		result[ExternalNode] = urlFiles
	}

	// 处理备注
	if len(c.Notes) > 0 {
		var noteFiles []types.File
		var allNotes strings.Builder

		for _, note := range c.Notes {
			allNotes.WriteString(note.Text + "\n\n")
		}

		noteFiles = append(noteFiles, types.File{
			Type:       CommentNode,
			BinaryData: []byte(allNotes.String()),
			FileName:   "comment/all_notes.txt",
			Metadata: map[string]string{
				"count": strconv.Itoa(len(c.Notes)),
			},
		})

		result[CommentNode] = noteFiles
	}

	// 处理视频
	if len(c.Videos) > 0 {
		var videoDataFiles []types.File // 专门存储视频二进制数据

		for i, video := range c.Videos {
			// 获取文件名
			fileName := filepath.Base(video.Path)
			if fileName == "" || fileName == "." {
				fileName = fmt.Sprintf("video_%d.%s", i+1, video.Format)
			}

			// 创建二进制文件 - 对于视频数据使用专门的类型
			binaryFile := types.File{
				Type:       VideoNode, // 使用专门的视频数据类型
				BinaryData: video.Content,
				FileName:   fmt.Sprintf("video/%s", fileName),
				Metadata: map[string]string{
					"slide_number": strconv.Itoa(video.SlideNumber),
					"format":       video.Format,
					"source_path":  video.Path,
					"index":        strconv.Itoa(i),
				},
			}
			videoDataFiles = append(videoDataFiles, binaryFile)
		}

		// 只有在实际有视频二进制数据时才添加videodata类型
		if len(videoDataFiles) > 0 {
			result[VideoNode] = videoDataFiles
			log.Infof("导出 %d 个视频二进制数据文件", len(videoDataFiles))
		}
	}
	if len(c.Images) > 0 {
		var imageFiles []types.File
		for i, image := range c.Images {
			imageFiles = append(imageFiles, types.File{
				Type:       ImageNode,
				FileName:   fmt.Sprintf("image/slide_%d_image%d.png", image.SlideNumber, i),
				BinaryData: image.Content,
			})
		}
		result[ImageNode] = imageFiles
	}
	if len(c.Comments) > 0 {
		var commentFiles []types.File
		for i, comment := range c.Comments {
			commentFiles = append(commentFiles, types.File{
				Type:       CommentNode,
				BinaryData: []byte(comment.Text),
				FileName:   fmt.Sprintf("comment/slide_%d_comment_%d.txt", comment.SlideNumber, i),
			})
		}
		result[CommentNode] = commentFiles
	}
	if len(c.Notes) > 0 {
		var noteFiles []types.File
		for i, note := range c.Notes {
			noteFiles = append(noteFiles, types.File{
				Type:       NoteNode,
				BinaryData: []byte(note.Text),
				FileName:   fmt.Sprintf("note/slide_%d_note_%d.txt", note.SlideNumber, i),
			})
		}
		result[NoteNode] = noteFiles
	}

	return result
}
