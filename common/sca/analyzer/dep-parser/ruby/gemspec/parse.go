package gemspec

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"

	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

const specNewStr = "Gem::Specification.new"

var (
	// Capture the variable name
	// e.g. Gem::Specification.new do |s|
	//      => s
	newVarRegexp = regexp.MustCompile(`\|(?P<var>.*)\|`)

	// Capture the value of "name"
	// e.g. s.name = "async".freeze
	//      => "async".freeze
	nameRegexp = regexp.MustCompile(`\.name\s*=\s*(?P<name>\S+)`)

	// Capture the value of "version"
	// e.g. s.version = "1.2.3"
	//      => "1.2.3"
	versionRegexp = regexp.MustCompile(`\.version\s*=\s*(?P<version>\S+)`)

	// Capture the value of "license"
	// e.g. s.license = "MIT"
	//      => "MIT"
	licenseRegexp = regexp.MustCompile(`\.license\s*=\s*(?P<license>\S+)`)

	// Capture the value of "licenses"
	// e.g. s.license = ["MIT".freeze, "BSDL".freeze]
	//      => "MIT".freeze, "BSDL".freeze
	licensesRegexp = regexp.MustCompile(`\.licenses\s*=\s*\[(?P<licenses>.+)\]`)
)

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

func (p *Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) (libs []types.Library, deps []types.Dependency, err error) {
	var newVar, name, version, license string

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), budget.From(types.ContextOf(r)).Limits.MaxFieldBytes)
	for scanner.Scan() {
		if err := types.ContextOf(r).Err(); err != nil {
			return nil, nil, err
		}
		line := strings.TrimSpace(scanner.Text())
		if strings.Contains(line, specNewStr) {
			newVar = findSubString(newVarRegexp, line, "var")
		}

		if newVar == "" {
			continue
		}

		// Capture name, version, license, and licenses
		switch {
		case strings.HasPrefix(line, fmt.Sprintf("%s.name", newVar)):
			// https://guides.rubygems.org/specification-reference/#name
			name, err = literalAssignment(line)
			if err != nil {
				return nil, nil, err
			}
		case strings.HasPrefix(line, fmt.Sprintf("%s.version", newVar)):
			// https://guides.rubygems.org/specification-reference/#version
			version, err = literalAssignment(line)
			if err != nil {
				return nil, nil, err
			}
		case strings.HasPrefix(line, fmt.Sprintf("%s.licenses", newVar)):
			// https://guides.rubygems.org/specification-reference/#licenses=
			license = findSubString(licensesRegexp, line, "licenses")
			license = parseLicenses(license)
		case strings.HasPrefix(line, fmt.Sprintf("%s.license", newVar)):
			// https://guides.rubygems.org/specification-reference/#license=
			license = findSubString(licenseRegexp, line, "license")
			license = trim(license)
		}

	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to parse gemspec: %w", err)
	}

	if name == "" || version == "" {
		return nil, nil, errors.New("failed to parse gemspec")
	}

	return []types.Library{
		{
			Name:    name,
			Version: version,
			License: license,
		},
	}, nil, nil
}

// Evaluate no Ruby. Even a quoted prefix followed by an expression is not a
// static value, and interpolation must not become a fictitious package version.
func literalAssignment(line string) (string, error) {
	_, s, ok := strings.Cut(line, "=")
	s = strings.TrimSpace(s)
	// A trailing comment outside the closed literal is not part of its value.
	if len(s) > 1 && (s[0] == '\'' || s[0] == '"') {
		if end := strings.IndexByte(s[1:], s[0]); end >= 0 {
			end++
			tail := strings.TrimSpace(s[end+1:])
			tail = strings.TrimSpace(strings.TrimPrefix(tail, ".freeze"))
			if strings.HasPrefix(tail, "#") {
				s = s[:end+1]
			}
		}
	}
	s = strings.TrimSuffix(s, ".freeze")
	if !ok || len(s) < 2 || (s[0] != '\'' && s[0] != '"') || s[len(s)-1] != s[0] || strings.ContainsAny(s[1:len(s)-1], "\\\"'`#") {
		return "", fmt.Errorf("failed to parse gemspec: unsupported_syntax: dynamic Ruby assignment")
	}
	return s[1 : len(s)-1], nil
}

func findSubString(re *regexp.Regexp, line, name string) string {
	m := re.FindStringSubmatch(line)
	if m == nil {
		return ""
	}
	return m[re.SubexpIndex(name)]
}

// Trim single quotes, double quotes and ".freeze"
// e.g. "async".freeze => async
func trim(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".freeze")
	return strings.Trim(s, `'"`)
}

func parseLicenses(s string) string {
	// e.g. `"Ruby".freeze, "BSDL".freeze`
	//      => {"\"Ruby\".freeze", "\"BSDL\".freeze"}
	ss := strings.Split(s, ",")

	// e.g. {"\"Ruby\".freeze", "\"BSDL\".freeze"}
	//      => {"Ruby", "BSDL"}
	var licenses []string
	for _, l := range ss {
		licenses = append(licenses, trim(l))
	}

	return strings.Join(licenses, ", ")
}
