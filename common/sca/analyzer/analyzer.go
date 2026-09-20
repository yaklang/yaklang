package analyzer

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/model"
	"io/fs"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/lazyfile"
	licenses "github.com/yaklang/yaklang/common/sca/license"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"

	lo "github.com/yaklang/yaklang/common/sca/internal/collection"
)

const (
	headerSize = 4

	AllMode ScanMode = 0
	PkgMode          = 1 << (iota - 1)
	LanguageMode
)

var (
	analyzers   = make(map[TypAnalyzer]Analyzer, 0)
	analyzerTyp = make(map[Analyzer]TypAnalyzer, 0)
)

type (
	TypAnalyzer string
	ScanMode    int
	Analyzer    interface {
		Analyze(AnalyzeFileInfo) ([]*dxtypes.Package, error)
		Match(MatchInfo) int
	}
)

type FileInfo struct {
	Path        string
	Analyzer    Analyzer
	LazyFile    *lazyfile.LazyFile
	MatchStatus int
	filesystem  fi.FileSystem
}

func SetFileInfoFileSystem(fi *FileInfo, fs fi.FileSystem) {
	fi.filesystem = fs
}

type AnalyzeFileInfo struct {
	Self *FileInfo
	// matched file
	MatchedFileInfos map[string]*FileInfo
}

type MatchInfo struct {
	Path       string
	FileInfo   fs.FileInfo
	FileHeader []byte
	fileSystem fi.FileSystem
}

func RegisterAnalyzer(typ TypAnalyzer, a Analyzer) {
	if _, ok := analyzers[typ]; ok {
		return
	}
	analyzers[typ] = a
	analyzerTyp[a] = typ
}

// FilterAnalyzer uses a stable type order and a set. Explicit types are unioned
// with a nonzero mode; ALL with an explicit list selects only that list.
func FilterAnalyzer(mode ScanMode, used []TypAnalyzer) []Analyzer {
	selected := map[TypAnalyzer]bool{}
	for typ := range analyzers {
		if mode == AllMode && len(used) == 0 || mode&PkgMode != 0 && strings.HasSuffix(string(typ), "-pkg") || mode&LanguageMode != 0 && strings.HasSuffix(string(typ), "-lang") {
			selected[typ] = true
		}
	}
	for _, typ := range used {
		if _, ok := analyzers[typ]; ok {
			selected[typ] = true
		}
	}
	names := make([]string, 0, len(selected))
	for typ := range selected {
		names = append(names, string(typ))
	}
	sort.Strings(names)
	out := make([]Analyzer, 0, len(names))
	for _, name := range names {
		out = append(out, analyzers[TypAnalyzer(name)])
	}
	return out
}
func ParseLanguageConfiguration(fi *FileInfo, parser types.Parser) ([]*dxtypes.Package, error) {
	parsedLibs, parsedDeps, err := parser.Parse(fi.filesystem, fi.LazyFile)
	if err != nil {
		return nil, err
	}
	ctx := context.Background()
	if fi != nil && fi.LazyFile != nil {
		ctx = fi.LazyFile.Context()
	}
	return handlerParsedBudget(ctx, parsedLibs, parsedDeps)
}

func handlerParsed(parsedLibs types.Libraries, parsedDeps types.Dependencies) ([]*dxtypes.Package, error) {
	return handlerParsedBudget(context.Background(), parsedLibs, parsedDeps)
}

