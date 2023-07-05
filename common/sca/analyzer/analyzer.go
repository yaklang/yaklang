package analyzer

import (
	"context"
	"io"
	"io/fs"
	"os"

	"github.com/yaklang/yaklang/common/sca/types"
	"github.com/yaklang/yaklang/common/utils"
)

type Analyzer interface {
	Analyze(string, io.Reader) ([]types.Package, error)
	Match(string, fs.FileInfo) bool
}

type Task struct {
	path string
	f    *os.File
	a    Analyzer
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

func (ag *AnalyzerGroup) Consume(ctx context.Context, cancel func()) {
	for i := 0; i < ag.numWorkers; i++ {
		go func() {
			for {
				select {
				case task, ok := <-ag.ch:
					// finish
					if !ok {
						cancel()
						return
					}
					defer func() {
						name := task.f.Name()
						task.f.Close()
						os.Remove(name)
					}()
					pkgs, err := task.a.Analyze(task.path, task.f)
					if err != nil {
						ag.err = err
						cancel()
						return
					}
					ag.pkgs = append(ag.pkgs, pkgs...)
				case <-ctx.Done():
					return
				}

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
		// match
		if a.Match(path, fi) {
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
			task := Task{path: path, f: f, a: a}
			ag.ch <- task
		}
	}
	return nil
}
