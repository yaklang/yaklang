package metadata

import (
	"embed"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"
)

func GetYakScript(fs embed.FS, name string) (string, error) {
	content, err := fs.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

type YakScriptMetadata struct {
	Name        string
	VerboseName string
	Description string
	Keywords    []string
}

func ParseYakScriptMetadataProg(name string, prog *ssaapi.Program) (*YakScriptMetadata, error) {
	var desc []string
	prog.Ref("__DESC__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		data, err := strconv.Unquote(value.String())
		if err != nil {
			data = value.String()
		}
		desc = append(desc, data)
	})

	var keywords []string
	prog.Ref("__KEYWORDS__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		data, err := strconv.Unquote(value.String())
		if err != nil {
			data = value.String()
		}
		keywords = append(keywords, strings.Split(data, ",")...)
	})
	// __VERBOSE_NAME__
	var verboseName string
	prog.Ref("__VERBOSE_NAME__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		data, err := strconv.Unquote(value.String())
		if err != nil {
			data = value.String()
		}
		if verboseName == "" {
			verboseName = data
		}
	})
	return &YakScriptMetadata{
		Name:        name,
		VerboseName: verboseName,
		Description: strings.Join(desc, "; "),
		Keywords:    keywords,
	}, nil
}

func ParseYakScriptMetadata(name string, code string) (*YakScriptMetadata, error) {
	prog, err := static_analyzer.SSAParse(code, "yak")
	if err != nil {
		return nil, fmt.Errorf("static_analyzer.SSAParse(string(content), \"yak\") error: %v", err)
	}
	return ParseYakScriptMetadataProg(name, prog)
}

// GenerateScriptWithMetadata 生成带有描述和关键词的脚本内容
func GenerateScriptWithMetadata(content string, description string, keywords []string) string {
	prog, err := static_analyzer.SSAParse(content, "yak")
	if err != nil {
		log.Errorf("Failed to parse metadata: %v", err)
		return content
	}

	contentLines := strings.Split(content, "\n")
	descRanges := make([]struct{ typ, start, end int }, 0)
	keywordsRanges := make([]struct{ typ, start, end int }, 0)

	// Find __DESC__ variables and their ranges
	prog.Ref("__DESC__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		descRange := value.GetRange()
		if descRange != nil {
			start := descRange.GetStart().GetLine()
			end := descRange.GetEnd().GetLine()
			descRanges = append(descRanges, struct{ typ, start, end int }{typ: 0, start: start, end: end})
		}
	})

	// Find __KEYWORDS__ variables and their ranges
	prog.Ref("__KEYWORDS__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		keywordsRange := value.GetRange()
		if keywordsRange != nil {
			start := keywordsRange.GetStart().GetLine()
			end := keywordsRange.GetEnd().GetLine()
			keywordsRanges = append(keywordsRanges, struct{ typ, start, end int }{typ: 1, start: start, end: end})
		}
	})

	allRange := append(descRanges, keywordsRanges...)
	// Sort ranges in reverse order to avoid index shifts when modifying the content
	sort.Slice(allRange, func(i, j int) bool {
		return allRange[i].start > allRange[j].start
	})

	// Replace or remove all __DESC__ variables
	for _, r := range allRange {
		// 确保索引在有效范围内
		if r.start <= 0 || r.end >= len(contentLines) {
			log.Warnf("Invalid range: start=%d, end=%d, content length=%d", r.start, r.end, len(contentLines))
			continue
		}

		switch r.typ {
		case 0:
			if r.start-1 >= 0 && r.end+1 <= len(contentLines) {
				contentLines = append(contentLines[:r.start-1], contentLines[r.end:]...)
			}
		case 1:
			if r.start-1 >= 0 && r.end+1 <= len(contentLines) {
				contentLines = append(contentLines[:r.start-1], contentLines[r.end:]...)
			}
		}
	}

	// Generate new declarations
	newDesc := ""
	if strings.Contains(description, "\n") {
		// Use heredoc format for multiline descriptions
		newDesc = fmt.Sprintf("__DESC__ = <<<EOF\n%s\nEOF\n\n", description)
	} else {
		newDesc = fmt.Sprintf("__DESC__ = %q\n\n", description)
	}
	newKeywords := fmt.Sprintf("__KEYWORDS__ = %q\n\n", strings.Join(keywords, ","))

	newContent := strings.TrimSpace(strings.Join(contentLines, "\n"))
	// Add new declarations at the beginning of the file
	newContent = newDesc + newKeywords + newContent
	return newContent
}
