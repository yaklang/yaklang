package dxtypes

import (
	"fmt"
	"github.com/yaklang/yaklang/common/sca/model"
	"strings"

	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
)

type PackageDetails struct {
	Provides          []string
	Condition, Scope  string
	DeclaredCondition string
	Diagnostics       []model.Diagnostic

	Requirements       []model.Requirement
	StartLine, EndLine int
	Locations          []SourceRange
	ArtifactPath       string

	UnresolvedDependencies []string

	Evidence, DeclaredName, DeclaredVersion, ReplacementVersion, DeclaredIntegrity string
	Indirect                                                                       bool

	// Qualifiers are optional on legacy producers and required by migrated ones.
	Ecosystem, Source, Architecture, Variant, Snapshot, ProjectRoot, Instance string

	RawLicenses []string
}

type Package struct {
	*PackageDetails
	Name           string
	Version        string
	IsVersionRange bool // Version is a version range

	FromFile     []string
	FromAnalyzer []string

	// Optional

	// sha1:abc
	// md5:abc
	// sha256:abc
	// ...
	Verification string

	// RawLicenses preserves original expressions; License is legacy display text.
	License []string

	// Related
	// id -> package
	UpStreamPackages   map[string]*Package
	DownStreamPackages map[string]*Package

	DependsOn PackageRelationShip

	Potential bool

	// 订正 CPE 和 强制关联 CVE
	AmendedCPE    []string
	AssociatedCVE []string
}

type SourceRange struct{ StartLine, EndLine int }

type PackageRelationShip struct {
	And map[string]string   // key: package name, value: version range
	Or  []map[string]string // key: package name, value: version range
}

// Identifier encodes fields separately; a name/version boundary cannot collide.
// Qualifiers and verification keep ecosystems, instances and conflicting evidence apart.
func (p *Package) Identifier() string {
	if p == nil {
		return ""
	}
	raw := p.IdentityDigest()
	return hex.EncodeToString(raw[:])
}

// IdentityDigest uses length-framed fields, preserving arbitrary separator bytes
// without allocating a reflection-based intermediate JSON object for each key.
func (p *Package) IdentityDigest() [32]byte {
	details := p.Details()
	h := sha256.New()
	var length [8]byte
	for _, field := range [...]string{details.Ecosystem, p.Name, p.Version, details.Source, details.Architecture, details.Variant, details.Snapshot, details.ProjectRoot, details.Instance, p.Verification} {
		binary.LittleEndian.PutUint64(length[:], uint64(len(field)))
		h.Write(length[:])
		h.Write([]byte(field))
	}
	var sum [32]byte
	h.Sum(sum[:0])
	return sum
}

func (p *Package) HasVersionRange() bool {
	return p.IsVersionRange || strings.ContainsAny(p.Version, "><=")
}

func (p Package) String() string {
	ret := fmt.Sprintf("%s-%s", p.Name, p.Version)
	ret += "\n\tupstream: "
	ret += strings.Join(
		relationStrings(p.UpStreamPackages),
		",",
	)
	ret += "\n\tdownstream: "
	ret += strings.Join(
		relationStrings(p.DownStreamPackages),
		",",
	)
	ret += "\n\tverfication: " + p.Verification
	ret += "\n\tlicense: " + strings.Join(p.License, ",")
	ret += fmt.Sprintf("\n\tpotential: %v", p.Potential)
	ret += fmt.Sprintf("\n\tdependson: %v", p.DependsOn)
	ret += fmt.Sprintf("\n\tfromAnalyzer: %v", p.FromAnalyzer)
	ret += fmt.Sprintf("\n\tfromFile: %v", p.FromFile)
	return ret
}

func (p *Package) SetFrom(analyzer, file string) {
	if p.FromAnalyzer == nil {
		p.FromAnalyzer = make([]string, 0)
	}
	p.FromAnalyzer = append(p.FromAnalyzer, analyzer)
	if p.FromFile == nil {
		p.FromFile = make([]string, 0)
	}
	p.FromFile = append(p.FromFile, file)
}

func (p *Package) From() ([]string, []string) {
	return p.FromAnalyzer, p.FromFile
}
func (down *Package) LinkDepend(up *Package) {
	if up.DownStreamPackages == nil {
		up.DownStreamPackages = make(map[string]*Package)
	}
	up.DownStreamPackages[down.Identifier()] = down
	if down.UpStreamPackages == nil {
		down.UpStreamPackages = make(map[string]*Package)
	}
	down.UpStreamPackages[up.Identifier()] = up
}

// merge p2 to p1
func (p *Package) Merge(p2 *Package) *Package {
	if CanMerge(p, p2) != 1 {
		return p
	}
	if p.License == nil {
		p.License = make([]string, 0)
	}
	p.License = uniqueStrings(append(p.License, p2.License...))
	if p2.PackageDetails != nil {
		p.EnsureDetails()
		p.MergeDetails(p2.Details())
	}

	if p.FromAnalyzer == nil {
		p.FromAnalyzer = make([]string, 0)
	}
	p.FromAnalyzer = uniqueStrings(append(p.FromAnalyzer, p2.FromAnalyzer...))
	if p.FromFile == nil {
		p.FromFile = make([]string, 0)
	}
	p.FromFile = uniqueStrings(append(p.FromFile, p2.FromFile...))

	pID, p2ID := p.Identifier(), p2.Identifier()
	for _, p2up := range p2.UpStreamPackages {
		p.LinkDepend(p2up)
		if pID != p2ID {
			delete(p2up.DownStreamPackages, p2ID)
		}
	}
	for _, p2down := range p2.DownStreamPackages {
		p2down.LinkDepend(p)
		if pID != p2ID {
			delete(p2down.UpStreamPackages, p2ID)
		}
	}
	return p
}

// CanMerge only permits exact identity. Constraints never select components.
func CanMerge(a, b *Package) int {
	if a != nil && b != nil && a.Identifier() == b.Identifier() && a.Potential == b.Potential && a.Details().Evidence == b.Details().Evidence && a.HasVersionRange() == b.HasVersionRange() {
		return 1
	}
	return 0
}

func relationStrings(pkgs map[string]*Package) []string {
	ret := make([]string, 0, len(pkgs))
	for name, p := range pkgs {
		ret = append(ret, fmt.Sprintf("%s-%s", name, p.Version))
	}
	sort.Strings(ret)
	return ret
}
func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// Details returns a zero value for legacy records that have no extended evidence.
// Use EnsureDetails before mutating the optional evidence.
func (p *Package) Details() PackageDetails {
	if p.PackageDetails == nil {
		return PackageDetails{}
	}
	return *p.PackageDetails
}
func (p *Package) EnsureDetails() {
	if p.PackageDetails == nil {
		p.PackageDetails = &PackageDetails{}
	}
}
func (p *Package) MergeDetails(other PackageDetails) {
	p.EnsureDetails()
	p.RawLicenses = uniqueStrings(append(p.RawLicenses, other.RawLicenses...))
	p.Locations = append(p.Locations, other.Locations...)
	p.Requirements = append(p.Requirements, other.Requirements...)
	p.Diagnostics = append(p.Diagnostics, other.Diagnostics...)
	p.UnresolvedDependencies = uniqueStrings(append(p.UnresolvedDependencies, other.UnresolvedDependencies...))
	p.Provides = uniqueStrings(append(p.Provides, other.Provides...))
}
