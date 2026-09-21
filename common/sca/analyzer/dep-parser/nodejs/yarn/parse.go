package yarn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/textdecode"
	"io"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/utils"
	"github.com/yaklang/yaklang/common/sca/internal/digest"
	"github.com/yaklang/yaklang/common/sca/model"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

var (
	yarnPatternRegexp    = regexp.MustCompile(`^\s?\\?"?(?P<package>\S+?)@(?:(?P<protocol>\S+?):)?(?P<version>.+?)\\?"?:?$`)
	yarnVersionRegexp    = regexp.MustCompile(`^"?version:?"?\s+"?(?P<version>[^"]+)"?`)
	yarnDependencyRegexp = regexp.MustCompile(`\s{4,}"?(?P<package>.+?)"?:?\s"?(?:(?P<protocol>\S+?):)?(?P<version>[^"]+)"?`)
)

type LockFile struct {
	Dependencies map[string]Dependency
}

type Library struct {
	Source    string
	Integrity string
	Patterns  []string
	Name      string
	Version   string
	Location  types.Location
}
type Dependency struct {
	Pattern string
	Name    string
}

type LineScanner struct {
	*textdecode.Lines
	lineCount int
}

func NewLineScanner(r io.Reader) *LineScanner {
	return &LineScanner{Lines: textdecode.NewLines(types.ContextOf(r), r)}
}

func (s *LineScanner) Scan() bool {
	scan := s.Lines.Scan()
	if scan {
		s.lineCount++
	}
	return scan
}

func (s *LineScanner) LineNum(prevNum int) int {
	return prevNum + s.lineCount - 1
}

func parsePattern(target string) (packagename, protocol, version string, err error) {
	capture := yarnPatternRegexp.FindStringSubmatch(target)
	if len(capture) < 3 {
		return "", "", "", errors.New("not package format")
	}
	for i, group := range yarnPatternRegexp.SubexpNames() {
		switch group {
		case "package":
			packagename = capture[i]
		case "protocol":
			protocol = capture[i]
		case "version":
			version = capture[i]
		}
	}
	return
}

func parsePackagePatterns(target string) (packagename, protocol string, patterns []string, err error) {
	patternsSplit := strings.Split(target, ", ")
	packagename, protocol, _, err = parsePattern(patternsSplit[0])
	if err != nil {
		return "", "", nil, err
	}
	for _, pattern := range patternsSplit {
		name, proto, version, e := parsePattern(pattern)
		if e != nil || proto != protocol {
			return "", "", nil, fmt.Errorf("malformed_input: inconsistent Yarn descriptor")
		}
		patterns = append(patterns, utils.PackageID(name, version))
	}
	return
}

func getVersion(target string) (version string, err error) {
	capture := yarnVersionRegexp.FindStringSubmatch(target)
	if len(capture) < 2 {
		return "", fmt.Errorf("failed to parse version: '%s", target)
	}
	return capture[len(capture)-1], nil
}

func getDependency(target string) (name, version string, err error) {
	capture := yarnDependencyRegexp.FindStringSubmatch(target)
	if len(capture) < 3 {
		return "", "", errors.New("not dependency")
	}
	if !validProtocol(capture[2]) {
		return "", "", nil
	}
	return capture[1], capture[3], nil
}

func validProtocol(protocol string) bool {
	switch protocol {
	// only scan npm packages
	case "npm", "":
		return true
	}
	return false
}

func ignoreProtocol(protocol string) bool {
	switch protocol {
	case "workspace", "patch", "file", "link", "portal", "github", "git", "git+ssh", "git+http", "git+https", "git+file":
		return true
	}
	return false
}

type yarnDep struct {
	Name, Constraint, Scope string
}

func parseResults(patternIDs map[string]string, dependsOn map[string][]yarnDep) (deps []types.Dependency) {
	for libID, refs := range dependsOn {
		d := types.Dependency{ID: libID}
		for _, ref := range refs {
			if ref.Name == "" {
				continue
			}
			raw := ref.Name
			if ref.Constraint != "" {
				raw = utils.PackageID(ref.Name, ref.Constraint)
			}
			req := types.Requirement{Target: ref.Name, Constraint: ref.Constraint, Scope: ref.Scope, Condition: raw}
			if id := patternIDs[raw]; id != "" {
				req.Resolved = id
				if ref.Scope != "optional" {
					d.DependsOn = append(d.DependsOn, id)
				}
			}
			d.Requirements = append(d.Requirements, req)
		}
		if len(d.DependsOn) == 0 && len(d.Requirements) == 0 {
			continue
		}
		sort.Strings(d.DependsOn)
		deps = append(deps, d)
	}
	return deps
}

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func scanBlocks(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.Index(data, []byte("\n\n")); i >= 0 {
		// We have a full newline-terminated line.
		return i + 2, data[0:i], nil
	} else if i := bytes.Index(data, []byte("\r\n\r\n")); i >= 0 {
		return i + 4, data[0:i], nil
	}

	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}

