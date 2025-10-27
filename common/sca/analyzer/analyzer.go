package analyzer

import (
	"bufio"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/lazyfile"
	licenses "github.com/yaklang/yaklang/common/sca/license"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"

	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
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

type AnalyzerGroup struct {
	fileSystem fi.FileSystem

	analyzers []Analyzer

	// consume
	ch         chan AnalyzeFileInfo
	numWorkers int

	// return
	pkgLock sync.Mutex
	pkgs    []*dxtypes.Package

	// matched file
	matchedFileInfos map[string]*FileInfo
}

func RegisterAnalyzer(typ TypAnalyzer, a Analyzer) {
	if _, ok := analyzers[typ]; ok {
		return
	}
	analyzers[typ] = a
	analyzerTyp[a] = typ
}

func FilterAnalyzer(mode ScanMode, usedAnalyzers []TypAnalyzer) []Analyzer {
	if mode == AllMode && len(usedAnalyzers) == 0 {
		return lo.MapToSlice(analyzers, func(_ TypAnalyzer, a Analyzer) Analyzer {
			return a
		})
	}

	ret := make([]Analyzer, 0, len(analyzers))
	for analyzerName, a := range analyzers {
		// filter by ScanMode
		if mode&PkgMode == PkgMode {
			if strings.HasSuffix(string(analyzerName), "-pkg") {
				ret = append(ret, a)
				continue
			}
		}
		if mode&LanguageMode == LanguageMode {
			if strings.HasSuffix(string(analyzerName), "-lang") {
				ret = append(ret, a)
				continue
			}
		}

	}

	for _, typ := range usedAnalyzers {
		if a, ok := analyzers[typ]; ok {
			ret = append(ret, a)
		}
	}
	return ret
}

func NewAnalyzerGroup(numWorkers int, scanMode ScanMode, usedAnalyzers []TypAnalyzer, customAnalyzers []Analyzer, fis ...fi.FileSystem) *AnalyzerGroup {
	var fi fi.FileSystem
	if len(fis) > 0 {
		fi = fis[0]
	}
	if fi == nil {
		fi = filesys.NewLocalFs()
	}
	ag := &AnalyzerGroup{
		ch:               make(chan AnalyzeFileInfo),
		numWorkers:       numWorkers,
		matchedFileInfos: make(map[string]*FileInfo),
		analyzers:        FilterAnalyzer(scanMode, usedAnalyzers),
		pkgs:             make([]*dxtypes.Package, 0),
		fileSystem:       fi,
	}
	if len(customAnalyzers) > 0 {
		ag.AddAnalyzers(customAnalyzers...)
	}
	return ag
}

func (ag *AnalyzerGroup) AddAnalyzers(analyzers ...Analyzer) {
	ag.analyzers = append(ag.analyzers, analyzers...)
}

func (ag *AnalyzerGroup) Packages() []*dxtypes.Package {
	return MergePackages(ag.pkgs)
}

func (ag *AnalyzerGroup) Consume(wg *sync.WaitGroup) {
	wg.Add(ag.numWorkers)

	for i := 0; i < ag.numWorkers; i++ {
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Errorf("analyzer worker panic: %v", r)
					utils.PrintCurrentGoroutineRuntimeStack()
				}
			}()
			defer wg.Done()
			for fileInfo := range ag.ch {
				pkgs, err := fileInfo.Self.Analyzer.Analyze(fileInfo)
				if err == nil {
					for _, pkg := range pkgs {
						a := fileInfo.Self.Analyzer
						if name, ok := analyzerTyp[a]; ok {
							pkg.SetFrom(string(name), fileInfo.Self.Path)
						} else {
							pkg.SetFrom("custom", fileInfo.Self.Path)
						}
					}
					ag.pkgLock.Lock()
					ag.pkgs = append(ag.pkgs, pkgs...)
					ag.pkgLock.Unlock()
				}
			}
		}()
	}
}

func (ag *AnalyzerGroup) Clear() {
	for _, info := range ag.matchedFileInfos {
		name := info.LazyFile.Name()
		info.LazyFile.Close()
		os.Remove(name)
	}
}

func (ag *AnalyzerGroup) Match(path string, fi fs.FileInfo, r io.Reader) error {
	var (
		header []byte
		err    error
	)
	br := bufio.NewReader(r)

	for _, a := range ag.analyzers {
		// if scanned, skip
		if _, ok := ag.matchedFileInfos[path]; ok {
			continue
		}

		if fi.Mode().IsRegular() {
			header, err = br.Peek(headerSize)
			if err != nil && err != io.EOF {
				return utils.Errorf("read file header error: %v", err)
			}
		}

		matchStatus := a.Match(MatchInfo{
			Path:       path,
			FileInfo:   fi,
			FileHeader: header,
			fileSystem: ag.fileSystem,
		})

		if matchStatus == 0 {
			continue
		}
		// match type > 0 mean matched and need to analyze

		// save to local instead of other file system
		f, err := os.CreateTemp("", "fanal-file-*")
		if err != nil {
			return utils.Errorf("failed to create analyzer temporary file")
		}
		if _, err := io.Copy(f, br); err != nil {
			return utils.Errorf("failed to copy the file: %v", err)
		}
		f.Close()

		// add to scanned files, use local fs instead
		fs := filesys.NewLocalFs()
		ag.matchedFileInfos[path] = &FileInfo{
			Path:        path,
			Analyzer:    a,
			LazyFile:    lazyfile.LazyOpenStreamByFile(fs, f),
			MatchStatus: matchStatus,
			filesystem:  fs,
		}
	}
	return nil
}

func (ag *AnalyzerGroup) Analyze() error {
	for _, info := range ag.matchedFileInfos {
		ag.ch <- AnalyzeFileInfo{
			Self:             info,
			MatchedFileInfos: ag.matchedFileInfos,
		}
	}
	close(ag.ch)
	return nil
}

func ParseLanguageConfiguration(fi *FileInfo, parser types.Parser) ([]*dxtypes.Package, error) {
	parsedLibs, parsedDeps, err := parser.Parse(fi.filesystem, fi.LazyFile)
	if err != nil {
		return nil, err
	}
	return handlerParsed(parsedLibs, parsedDeps)
}

func handlerParsed(parsedLibs types.Libraries, parsedDeps types.Dependencies) ([]*dxtypes.Package, error) {
	pkgIDMap := make(map[string]*dxtypes.Package, len(parsedLibs))

	for _, lib := range parsedLibs {
		p := dxtypes.Package{
			Name:    lib.Name,
			Version: lib.Version,
		}
		if lib.License != "" {
			p.License = lo.Map(strings.Split(lib.License, ","), func(license string, _ int) string {
				return licenses.Normalize(strings.TrimSpace(license))
			})
		}
		id := lib.ID
		if id == "" {
			id = p.Identifier()
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
		for _, uid := range upStreamIDs {
			upPkg, ok := pkgIDMap[uid]
			if !ok {
				data := strings.Split(uid, "@")
				// name(data[0]) => version(data[1])
				// pkg.DependsOn.And[data[0]] = data[1]
				upPkg = &dxtypes.Package{
					Name:    data[0],
					Version: data[1],
				}
				pkgIDMap[uid] = upPkg
			}
			pkg.LinkDepend(upPkg)
		}
	}

	return lo.MapToSlice(pkgIDMap, func(_ string, pkg *dxtypes.Package) *dxtypes.Package {
		return pkg
	}), nil
}
