package sfpattern

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minirehs"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/memedit"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

// SourceScanPlan is an immutable, scan-local index of direct file searches.
// It deliberately does not rewrite SFVM code: a PatternRoot can consult a
// runtime for raw hits while assignments, set operations and alerts run as usual.
// The owner must close runtimes before closing their shared plan.
type SourceScanPlan struct {
	searches map[string]sourceSearch
	patterns []sourcePattern
	db       minirehs.Database
}

type sourcePattern struct {
	expr     string
	flags    minirehs.Flag
	captures bool // legacy single-pattern searches also expose capture ranges
}

type sourceSearch struct {
	path      string
	positive  []int
	negative  []int
	matchPath func(string) bool
}

// SourceScanStats describes the work shared by all rules on one source snapshot.
// Read it after Wait returns. Lookups counts cached searches, including misses.
type SourceScanStats struct {
	Patterns      int
	Searches      int
	Workers       int
	Files         int
	Bytes         int64
	Hits          int
	FallbackFiles int
	Lookups       int64
}

// NewSourceScanPlan collects every named OpFileFilterReg, not just the first
// statement of each rule. Chained searches have no path and are not collected.
// Invalid searches stay on the legacy matcher without poisoning valid patterns.
func NewSourceScanPlan(frames []*sfvm.SFFrame) (*SourceScanPlan, error) {
	p := &SourceScanPlan{searches: make(map[string]sourceSearch)}
	ids := make(map[sourcePattern]int)
	for _, frame := range frames {
		if !sfvm.FrameIsSourceMode(frame) {
			continue
		}
		for _, code := range frame.Codes {
			if code.OpCode != sfvm.OpFileFilterReg || code.UnaryStr == "" {
				continue
			}
			key := directSearchKey(code.UnaryStr, code.FileFilterMethodItem, code.Values)
			if _, ok := p.searches[key]; ok {
				continue
			}
			positive, negative := code.Values, []string(nil)
			if code.FileFilterMethodItem["__sf_pattern_not_list"] == "1" {
				if len(positive) < 2 {
					continue
				}
				positive, negative = positive[:1], positive[1:]
			}
			positive, negative = cleanPatterns(positive), cleanPatterns(negative)
			if len(positive) == 0 || !usablePatterns(positive) || !usablePatterns(negative) {
				continue
			}
			add := func(exprs []string) []int {
				// Preserve compileContentMatchers' existing multi-pattern flags.
				var flags minirehs.Flag
				if len(exprs) >= 2 {
					flags = minirehs.FlagMultiline
				}
				var out []int
				for _, expr := range exprs {
					pattern := sourcePattern{expr: expr, flags: flags}
					if len(exprs) == 1 {
						if re, err := regexp.Compile(expr); err == nil {
							pattern.captures = re.NumSubexp() > 0
						} else {
							pattern.captures = true // PCRE2 uses the same legacy locator
						}
					}
					id, ok := ids[pattern]
					if !ok {
						id = len(p.patterns)
						ids[pattern] = id
						p.patterns = append(p.patterns, pattern)
					}
					out = append(out, id)
				}
				return out
			}
			p.searches[key] = sourceSearch{
				path: code.UnaryStr, positive: add(positive), negative: add(negative),
				matchPath: compilePathMatcher(code.UnaryStr),
			}
		}
	}
	if len(p.patterns) == 0 {
		return p, nil
	}
	patterns := make([]minirehs.Pattern, len(p.patterns))
	for id, pattern := range p.patterns {
		patterns[id] = minirehs.Pattern{ID: minirehs.PatternID(id), Expr: pattern.expr, Flags: pattern.flags}
	}
	// Engine retains FindAll's leftmost-first, non-overlapping byte offsets.
	// MVS location semantics differ; existence-only matches need a locator.
	var err error
	p.db, err = minirehs.Compile(patterns, minirehs.WithBackend(minirehs.BackendEngine))
	if err != nil {
		return nil, fmt.Errorf("compile source pre-scan: %w", err)
	}
	return p, nil
}

func cleanPatterns(patterns []string) []string {
	var out []string
	for _, expr := range patterns {
		if strings.TrimSpace(expr) != "" {
			out = append(out, expr)
		}
	}
	return out
}

func usablePatterns(patterns []string) bool {
	for _, expr := range patterns {
		// The legacy multi-pattern helper omits PCRE2 existence-only entries.
		// Keep those uncommon calls on that path rather than changing their
		// capture/negative-filter semantics as part of a performance change.
		if len(patterns) >= 2 {
			if _, err := regexp.Compile(expr); err != nil {
				return false
			}
		}
		if !regexp_utils.NewYakRegexpUtils(expr).CanUse() {
			return false
		}
	}
	return true
}

