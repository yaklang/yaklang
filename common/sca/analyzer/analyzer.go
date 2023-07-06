package analyzer

import (
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/sca/types"
	"github.com/yaklang/yaklang/common/utils"
)

type Analyzer interface {
	Analyze(AnalyzeFileInfo) ([]types.Package, error)
	Match(string, fs.FileInfo) int
}

type AnalyzeFileInfo struct {
	path      string
	f         *os.File
	matchType int
}

type Task struct {
	fileInfo AnalyzeFileInfo
	a        Analyzer
}

type AnalyzerGroup struct {
	analyzers []Analyzer

	// consume
	ch         chan Task
	numWorkers int

	// return
	pkgs []types.Package
	err  error
}

func NewAnalyzerGroup(numWorkers int) *AnalyzerGroup {
	return &AnalyzerGroup{
		ch:         make(chan Task),
		numWorkers: numWorkers,
	}
}

func (ag *AnalyzerGroup) Error() error {
	return ag.err
}

func (ag *AnalyzerGroup) Packages() []types.Package {
	return funk.Uniq(ag.pkgs).([]types.Package)
}

func (ag *AnalyzerGroup) Append(a ...Analyzer) {
	ag.analyzers = append(ag.analyzers, a...)
}

func (ag *AnalyzerGroup) Consume(wg *sync.WaitGroup) {
	wg.Add(ag.numWorkers)

	for i := 0; i < ag.numWorkers; i++ {
		go func() {
			defer wg.Done()
			for task := range ag.ch {
				defer func() {
					name := task.fileInfo.f.Name()
					task.fileInfo.f.Close()
					os.Remove(name)
				}()
				pkgs, err := task.a.Analyze(task.fileInfo)
				if err != nil {
					ag.err = err
					return
				}
				ag.pkgs = append(ag.pkgs, pkgs...)
			}
		}()
	}
}

func (ag *AnalyzerGroup) Close() {
	close(ag.ch)
}

// write
func (ag *AnalyzerGroup) Analyze(path string, fi fs.FileInfo, r io.Reader) error {
	for _, a := range ag.analyzers {
		// match type > 0 mean matched and need to analyze
		if matchType := a.Match(path, fi); matchType > 0 {
			// save
			f, err := os.CreateTemp("", "fanal-file-*")
			if err != nil {
				return utils.Errorf("failed to create a temporary file for analyzer")
			}
			if _, err := io.Copy(f, r); err != nil {
				return utils.Errorf("failed to copy the file: %v", err)
			}
			f.Seek(0, 0)

			// send
			task := Task{
				fileInfo: AnalyzeFileInfo{
					path:      path,
					f:         f,
					matchType: matchType,
				},
				a: a,
			}
			ag.ch <- task
		}
	}
	return nil
}
