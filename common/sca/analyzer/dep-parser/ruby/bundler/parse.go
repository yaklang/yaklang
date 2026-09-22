package bundler

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"sort"
	"strconv"
	"strings"
)

type Parser struct{}

func NewParser() types.Parser { return &Parser{} }
func (p *Parser) Parse(_ fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	ctx := types.ContextOf(r)
	st := budget.From(ctx)
	limits := st.Limits
	scanner := textdecode.NewLines(ctx, r)
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
			name, _, _ := strings.Cut(text, " ")
			name = strings.TrimSuffix(name, "!")
			if err := st.Insert(name); err != nil {
				return nil, nil, err
			}
			direct[name] = true
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
				if err := st.Working(int64(len(origin) + len(revision) + 1)); err != nil {
					return nil, nil, err
				}
				origin += "#" + revision
			}
			var err error
			current, err = bundlerNativeID(st, name, version, origin, platform)
			if err != nil {
				return nil, nil, err
			}
			if len(libs) >= limits.MaxComponents {
				return nil, nil, fmt.Errorf("resource_limit: gem records")
			}
			libs, err = budget.Grow(st, libs, 1, budget.SizeOfPackage("", "", ""))
			if err != nil {
				return nil, nil, err
			}
			if err = st.Working(32); err != nil {
				return nil, nil, err
			}
			libs = append(libs, types.Library{ID: current, Name: name, Version: version, Source: origin, Variant: platform, Indirect: true, Locations: []types.Location{{StartLine: line, EndLine: line}}})
			if err = st.Insert(name); err != nil {
				return nil, nil, err
			}
			ids, err := budget.Grow(st, byName[name], 1, budget.SizeString)
			if err != nil {
				return nil, nil, err
			}
			byName[name] = append(ids, current)
			if len(libs) > limits.MaxComponents {
				return nil, nil, fmt.Errorf("resource_limit: gem records")
			}
			continue
		}
		if indent == 6 {
			if current == "" {
				return nil, nil, fmt.Errorf("malformed_input: orphan gem dependency")
			}
			name, _, _ := strings.Cut(text, " ")
			constraint := strings.TrimSpace(strings.TrimPrefix(text, name))
			constraint = strings.TrimSuffix(strings.TrimPrefix(constraint, "("), ")")
			if len(deps) == 0 || deps[len(deps)-1].ID != current {
				var err error
				deps, err = budget.Grow(st, deps, 1, 64)
				if err != nil {
					return nil, nil, err
				}
				deps = append(deps, types.Dependency{ID: current})
			}
			d := &deps[len(deps)-1]
			qs, err := budget.Grow(st, d.Requirements, 1, 80)
			if err != nil {
				return nil, nil, err
			}
			d.Requirements = append(qs, types.Requirement{Target: name, Constraint: constraint, Condition: text})
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

// Decimal length framing preserves field boundaries even when a source
// contains delimiter bytes. Prefixes themselves are valid UTF-8 in report IDs.
func bundlerNativeID(st *budget.State, name, version, origin, platform string) (string, error) {
	size, err := budget.SizeAdd(int64(len(name)), int64(len(version)), int64(len(origin)), int64(len(platform)), 4*21)
	if err != nil {
		return "", err
	}
	if size > int64(int(^uint(0)>>1)) {
		return "", fmt.Errorf("resource_limit: bundler identity size")
	}
	charge, err := budget.SizeAdd(size, budget.SizeString, budget.SizeObject)
	if err != nil {
		return "", err
	}
	if err = st.Result(charge); err != nil {
		return "", err
	}
	var b strings.Builder
	b.Grow(int(size))
	var digits [20]byte
	for _, field := range [...]string{name, version, origin, platform} {
		b.Write(strconv.AppendInt(digits[:0], int64(len(field)), 10))
		b.WriteByte(':')
		b.WriteString(field)
	}
	return b.String(), nil
}

func splitSpec(s string) (string, string, bool) {
	i := strings.Index(s, " (")
	if i < 1 || !strings.HasSuffix(s, ")") {
		return "", "", false
	}
	return s[:i], s[i+2 : len(s)-1], true
}
