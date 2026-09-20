package pipenv

import (
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/sca/core/jsonrecord"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type lockFile struct {
	Default map[string]dependency `json:"default"`
}
type dependency struct {
	Markers   string `json:"markers"`
	Index     string `json:"index"`
	Version   string `json:"version"`
	StartLine int
	EndLine   int
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var lockFile lockFile
	input, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read packages.lock.json: %w", err)
	}
	nodes, err := jsonrecord.Decode(types.ContextOf(r), input, &lockFile)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode Pipenv.lock: %w", err)
	}
	for k, v := range lockFile.Default {
		v.StartLine, v.EndLine = nodes.Get("default", k).Lines()
		lockFile.Default[k] = v
	}

	var libs []types.Library
	for pkgName, dependency := range lockFile.Default {
		libs = append(libs, types.Library{
			Name:      pkgName,
			Condition: dependency.Markers, Source: dependency.Index,
			Version:   strings.TrimLeft(dependency.Version, "="),
			Locations: []types.Location{{StartLine: dependency.StartLine, EndLine: dependency.EndLine}},
		})
	}
	return libs, nil, nil
}