func directSearchKey(path string, params map[string]string, patterns []string) string {
	// Length-safe encoding: patterns can themselves contain NULs. Negative
	// and ordinary calls with identical arguments must not share a result.
	key, _ := json.Marshal(struct {
		Path     string
		Patterns []string
		NotList  bool
	}{path, patterns, params["__sf_pattern_not_list"] == "1"})
	return string(key)
}

func (p *SourceScanPlan) Close() error {
	if p != nil && p.db != nil {
		return p.db.Close()
	}
	return nil
}

type storedSpan struct {
	Start, End int
	Text       string
}

// SharedBodySource is optional. A store that already owns an immutable body
// (a compiled program editor, for example) lets the runtime reuse it.
type SharedBodySource interface {
	SharedBody(path string) (*memedit.SafeString, bool)
}

// SourceScanRuntime precomputes raw offsets for one immutable files snapshot.
// It publishes only complete results by closing done. SFVM roots, values,
// editors and batch cursors remain query-local, never shared between rules.
// The first read of a path, including an empty body or a read error, is kept
// for this scan only. Direct search, fallback, chained search and editors all
// use that snapshot.
type SourceScanRuntime struct {
	plan    *SourceScanPlan
	store   SourceStore
	done    chan struct{}
	cancel  context.CancelFunc
	err     error
	matches []map[string][]storedSpan
	stats   SourceScanStats
	lookups atomic.Int64
	mu      sync.Mutex
	close   sync.Once

	bodyMu sync.Mutex
	bodies map[string]*sourceBody
}

// sourceBody is the one snapshot of a path. once publishes text, raw and err
// together; later readers wait for that publication.
type sourceBody struct {
	once sync.Once
	text string
	raw  *memedit.SafeString
	err  error
}

// Start begins the shared scan independently of any individual rule's context.
// Closing the runtime cancels and joins its worker; no detached scan survives.
// The map adapter keeps existing callers working. Live scans use StartStore.
func (p *SourceScanPlan) Start(ctx context.Context, files map[string]string) *SourceScanRuntime {
	return p.StartStore(ctx, MapSource(files))
}

// StartStore scans store.Paths() and reads each body through store.Read.
func (p *SourceScanPlan) StartStore(ctx context.Context, store SourceStore) *SourceScanRuntime {
	ctx, cancel := context.WithCancel(ctx)
	r := &SourceScanRuntime{
		plan: p, store: store, done: make(chan struct{}), cancel: cancel,
		matches: make([]map[string][]storedSpan, len(p.patterns)),
		stats:   SourceScanStats{Patterns: len(p.patterns), Searches: len(p.searches)},
	}
	go func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				r.err = fmt.Errorf("source pre-scan panic: %v", recovered)
			}
			close(r.done)
		}()
		r.err = r.scan(ctx)
	}()
	return r
}

func (r *SourceScanRuntime) Wait(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-r.done:
		return r.err
	}
}

func (r *SourceScanRuntime) Close() {
	if r == nil {
		return
	}
	r.close.Do(func() {
		r.cancel()
		<-r.done
	})
}

func (r *SourceScanRuntime) Stats() SourceScanStats {
	<-r.done
	stats := r.stats
	stats.Lookups = r.lookups.Load()
	return stats
}

func (r *SourceScanRuntime) scan(ctx context.Context) error {
	if r.plan.db == nil {
		return nil
	}
	locators := make([]*regexp_utils.YakRegexpUtils, len(r.plan.patterns))
	for id, pattern := range r.plan.patterns {
		expr := pattern.expr
		if pattern.flags&minirehs.FlagMultiline != 0 {
			expr = "(?m)" + expr
		}
		locators[id] = regexp_utils.NewYakRegexpUtils(expr)
	}
	if r.store == nil {
		return nil
	}
	paths := append([]string(nil), r.store.Paths()...)
	sort.Strings(paths)
	type scanJob struct {
		path    string
		allowed []bool
	}
	var jobs []scanJob
	for _, path := range paths {
		allowed := make([]bool, len(r.plan.patterns))
		eligible := false
		for _, search := range r.plan.searches {
			if search.matchPath(path) {
				eligible = true
				for _, id := range search.positive {
					allowed[id] = true
				}
				for _, id := range search.negative {
					allowed[id] = true
				}
			}
		}
		if eligible {
			jobs = append(jobs, scanJob{path, allowed})
		}
	}
	if len(jobs) == 0 {
		return nil
	}
	// The compiled database is immutable. Scratch is not, so each worker owns one.
	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(jobs) {
		workers = len(jobs)
	}
	r.stats.Workers = workers
	log.Infof("source pre-scan workers=%d files=%d", workers, len(jobs))

	jobCh := make(chan scanJob)
	var wg sync.WaitGroup
	var scanErr error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		r.mu.Lock()
		if scanErr == nil {
			scanErr = err
		}
		r.mu.Unlock()
	}
	failed := func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return scanErr != nil
	}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					recordErr(fmt.Errorf("source pre-scan panic: %v", recovered))
				}
			}()
			scratch, err := r.plan.db.NewScratch()
			if err != nil {
				recordErr(err)
				for range jobCh {
				}
				return
			}
			defer scratch.Close()
			for job := range jobCh {
				if ctx.Err() != nil || failed() {
					continue
				}
				func() {
					content, _, err := r.body(job.path)
					if err != nil {
						log.Warnf("source pre-scan: skip %s: %v", job.path, err)
						return
					}
					hits, fallback, err := r.scanFile(ctx, scratch, locators, job.path, content, job.allowed, failed)
					if err != nil {
						recordErr(err)
						return
					}
					if ctx.Err() != nil || failed() {
						return
					}
					r.publish(job.path, len(content), hits, fallback)
				}()
			}
		}()
	}
	sendErr := func() error {
		defer close(jobCh)
		for _, job := range jobs {
			if failed() {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case jobCh <- job:
			}
		}
		return nil
	}()
	wg.Wait()
	if sendErr != nil {
		return sendErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return scanErr
}

