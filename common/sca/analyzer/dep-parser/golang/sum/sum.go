package sum

import (
	"bufio"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/golang/mod"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

// Parse parses a go.sum file
func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	var libs []types.Library
	uniqueLibs := make(map[string]string)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		s := strings.Fields(line)
		if len(s) < 2 {
			continue
		}

		// go.sum records and sorts all non-major versions
		// with the latest version as last entry
		uniqueLibs[s[0]] = strings.TrimSuffix(strings.TrimPrefix(s[1], "v"), "/go.mod")
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, utils.Errorf("scan error: %w", err)
	}

	for k, v := range uniqueLibs {
		libs = append(libs, types.Library{
			ID:      mod.ModuleID(k, v),
			Name:    k,
			Version: v,
		})
	}

	return libs, nil, nil
}
