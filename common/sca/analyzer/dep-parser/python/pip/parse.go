package pip

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/pyrequire"
	"strings"
	"unicode"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"

	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const (
	commentMarker string = "#"
	endColon      string = ";"
	hashMarker    string = "--"
	startExtras   string = "["
	endExtras     string = "]"
)

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	data, err := textdecode.ReadContext(ctx, r)
	if err != nil {
		return nil, nil, err
	}
	records, err := pyrequire.Parse(ctx, data)
	if err != nil {
		return nil, nil, err
	}
	var libs []types.Library
	for _, d := range records {
		verification := strings.Join(d.Hashes, ",")
		if err := budget.From(ctx).Result(budget.SizeOfPackage(d.Name, d.Version, d.Constraint)); err != nil {
			return nil, nil, err
		}
		libs = append(libs, types.Library{ID: fmt.Sprintf("requirement:%d", d.StartLine), Name: d.Name, Version: d.Version, Evidence: "declared", DeclaredName: d.Name, DeclaredVersion: d.Constraint, DeclaredCondition: d.Marker, Source: d.URL, Variant: d.Extras, Extras: d.Extras, Verification: verification, Locations: []types.Location{{StartLine: d.StartLine, EndLine: d.EndLine}}})
	}
	return libs, nil, nil
}

func rStripByKey(line string, key string) string {
	if pos := strings.Index(line, key); pos >= 0 {
		line = strings.TrimRightFunc((line)[:pos], unicode.IsSpace)
	}
	return line
}

func removeExtras(line string) string {
	startIndex := strings.Index(line, startExtras)
	endIndex := strings.Index(line, endExtras) + 1
	if startIndex >= 0 && endIndex > startIndex {
		line = line[:startIndex] + line[endIndex:]
	}
	return line
}