func (r *SourceScanRuntime) scanFile(ctx context.Context, scratch minirehs.Scratch, locators []*regexp_utils.YakRegexpUtils, path, content string, allowed []bool, stop func() bool) ([][]storedSpan, bool, error) {
	fileHits := make([][]storedSpan, len(r.plan.patterns))
	located := make([]bool, len(r.plan.patterns))
	var locateErr error
	// Skip patterns whose path glob cannot match this file. The engine would
	// otherwise rescan the whole record for every triggered pattern.
	minirehs.RestrictScratch(scratch, allowed)
	err := r.plan.db.Scan([]byte(content), scratch, func(m minirehs.Match) bool {
		if ctx.Err() != nil || stop() {
			return false
		}
		id := int(m.ID)
		if !allowed[id] {
			return true
		}
		if m.From < 0 || r.plan.patterns[id].captures {
			// PCRE2 needs exact location; single-pattern legacy searches also
			// report captures. Locate once per candidate, not once per match.
			if !located[id] {
				located[id] = true
				var locatedHits []byteIndex
				locatedHits, locateErr = locateContent(locators[id], content, r.plan.patterns[id].captures)
				fileHits[id] = ownSpans(content, locatedHits)
			}
			return locateErr == nil
		}
		fileHits[id] = append(fileHits[id], ownSpan(content, m.From, m.To))
		return true
	})
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if stop() {
		return nil, false, nil
	}
	if err != nil || locateErr != nil {
		// The batch may have stopped after a prefix. Discard it and use the
		// original locator for this entire file, so unrelated rules don't lose
		// findings because one PCRE2 expression hit an execution limit.
		log.Warnf("source pre-scan: file %s uses per-pattern fallback: scan=%v locate=%v", path, err, locateErr)
		for id, locator := range locators {
			fileHits[id] = nil
			if !allowed[id] {
				continue
			}
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			var locatedHits []byteIndex
			locatedHits, err = locateContent(locator, content, r.plan.patterns[id].captures)
			fileHits[id] = ownSpans(content, locatedHits)
			if err != nil {
				return nil, false, fmt.Errorf("source pre-scan %s pattern %d: %w", path, id, err)
			}
		}
		return fileHits, true, nil
	}
	return fileHits, false, nil
}

func ownSpan(content string, start, end int) storedSpan {
	if start < 0 || end < start || end > len(content) {
		return storedSpan{Start: start, End: end}
	}
	return storedSpan{Start: start, End: end, Text: detachString(content[start:end])}
}

func ownSpans(content string, spans []byteIndex) []storedSpan {
	out := make([]storedSpan, len(spans))
	for i, span := range spans {
		out[i] = ownSpan(content, span.Start, span.End)
	}
	return out
}

func (r *SourceScanRuntime) publish(path string, size int, fileHits [][]storedSpan, fallback bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stats.Files++
	r.stats.Bytes += int64(size)
	if fallback {
		r.stats.FallbackFiles++
	}
	for id, hits := range fileHits {
		if len(hits) == 0 {
			continue
		}
		if r.matches[id] == nil {
			r.matches[id] = make(map[string][]storedSpan)
		}
		r.matches[id][path] = hits
		r.stats.Hits += len(hits)
	}
}