func parseBlock(block []byte, lineNum int) (Library, []yarnDep, int, error) {
	return parseBlockContext(context.Background(), block, lineNum)
}
func parseBlockContext(ctx context.Context, block []byte, lineNum int) (lib Library, deps []yarnDep, newLine int, err error) {
	var (
		emptyLines int // lib can start with empty lines first
		skipBlock  bool
		pending    *string
	)

	scanner := &LineScanner{Lines: textdecode.NewLines(ctx, bytes.NewReader(block))}
	next := func() (string, bool) {
		if pending != nil {
			line := *pending
			pending = nil
			return line, true
		}
		if scanner.Scan() {
			return scanner.Text(), true
		}
		return "", false
	}
	for {
		line, ok := next()
		if !ok {
			break
		}

		if len(line) == 0 {
			emptyLines++
			continue
		}

		if line[0] == '#' || skipBlock {
			continue
		}

		// Skip this block
		if strings.HasPrefix(line, "__metadata") {
			skipBlock = true
			continue
		}

		line = strings.TrimPrefix(strings.TrimSpace(line), "\"")

		switch {
		case strings.HasPrefix(line, "resolved "):
			lib.Source = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "resolved ")), `"`)
			continue
		case strings.HasPrefix(line, "integrity "):
			lib.Integrity = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "integrity ")), `"`)
			continue
		case strings.HasPrefix(line, "version"):
			if lib.Version, err = getVersion(line); err != nil {
				skipBlock = true
			}
			continue
		case strings.HasPrefix(line, "dependencies:"):
			more, left, hasLeft := parseDependencies(scanner, "")
			deps = append(deps, more...)
			if hasLeft {
				pending = &left
			}
			continue
		case strings.HasPrefix(line, "optionalDependencies:"):
			more, left, hasLeft := parseDependencies(scanner, "optional")
			deps = append(deps, more...)
			if hasLeft {
				pending = &left
			}
			continue
		}

		// try parse package patterns
		if name, protocol, patterns, patternErr := parsePackagePatterns(line); patternErr == nil {
			if patterns == nil || !validProtocol(protocol) {
				skipBlock = true
				if !ignoreProtocol(protocol) {
					// we need to calculate the last line of the block in order to correctly determine the line numbers of the next blocks
					// store the error. we will handle it later
					err = fmt.Errorf("unknown protocol: '%s', line: %s", protocol, line)
					continue
				}
				continue
			} else {
				lib.Patterns = patterns
				lib.Name = name
				continue
			}
		}
	}

	// in case an unsupported protocol is detected
	// show warning and continue parsing
	if err != nil {
		return Library{}, nil, scanner.LineNum(lineNum), fmt.Errorf("unsupported_syntax: %w", err)
	}

	lib.Location = types.Location{
		StartLine: lineNum + emptyLines,
		EndLine:   scanner.LineNum(lineNum),
	}

	if scanErr := scanner.Err(); scanErr != nil {
		err = scanErr
	}

	return lib, deps, scanner.LineNum(lineNum), err
}

func parseDependencies(scanner *LineScanner, scope string) (deps []yarnDep, leftover string, hasLeftover bool) {
	for scanner.Scan() {
		line := scanner.Text()
		name, version, err := getDependency(line)
		if err != nil {
			return deps, line, true
		}
		if name == "" {
			continue
		}
		deps = append(deps, yarnDep{Name: name, Constraint: version, Scope: scope})
	}
	return deps, "", false
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	lineNumber := 1
	var libs []types.Library

	// patternIDs holds mapping between patterns and library IDs
	// e.g. ajv@^6.5.5 => ajv@6.10.0
	patternIDs := map[string]string{}

	ctx := types.ContextOf(r)
	input, err := textdecode.ReadRaw(ctx, r, budget.From(ctx).Limits.MaxFileBytes)
	if err != nil {
		return nil, nil, err
	}
	dependsOn := map[string][]yarnDep{}
	for len(input) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		advance, block, err := scanBlocks(input, true)
		if err != nil {
			return nil, nil, err
		}
		if advance <= 0 {
			return nil, nil, fmt.Errorf("malformed_input: stalled Yarn block")
		}
		if len(block) > budget.From(ctx).Limits.MaxFieldBytes {
			return nil, nil, fmt.Errorf("resource_limit: Yarn block bytes")
		}
		// Each physical line/descriptor can create bounded regex captures,
		// one record or dependency and identity map entries. Reserve these
		// before parseBlock and before serializing any native IDs.
		slots := bytes.Count(block, []byte{'\n'}) + bytes.Count(block, []byte{','}) + 1
		if err := budget.From(ctx).Working(int64(slots)*4096 + int64(len(block))*64); err != nil {
			return nil, nil, err
		}
		input = input[advance:]
		lib, deps, newLine, err := parseBlockContext(ctx, block, lineNumber)
		lineNumber = newLine + 2
		if err != nil {
			return nil, nil, err
		} else if lib.Name == "" {
			continue
		}

		sort.Strings(lib.Patterns)
		rawID, _ := json.Marshal(lib.Patterns)
		libID := string(rawID)
		declared := digest.ParseDeclared(lib.Integrity)
		item := types.Library{
			ID:                libID,
			Name:              lib.Name,
			Version:           lib.Version,
			Source:            lib.Source,
			Verification:      declared.Canonical,
			DeclaredIntegrity: declared.Original,
			Locations:         []types.Location{lib.Location},
		}
		for _, issue := range declared.Issues {
			item.Diagnostics = append(item.Diagnostics, model.Diagnostic{Code: "malformed_input", Stage: "yarn", Reason: issue, Incomplete: true})
		}
		libs = append(libs, item)

		for _, pattern := range lib.Patterns {
			// e.g.
			//   combined-stream@^1.0.6 => combined-stream@1.0.8
			//   combined-stream@~1.0.6 => combined-stream@1.0.8
			if prior, exists := patternIDs[pattern]; exists && prior != libID {
				return nil, nil, fmt.Errorf("malformed_input: conflicting Yarn descriptor %s", pattern)
			}
			patternIDs[pattern] = libID
			if len(deps) > 0 {
				dependsOn[libID] = deps
			}
		}
	}

	// Replace dependency patterns with library IDs
	// e.g. ajv@^6.5.5 => ajv@6.10.0
	deps := parseResults(patternIDs, dependsOn)
	return libs, deps, nil
}
