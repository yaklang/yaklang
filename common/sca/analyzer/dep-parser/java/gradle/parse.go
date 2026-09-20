package gradle

import (
	"bufio"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var libs []types.Library
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), budget.From(types.ContextOf(r)).Limits.MaxFieldBytes)
	var lineNum int
	for scanner.Scan() {
		lineNum++
		if err := types.ContextOf(r).Err(); err != nil {
			return nil, nil, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") { // skip comments
			continue
		}

		if strings.HasPrefix(line, "empty=") {
			continue
		}
		coordinate, scope, _ := strings.Cut(line, "=")
		dep := strings.Split(coordinate, ":")
		if len(dep) != 3 || dep[0] == "" || dep[1] == "" || dep[2] == "" || strings.ContainsAny(coordinate, " \t") {
			return nil, nil, fmt.Errorf("malformed_input: Gradle line %d", lineNum)
		}
		name := strings.Join(dep[:2], ":")
		version := dep[2]
		libs = append(libs, types.Library{
			ID:      fmt.Sprintf("%s:%s", name, version),
			Name:    name,
			Scope:   scope,
			Version: version,
			Locations: []types.Location{
				{
					StartLine: lineNum,
					EndLine:   lineNum,
				},
			},
		})

	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("resource_limit: Gradle scanner: %w", err)
	}
	return utils.UniqueLibraries(libs), nil, nil
}
