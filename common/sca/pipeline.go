package sca

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/yaklang/yaklang/common/sca/analyzer"
	"github.com/yaklang/yaklang/common/sca/core/budget"
	"github.com/yaklang/yaklang/common/sca/core/scanerr"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
	"github.com/yaklang/yaklang/common/sca/internal/fsio"
	"github.com/yaklang/yaklang/common/sca/lazyfile"
	"github.com/yaklang/yaklang/common/sca/model"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
)

type material struct {
	info fs.FileInfo
	once sync.Once
	data []byte
	err  error
}
type materials struct {
	source fs.FS
	ctx    context.Context
	files  map[string]*material
	dirs   map[string]fs.FileInfo
	limits ResourceLimits
	mu     sync.Mutex
	total  int64
}

func (m *materials) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
		return nil, fs.ErrPermission
	}
	v, ok := m.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	v.once.Do(func() {
		if v.err = m.ctx.Err(); v.err != nil {
			return
		}
		if v.info.Size() < 0 || v.info.Size() > m.limits.MaxFileBytes {
			v.err = scanerr.New(scanerr.ResourceLimit, "file bytes %s", name)
			return
		}
		f, err := m.source.Open(name)
		if err != nil {
			v.err = err
			return
		}
		defer f.Close()
		before, err := f.Stat()
		if err != nil {
			v.err = err
			return
		}
		if !sameMaterial(v.info, before) {
			v.err = scanerr.New(scanerr.InputChanged, "%s", name)
			return
		}
		if err := budget.From(m.ctx).Once("material:"+name, 1, budget.SizeOfBytes(int(v.info.Size()))); err != nil {
			v.err = err
			return
		}
		var data []byte
		if v.info.Size() > 0 {
			data = make([]byte, 0, int(v.info.Size()))
		}
		buf := make([]byte, 32<<10)
		for {
			if err = m.ctx.Err(); err != nil {
				v.err = err
				return
			}
			n, e := f.Read(buf)
			if n > 0 {
				m.mu.Lock()
				m.total += int64(n)
				over := budget.From(m.ctx).Read(int64(n)) != nil
				m.mu.Unlock()
				if over || int64(len(data)+n) > m.limits.MaxFileBytes {
					v.err = scanerr.New(scanerr.ResourceLimit, "snapshot reads")
					return
				}
				data = append(data, buf[:n]...)
			}
			if e == io.EOF {
				break
			}
			if e != nil {
				v.err = e
				return
			}
			if n == 0 {
				v.err = io.ErrNoProgress
				return
			}
		}
		after, err := f.Stat()
		if err != nil {
			v.err = err
			return
		}
		if !sameMaterial(before, after) || int64(len(data)) != before.Size() {
			v.err = scanerr.New(scanerr.InputChanged, "%s", name)
			return
		}
		v.data = data
	})
	if v.err != nil {
		return nil, v.err
	}
	lf := lazyfile.NewMemory(name, v.data)
	lf.SetContext(m.ctx)
	return lf, nil
}
func sameMaterial(a, b fs.FileInfo) bool {
	if a.Mode() != b.Mode() || a.Size() != b.Size() || !a.ModTime().Equal(b.ModTime()) || !b.Mode().IsRegular() {
		return false
	}
	if a.Sys() != nil && b.Sys() != nil {
		return os.SameFile(a, b)
	}
	return true
}
func (m *materials) Stat(name string) (fs.FileInfo, error) {
	if f := m.files[name]; f != nil {
		return f.info, nil
	}
	if d := m.dirs[name]; d != nil {
		return d, nil
	}
	return nil, fs.ErrNotExist
}
func (m *materials) ReadDir(name string) ([]fs.DirEntry, error) {
	if _, ok := m.dirs[name]; !ok {
		return nil, fs.ErrNotExist
	}
	var out []fs.DirEntry
	for p, v := range m.files {
		if path.Dir(p) == name {
			out = append(out, fs.FileInfoToDirEntry(v.info))
		}
	}
	for p, v := range m.dirs {
		if p != "." && path.Dir(p) == name {
			out = append(out, fs.FileInfoToDirEntry(v))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out, nil
}
func (m *materials) identity() string {
	names := make([]string, 0, len(m.files))
	for n, v := range m.files {
		if v.data != nil {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		fmt.Fprintf(h, "%d:%s:%d:", len(n), n, len(m.files[n].data))
		h.Write(m.files[n].data)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type scanJob struct {
	info *analyzer.FileInfo
	name string
}
type jobResult struct {
	pkgs       []*dxtypes.Package
	err        error
	file, name string
}

// ScanReport scans only the explicitly supplied immutable filesystem snapshot.
// Incomplete reports always carry diagnostics and a non-nil error. References
// can open only regular files discovered under this snapshot's logical root.
func ScanReport(ctx context.Context, input fs.FS, opts ...ScanOption) (*model.Report, error) {
	c := NewConfig()
	for _, opt := range opts {
		opt(c)
	}
	c.fs = fsio.New(input)
	_, r, e := scanPipeline(ctx, input, c)
	return r, e
}
func scanPipeline(ctx context.Context, input fs.FS, c *ScanConfig) ([]*dxtypes.Package, *model.Report, error) {
	report := &model.Report{Complete: true}
	var classified []error
	fail := func(code, file string, e error) {
		e = scanerr.Wrap(code, e)
		if file != "" {
			e = scanerr.WithFile(e, file)
		}
		if code == "" || scanerr.CodeOf(e) != "" && scanerr.CodeOf(e) != code {
			code = scanerr.CodeOf(e)
		}
		if code == "" {
			code = scanerr.MalformedInput
		}
		report.Complete = false
		reason := ""
		if e != nil {
			reason = e.Error()
			classified = append(classified, e)
		}
		report.Diagnostics = append(report.Diagnostics, model.Diagnostic{Code: code, Stage: "scan", File: file, Reason: reason, Incomplete: true})
	}
	if ctx == nil || input == nil {
		fail(scanerr.InvalidInput, "", fmt.Errorf("nil scan context or filesystem"))
		return nil, report, errors.Join(classified...)
	}
	if c.numWorkers < 1 || c.numWorkers > 64 {
		fail(scanerr.InvalidConfig, "", fmt.Errorf("worker count must be between 1 and 64"))
		return nil, report, errors.Join(classified...)
	}
	limits, err := c.limits.Normalize()
	if err != nil {
		fail(scanerr.InvalidConfig, "", err)
		return nil, report, errors.Join(classified...)
	}
	ctx = budget.Bind(ctx, limits)
	m := &materials{source: input, ctx: ctx, files: map[string]*material{}, dirs: map[string]fs.FileInfo{}, limits: limits}
	count := 0
	err = fs.WalkDir(input, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		count++
		if count > limits.MaxFiles {
			return scanerr.New(scanerr.ResourceLimit, "discovered files")
		}
		if !fs.ValidPath(name) || strings.ContainsAny(name, `\:`) {
			return scanerr.New(scanerr.InvalidPath, "%s", name)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			if err := budget.From(ctx).Result(budget.SizeObject + budget.SizeOfString(name)); err != nil {
				return err
			}
			m.dirs[name] = info
			return nil
		}
		if !info.Mode().IsRegular() {
			fail("unsupported_input", name, fmt.Errorf("non-regular material omitted"))
			return nil
		}
		if err := budget.From(ctx).Result(budget.SizeObject + budget.SizeOfString(name)); err != nil {
			return err
		}
		m.files[name] = &material{info: info}
		return nil
	})
	if err != nil {
		fail(scanerr.CodeOf(err), "", err)
		report.Normalize()
		return nil, report, errors.Join(classified...)
	}
	selected := analyzer.FilterAnalyzer(c.scanMode, c.usedAnalyzers)
	selected = append(selected, c.customAnalyzers...)
	wrapped := fsio.New(m)
	names := make([]string, 0, len(m.files))
	for n := range m.files {
		names = append(names, n)
	}
	sort.Strings(names)
	var jobs []scanJob
	matched := map[string]*analyzer.FileInfo{}
	for _, name := range names {
		if err = ctx.Err(); err != nil {
			fail("cancelled", name, err)
			break
		}
		v := m.files[name]
		// Matchers requiring binary signatures get a bounded read; no content match
		// may infer a host path from a filename or open an undiscovered file.
		var header []byte
		f, e := input.Open(name)
		if e != nil {
			fail(scanerr.CodeOf(e), name, e)
			continue
		}
		actual, e := f.Stat()
		if e == nil && !sameMaterial(v.info, actual) {
			e = scanerr.New(scanerr.InputChanged, "%s", name)
		}
		if e == nil {
			header = make([]byte, 4)
			n, re := io.ReadFull(f, header)
			header = header[:n]
			if re != nil && re != io.EOF && re != io.ErrUnexpectedEOF {
				e = re
			}
			m.total += int64(n)
			if er := budget.From(ctx).Read(int64(n)); er != nil {
				e = er
			}
		}
		f.Close()
		if e != nil {
			fail(scanerr.CodeOf(e), name, e)
			continue
		}
		if m.total > limits.MaxTotalReadBytes {
			fail(scanerr.ResourceLimit, name, scanerr.New(scanerr.ResourceLimit, "snapshot header read budget"))
			break
		}
		for _, a := range selected {
			status, matchErr := safeMatch(a, analyzer.MatchInfo{Path: name, FileInfo: v.info, FileHeader: header}, wrapped)
			if matchErr != nil {
				fail("internal_error", name, matchErr)
				continue
			}
			if status == 0 {
				continue
			}
			if len(jobs) >= limits.MaxCandidateFiles {
				fail("resource_limit", name, fmt.Errorf("candidate count"))
				break
			}
			if analyzer.Name(a) == string(analyzer.TypRPM) && strings.HasSuffix(name, ".sqlite") {
				active := false
				for _, suffix := range []string{"-wal", "-journal", "-shm"} {
					if side, ok := m.files[name+suffix]; ok && side.info.Size() > 0 {
						active = true
					}
				}
				if active {
					fail("evidence_insufficient", name, fmt.Errorf("RPM SQLite snapshot contains transaction sidecars; caller must provide a consistent checkpointed snapshot"))
					continue
				}
			}
			file, e := m.Open(name)
			if e != nil {
				fail(scanerr.CodeOf(e), name, e)
				break
			}
			lf := file.(*lazyfile.LazyFile)
			lf.SetContext(ctx)
			info := &analyzer.FileInfo{Path: name, Analyzer: a, LazyFile: lf, MatchStatus: status}
			analyzer.SetFileInfoFileSystem(info, wrapped)
			if _, ok := matched[name]; !ok {
				matched[name] = info
			}
			if err := budget.From(ctx).Result(budget.SizeOfJob()); err != nil {
				fail(scanerr.CodeOf(err), name, err)
				break
			}
			jobs = append(jobs, scanJob{info, analyzer.Name(a)})
		}
	}
	results := make([]jobResult, len(jobs))
	work := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < c.numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range work {
				j := jobs[i]
				results[i] = safeAnalyze(ctx, j, matched)
				j.info.LazyFile.Close()
			}
		}()
	}
	for i := range jobs {
		work <- i
	}
	close(work)
	wg.Wait()
	if budget.From(ctx).Exhausted() {
		// Which worker reaches the shared limit first must not select the partial
		// graph. Discovery diagnostics were produced in sorted input order.
		results = nil
		fail("resource_limit", "", fmt.Errorf("shared scan read or archive budget exhausted; graph discarded"))
	}
	snapshot := c.snapshot
	if snapshot == "" {
		snapshot = m.identity()
	}
	var pkgs []*dxtypes.Package
	for _, r := range results {
		if r.err != nil {
			fail(errorCode(r.err), r.file, r.err)
		}
		for _, p := range r.pkgs {
			if p == nil {
				fail("internal_error", r.file, fmt.Errorf("nil package"))
				continue
			}
			p.EnsureDetails()
			p.SetFrom(r.name, r.file)
			if p.ArtifactPath != "" {
				artifact := p.ArtifactPath
				if artifact != r.file && !strings.HasPrefix(artifact, r.file+"!/") {
					artifact = r.file + "!/" + artifact
				}
				p.FromFile = []string{artifact}
			}
			if p.Evidence == "" && manifestEvidence(r.file) {
				p.Evidence = "declared"
			}
			if p.Ecosystem == "" {
				p.Ecosystem = ecosystem(r.name)
			}
			p.Snapshot = snapshot
			p.ProjectRoot = path.Dir(r.file)
			if p.Instance == "" {
				p.Instance = r.file + "#" + p.Name + "@" + p.Version
			} else {
				p.Instance = r.file + "#" + p.Instance
			}
			for i := range p.Requirements {
				for k := range p.Requirements[i].Resolved {
					p.Requirements[i].Resolved[k] = r.file + "#" + p.Requirements[i].Resolved[k]
				}
			}
			pkgs = append(pkgs, p)
		}
	}
	if len(pkgs) > limits.MaxObservations {
		fail("resource_limit", "", fmt.Errorf("observations"))
		pkgs = nil
	}
	classified = append(classified, fillReport(report, pkgs, limits, budget.From(ctx))...)
	report.Normalize()
	if err = ctx.Err(); err != nil {
		fail("cancelled", "", err)
		report.Normalize()
	}
	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].Identifier() < pkgs[j].Identifier() })
	return pkgs, report, errors.Join(classified...)
}
func safeMatch(a analyzer.Analyzer, info analyzer.MatchInfo, source *fsio.Snapshot) (status int, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("internal_error: match panic: %v", p)
		}
	}()
	return analyzer.MatchInSnapshot(a, info, source), nil
}
func safeAnalyze(ctx context.Context, j scanJob, matched map[string]*analyzer.FileInfo) (r jobResult) {
	r.file = j.info.Path
	r.name = j.name
	defer func() {
		if p := recover(); p != nil {
			r.err = fmt.Errorf("internal_error: analyzer panic: %v", p)
		}
	}()
	if r.err = ctx.Err(); r.err != nil {
		return
	}
	r.pkgs, r.err = j.info.Analyzer.Analyze(analyzer.AnalyzeFileInfo{Self: j.info, MatchedFileInfos: matched})
	return
}
func errorCode(e error) string {
	return scanerr.CodeOf(e)
}
func ecosystem(name string) string {
	switch {
	case strings.HasPrefix(name, "go-"):
		return "golang"
	case name == "pom-lang" || name == "jar-lang" || name == "gradle-lang":
		return "maven"
	case strings.HasPrefix(name, "python-"):
		return "pypi"
	case strings.HasPrefix(name, "ruby-"):
		return "gem"
	case name == "npm-lang" || name == "npmp-lang" || name == "yarm-lang":
		return "npm"
	}
	return strings.TrimSuffix(strings.TrimSuffix(name, "-lang"), "-pkg")
}
