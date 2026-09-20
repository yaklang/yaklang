package bundler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"sort"
	"strings"
)

type Parser struct{}

func NewParser() types.Parser { return &Parser{} }
func (p *Parser) Parse(_ fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	limits := budget.From(ctx).Limits
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), limits.MaxFieldBytes)
	var libs []types.Library
	var deps []types.Dependency
	byName := map[string][]string{}
	direct := map[string]bool{}
	section, source, revision, current := "", "", "", ""
	line := 0
	for scanner.Scan() {
		line++
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		raw := scanner.Text()
		text := strings.TrimSpace(raw)
		if text == "" {
			continue
		}
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		if indent == 0 {
			switch text {
			case "GEM", "GIT", "PATH", "DEPENDENCIES", "PLATFORMS", "BUNDLED WITH", "RUBY VERSION", "CHECKSUMS":
			default:
				return nil, nil, fmt.Errorf("unsupported_syntax: Gemfile.lock section %q", text)
			}
			section = text
			source = ""
			revision = ""
			current = ""
			continue
		}
		if section == "DEPENDENCIES" && indent == 2 {
			fields := strings.Fields(text)
			direct[strings.TrimSuffix(fields[0], "!")] = true
			continue
		}
		if section != "GEM" && section != "GIT" && section != "PATH" {
			continue
		}
		if indent == 2 {
			switch {
			case strings.HasPrefix(text, "remote:"):
				source = strings.TrimSpace(strings.TrimPrefix(text, "remote:"))
			case strings.HasPrefix(text, "revision:"):
				revision = strings.TrimSpace(strings.TrimPrefix(text, "revision:"))
			}
			continue
		}
		if indent == 4 {
			name, version, ok := splitSpec(text)
			if !ok {
				return nil, nil, fmt.Errorf("malformed_input: Gemfile.lock line %d", line)
			}
			platform := ""
			for _, suffix := range []string{"-x86_64", "-x86-", "-arm64", "-aarch64", "-universal", "-java", "-mingw", "-mswin"} {
				if i := strings.Index(version, suffix); i >= 0 {
					platform = version[i+1:]
					version = version[:i]
					break
				}
			}
			origin := source
			if revision != "" {
				origin += "#" + revision
			}
			key, _ := json.Marshal([]string{name, version, origin, platform})
			current = string(key)
			libs = append(libs, types.Library{ID: current, Name: name, Version: version, Source: origin, Variant: platform, Indirect: true, Locations: []types.Location{{StartLine: line, EndLine: line}}})
			byName[name] = append(byName[name], current)
			if len(libs) > limits.MaxComponents {
				return nil, nil, fmt.Errorf("resource_limit: gem records")
			}
			continue
		}
		if indent == 6 {
			if current == "" {
				return nil, nil, fmt.Errorf("malformed_input: orphan gem dependency")
			}
			fields := strings.Fields(text)
			name := fields[0]
			constraint := strings.TrimSpace(strings.TrimPrefix(text, name))
			constraint = strings.TrimSuffix(strings.TrimPrefix(constraint, "("), ")")
			if len(deps) == 0 || deps[len(deps)-1].ID != current {
				deps = append(deps, types.Dependency{ID: current})
			}
			d := &deps[len(deps)-1]
			d.Requirements = append(d.Requirements, types.Requirement{Target: name, Constraint: constraint})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("scan error: %w", err)
	}
	for i := range libs {
		libs[i].Indirect = !direct[libs[i].Name]
	}
	for i := range deps {
		for j := range deps[i].Requirements {
			q := &deps[i].Requirements[j]
			if candidates := byName[q.Target]; len(candidates) == 1 {
				q.Resolved = candidates[0]
			}
		}
	}
	sort.Sort(types.Libraries(libs))
	sort.Sort(types.Dependencies(deps))
	return libs, deps, nil
}
func splitSpec(s string) (string, string, bool) {
	i := strings.Index(s, " (")
	if i < 1 || !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	return s[:i], s[i+2 : len(s)-1], true
}