func handlerParsedBudget(ctx context.Context, parsedLibs types.Libraries, parsedDeps types.Dependencies) ([]*dxtypes.Package, error) {
	st := budget.From(ctx)
	pkgIDMap := make(map[string]*dxtypes.Package, len(parsedLibs))

	for _, lib := range parsedLibs {
		if err := st.Result(budget.SizeOfPackage(lib.Name, lib.Version, lib.Verification)); err != nil {
			return nil, err
		}
		p := dxtypes.Package{

			IsVersionRange: lib.IsVersionRange,
			Verification:   lib.Verification,

			Name:    lib.Name,
			Version: lib.Version, PackageDetails: &dxtypes.PackageDetails{Condition: lib.Condition, Scope: lib.Scope,

				DeclaredCondition: lib.DeclaredCondition,
				Diagnostics:       lib.Diagnostics,
				DeclaredIntegrity: lib.DeclaredIntegrity,

				Instance: lib.ID, Source: lib.Source, Variant: lib.Variant, Evidence: lib.Evidence, DeclaredName: lib.DeclaredName, DeclaredVersion: lib.DeclaredVersion, Indirect: lib.Indirect},
		}
		if lib.Dev {
			p.Scope = "dev"
		}
		p.ArtifactPath = lib.FilePath
		if p.Instance == "" && p.ArtifactPath != "" {
			p.Instance = p.ArtifactPath + "#" + p.Name + "@" + p.Version
		}
		for _, loc := range lib.Locations {
			if err := st.Result(budget.SizeOfObservation()); err != nil {
				return nil, err
			}
			p.Locations = append(p.Locations, dxtypes.SourceRange{StartLine: loc.StartLine, EndLine: loc.EndLine})
		}
		if len(lib.Locations) > 0 {
			p.StartLine = lib.Locations[0].StartLine
			p.EndLine = lib.Locations[0].EndLine
		}
		if lib.License != "" {
			p.RawLicenses = []string{lib.License}
			p.License = lo.Map(strings.Split(lib.License, ","), func(license string, _ int) string {
				return licenses.Normalize(strings.TrimSpace(license))
			})
		}
		id := lib.ID
		if id == "" {
			id = p.Identifier()
		}
		if err := st.Result(budget.SizeMap + budget.SizePtr); err != nil {
			return nil, err
		}
		if prior, exists := pkgIDMap[id]; exists {
			if prior.Identifier() != p.Identifier() {
				return nil, fmt.Errorf("malformed_input: conflicting native reference %q", id)
			}
			prior.Locations = append(prior.Locations, p.Locations...)
			prior.RawLicenses = append(prior.RawLicenses, p.RawLicenses...)
			prior.Diagnostics = append(prior.Diagnostics, p.Diagnostics...)
			if prior.DeclaredIntegrity == "" {
				prior.DeclaredIntegrity = p.DeclaredIntegrity
			} else if p.DeclaredIntegrity != "" && p.DeclaredIntegrity != prior.DeclaredIntegrity {
				prior.DeclaredIntegrity = prior.DeclaredIntegrity + " " + p.DeclaredIntegrity
			}
			continue
		}
		if lib.DeclaredVersion != "" {
			req := model.Requirement{Target: lib.Name, Constraint: lib.DeclaredVersion, Scope: p.Scope}
			if err := st.Result(budget.SizeOfEdge() + budget.SizeOfString(req.Target) + budget.SizeOfString(req.Constraint)); err != nil {
				return nil, err
			}
			p.Requirements = append(p.Requirements, req)
		}
		pkgIDMap[id] = &p
	}

	// parse deps
	for _, dep := range parsedDeps {
		id := dep.ID
		upStreamIDs := dep.DependsOn

		pkg, ok := pkgIDMap[id]
		if !ok {
			continue
		}
		for _, q := range dep.Requirements {
			req := model.Requirement{Target: q.Target, Constraint: q.Constraint, Scope: q.Scope, Condition: q.Condition}
			if up := pkgIDMap[q.Resolved]; q.Resolved != "" && up != nil {
				pkg.LinkDepend(up)
				req.Resolved = []string{up.Instance}
			}
			if err := st.Result(budget.SizeOfEdge() + budget.SizeOfString(q.Target) + budget.SizeOfString(q.Constraint)); err != nil {
				return nil, err
			}
			pkg.Requirements = append(pkg.Requirements, req)
		}
		for _, uid := range upStreamIDs {
			upPkg, ok := pkgIDMap[uid]
			if !ok {
				pkg.UnresolvedDependencies = append(pkg.UnresolvedDependencies, uid)
				continue
			}
			pkg.LinkDepend(upPkg)
		}
	}

	pkgs := make([]*dxtypes.Package, 0, len(pkgIDMap))
	for _, pkg := range pkgIDMap {
		pkgs = append(pkgs, pkg)
	}
	sort.Slice(pkgs, func(i, j int) bool {
		a, b := pkgs[i], pkgs[j]
		if a.Instance != b.Instance {
			return a.Instance < b.Instance
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		return a.Verification < b.Verification
	})
	return pkgs, nil
}

func Name(a Analyzer) string {
	if name, ok := analyzerTyp[a]; ok {
		return string(name)
	}
	return "custom"
}
func MatchInSnapshot(a Analyzer, info MatchInfo, snapshot fi.FileSystem) int {
	info.fileSystem = snapshot
	return a.Match(info)
}
