package metadata

import (
	"embed"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/static_analyzer"

	_ "github.com/yaklang/yaklang/common/yak"
)

func GetYakScript(fs embed.FS, name string) (string, error) {
	content, err := fs.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

type YakToolMetadata struct {
	Name        string
	Description string
	Keywords    []string
}

func ParseYakScriptMetadata(name string, code string) (*YakToolMetadata, error) {
	prog, err := static_analyzer.SSAParse(code, "yak")
	if err != nil {
		return nil, fmt.Errorf("static_analyzer.SSAParse(string(content), \"yak\") error: %v", err)
	}

	var desc []string
	prog.Ref("__DESC__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		desc = append(desc, value.String())
	})

	var keywords []string
	prog.Ref("__KEYWORDS__").ForEach(func(value *ssaapi.Value) {
		if !value.IsConstInst() {
			return
		}
		keywords = append(keywords, strings.Split(value.String(), ",")...)
	})
	return &YakToolMetadata{
		Name:        name,
		Description: strings.Join(desc, "; "),
		Keywords:    keywords,
	}, nil
}
