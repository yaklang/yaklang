package analyzer

import (
	"bufio"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"io"
	"io/fs"
	"os"
	"strings"
	"sync"

	godeptypes "github.com/aquasecurity/go-dep-parser/pkg/types"

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
	analyzers = make(map[TypAnalyzer]Analyzer, 0)
)

type TypAnalyzer string
type ScanMode int
type Analyzer interface {
	Analyze(AnalyzeFileInfo) ([]dxtypes.Package, error)
	Match(MatchInfo) int
}

type fileInfo struct {
	path        string
	a           Analyzer
	f           *os.File
	matchStatus int
}

type AnalyzeFileInfo struct {
	self fileInfo
	// matched file
	matchedFileInfos map[string]fileInfo
}

type MatchInfo struct {
	path   string
	fi     fs.FileInfo
	header []byte
}

type AnalyzerGroup struct {
	analyzers []Analyzer

	// consume
	ch         chan AnalyzeFileInfo
	numWorkers int

	// return
	pkgs []dxtypes.Package

	// matched file
	matchedFileInfos map[string]fileInfo
}

func RegisterAnalyzer(typ TypAnalyzer, a Analyzer) {
	if _, ok := analyzers[typ]; ok {
		return
	}
	analyzers[typ] = a
}

func FilterAnalyzer(mode ScanMode) []Analyzer {
	ret := make([]Analyzer, 0, len(analyzers))
	if mode == AllMode {
		return lo.MapToSlice(analyzers, func(_ TypAnalyzer, a Analyzer) Analyzer {
			return a
		})
	}

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
	return ret
}

func NewAnalyzerGroup(numWorkers int, scanMode ScanMode) *AnalyzerGroup {
	return &AnalyzerGroup{
		ch:               make(chan AnalyzeFileInfo),
		numWorkers:       numWorkers,
		matchedFileInfos: make(map[string]fileInfo),
		analyzers:        FilterAnalyzer(scanMode),
	}
}

func (ag *AnalyzerGroup) Packages() []dxtypes.Package {
	return lo.UniqBy(ag.pkgs, func(item dxtypes.Package) string {
		return item.Identifier()
	})
}

func (ag *AnalyzerGroup) Append(a ...Analyzer) {
	ag.analyzers = append(ag.analyzers, a...)
}

func (ag *AnalyzerGroup) Consume(wg *sync.WaitGroup) {
	wg.Add(ag.numWorkers)

	for i := 0; i < ag.numWorkers; i++ {
		go func() {
			defer wg.Done()
			for fileInfo := range ag.ch {
				pkgs, err := fileInfo.self.a.Analyze(fileInfo)
				if err == nil {
					ag.pkgs = append(ag.pkgs, pkgs...)
				}
			}
		}()
	}
}

func (ag *AnalyzerGroup) Clear() {
	for _, info := range ag.matchedFileInfos {
		name := info.f.Name()
		info.f.Close()
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
			path:   path,
			fi:     fi,
			header: header,
		})

		if matchStatus == 0 {
			continue
		}
		// match type > 0 mean matched and need to analyze

		// save
		f, err := os.CreateTemp("", "fanal-file-*")
		if err != nil {
			return utils.Errorf("failed to create a temporary file for analyzer")
		}

		if _, err := io.Copy(f, br); err != nil {
			return utils.Errorf("failed to copy the file: %v", err)
		}
		f.Seek(0, 0)

		// add to scanned files
		ag.matchedFileInfos[path] = fileInfo{
			path:        path,
			a:           a,
			f:           f,
			matchStatus: matchStatus,
		}
	}
	return nil
}

func (ag *AnalyzerGroup) Analyze() error {
	for _, info := range ag.matchedFileInfos {
		ag.ch <- AnalyzeFileInfo{
			self:             info,
			matchedFileInfos: ag.matchedFileInfos,
		}
	}
	close(ag.ch)
	return nil
}

func ParseLanguageConfiguration(fi fileInfo, parser godeptypes.Parser) ([]dxtypes.Package, error) {
	parsedLibs, _, err := parser.Parse(fi.f)
	if err != nil {
		return nil, err
	}

	pkgs := lo.Map(parsedLibs, func(lib godeptypes.Library, index int) dxtypes.Package {
		return dxtypes.Package{
			Name:     lib.Name,
			Version:  lib.Version,
			Indirect: lib.Indirect,
		}
	})
	return pkgs, nil
}