func locateContent(locator *regexp_utils.YakRegexpUtils, content string, captures bool) ([]byteIndex, error) {
	var indexes [][]int
	var err error
	if captures {
		indexes, err = locator.FindAllSubmatchIndex(content)
	} else {
		indexes, err = locator.FindAllMatchIndex(content)
	}
	if err != nil {
		return nil, err
	}
	var hits []byteIndex
	for _, index := range indexes {
		if len(index) >= 2 && index[0] >= 0 && index[1] >= index[0] && index[1] <= len(content) {
			hits = append(hits, byteIndex{index[0], index[1]})
		}
	}
	return hits, nil
}

// body returns the scan's one snapshot of path. The first caller reads the
// store; everyone else, including a later rule, waits for that result.
func (r *SourceScanRuntime) body(path string) (string, *memedit.SafeString, error) {
	if r == nil || r.store == nil {
		return "", nil, fmt.Errorf("source body %q is not available", path)
	}
	r.bodyMu.Lock()
	if r.bodies == nil {
		r.bodies = make(map[string]*sourceBody)
	}
	cached := r.bodies[path]
	if cached == nil {
		cached = &sourceBody{}
		r.bodies[path] = cached
	}
	r.bodyMu.Unlock()
	cached.once.Do(func() {
		if shared, ok := r.store.(SharedBodySource); ok {
			if raw, ok := shared.SharedBody(path); ok && raw != nil {
				cached.raw = raw
				cached.text = raw.String()
				return
			}
		}
		content, release, err := r.store.Read(path)
		if release != nil {
			defer release()
		}
		if err != nil {
			cached.err = err
			return
		}
		cached.raw = memedit.NewSafeString(content)
		cached.text = cached.raw.String()
	})
	return cached.text, cached.raw, cached.err
}

func sharedScanFiles(root *sfvm.PatternRoot, files map[string]string) map[string]string {
	if root != nil && root.UsesSharedBody() {
		return nil
	}
	return files
}

func (r *SourceScanRuntime) bind(root *sfvm.PatternRoot) {
	if r == nil || root == nil {
		return
	}
	root.SetFileReader(func(path string) (string, error) {
		text, _, err := r.body(path)
		return text, err
	})
	root.SetSharedSource(func(path string) (*memedit.SafeString, error) {
		_, raw, err := r.body(path)
		return raw, err
	})
}

// Matcher decorates only PatternRoot's direct searches. Unknown requests still
// use the existing matcher, reading the same cached body. A completed empty
// result is handled, not a miss. Cancellation stops this caller's wait, not
// the shared scan.
func (r *SourceScanRuntime) Matcher(ctx context.Context) sfvm.FileFilterFunc {
	return func(root *sfvm.PatternRoot, files map[string]string, path, matchType string, params map[string]string, patterns []string) (sfvm.Values, error) {
		r.bind(root)
		key := directSearchKey(path, params, patterns)
		search, ok := r.plan.searches[key]
		matchType = strings.ToLower(matchType)
		if !ok || (matchType != "regexp" && matchType != "re" && matchType != "pattern_regex" && matchType != "pattern-regex" && matchType != "") {
			return MatchFileFilter(root, sharedScanFiles(root, files), path, matchType, params, patterns)
		}
		if err := r.Wait(ctx); err != nil {
			return nil, err
		}
		r.lookups.Add(1)
		cacheKey := "source-runtime:" + key
		hits, cached := root.SourceHits(cacheKey)
		if !cached {
			for _, id := range search.positive {
				for path, spans := range r.matches[id] {
					if !search.matchPath(path) {
						continue
					}
					for _, span := range spans {
						overlaps := false
						for _, negID := range search.negative {
							for _, negative := range r.matches[negID][path] {
								if span.Start < negative.End && negative.Start < span.End {
									overlaps = true
									break
								}
							}
							if overlaps {
								break
							}
						}
						if !overlaps {
							hits = append(hits, Hit{Path: path, Text: span.Text, Start: span.Start, End: span.End})
						}
					}
				}
			}
			// Stable windows are important when different rules reuse one pattern.
			sort.SliceStable(hits, func(i, j int) bool {
				if hits[i].Path != hits[j].Path {
					return hits[i].Path < hits[j].Path
				}
				if hits[i].Start != hits[j].Start {
					return hits[i].Start < hits[j].Start
				}
				return hits[i].End < hits[j].End
			})
			root.SetSourceHits(cacheKey, hits)
		}
		if len(hits) == 0 {
			matchedFile := false
			for _, path := range r.store.Paths() {
				if search.matchPath(path) {
					matchedFile = true
					break
				}
			}
			if !matchedFile {
				return nil, fmt.Errorf("no file matched by path %s", search.path)
			}
			return nil, fmt.Errorf("no file contains data matching rule %v", patterns)
		}
		return sourceHitsToValues(root, hits)
	}
}
