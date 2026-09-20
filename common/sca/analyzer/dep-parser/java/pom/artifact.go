package pom

import (
	"fmt"
	"regexp"
	"strings"

	lo "github.com/yaklang/yaklang/common/sca/internal/collection"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
)

var varRegexp = regexp.MustCompile(`\${(\S+?)}`)

type artifact struct {
	GroupID    string
	ArtifactID string
	Version    version
	Licenses   []string

	Exclusions map[string]struct{}

	Module bool
	Root   bool
	Direct bool

	Locations types.Locations
}

func newArtifact(groupID, artifactID, version string, licenses []string, props map[string]string) artifact {
	return artifact{
		GroupID:    evaluateVariable(groupID, props, nil),
		ArtifactID: evaluateVariable(artifactID, props, nil),
		Version:    newVersion(evaluateVariable(version, props, nil)),
		Licenses:   licenses,
	}
}

func (a artifact) IsEmpty() bool {
	return a.GroupID == "" || a.ArtifactID == ""
}

func (a artifact) Equal(o artifact) bool {
	return a.GroupID == o.GroupID && a.ArtifactID == o.ArtifactID && a.Version.String() == o.Version.String()
}

func (a artifact) JoinLicenses() string {
	return strings.Join(a.Licenses, ", ")
}

func (a artifact) ToPOMLicenses() pomLicenses {
	return pomLicenses{
		License: lo.Map(a.Licenses, func(lic string, _ int) pomLicense {
			return pomLicense{Name: lic}
		}),
	}
}

func (a artifact) Inherit(parent artifact) artifact {
	// inherited from a parent
	if a.GroupID == "" {
		a.GroupID = parent.GroupID
	}

	if len(a.Licenses) == 0 {
		a.Licenses = parent.Licenses
	}

	if a.Version.String() == "" {
		a.Version = parent.Version
	}
	return a
}

func (a artifact) Name() string {
	return fmt.Sprintf("%s:%s", a.GroupID, a.ArtifactID)
}

func (a artifact) String() string {
	return fmt.Sprintf("%s:%s", a.Name(), a.Version)
}

type version struct {
	ver  string
	hard bool
}

// Only soft and hard requirements for the specified version are supported at the moment.
func newVersion(s string) version {
	var hard bool
	if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") && !strings.Contains(s, ",") {
		s = strings.Trim(s, "[]")
		hard = true
	}

	return version{
		ver:  s,
		hard: hard,
	}
}

func (v1 version) shouldOverride(v2 version) bool {
	if !v1.hard && v2.hard {
		return true
	}
	return false
}

func (v1 version) String() string {
	return v1.ver
}

func evaluateVariable(s string, props map[string]string, seenProps []string) string {
	active := map[string]bool{}
	steps := 0
	var expand func(string, int) string
	expand = func(text string, depth int) string {
		if depth >= 64 || len(text) > 1<<20 {
			return text
		}
		matches := varRegexp.FindAllStringSubmatchIndex(text, -1)
		var out strings.Builder
		last := 0
		for _, m := range matches {
			steps++
			if steps > 10000 {
				return text
			}
			key := text[m[2]:m[3]]
			value := text[m[0]:m[1]]
			if raw, ok := props[key]; ok && !active[key] && !strings.HasPrefix(key, "env.") {
				active[key] = true
				value = expand(raw, depth+1)
				delete(active, key)
			}
			if out.Len()+m[0]-last+len(value) > 1<<20 {
				return text
			}
			out.WriteString(text[last:m[0]])
			out.WriteString(value)
			last = m[1]
		}
		if out.Len()+len(text)-last > 1<<20 {
			return text
		}
		out.WriteString(text[last:])
		return out.String()
	}
	return expand(s, 0)
}
