package analyzer

import (
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/yaklang/yaklang/common/sca/types"
	"github.com/yaklang/yaklang/common/utils"
)

type Analyzer interface {
	Analyze(int, io.Reader) ([]types.Package, error)
	Match(string, fs.FileInfo) int
}

type Task struct {
	path      string
	matchType int
	f         *os.File
	a         Analyzer
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
	return ag.pkgs
}

func (ag *AnalyzerGroup) Append(a Analyzer) {
	ag.analyzers = append(ag.analyzers, a)
}

func (ag *AnalyzerGroup) Consume(wg *sync.WaitGroup) {
	wg.Add(ag.numWorkers)

	for i := 0; i < ag.numWorkers; i++ {
		go func() {
			defer wg.Done()
			for task := range ag.ch {
				defer func() {
					name := task.f.Name()
					task.f.Close()
					os.Remove(name)
				}()
				pkgs, err := task.a.Analyze(task.matchType, task.f)
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
			task := Task{path: path, matchType: matchType, f: f, a: a}
			ag.ch <- task
		}
	}
	return nil
}
