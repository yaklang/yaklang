// Package sfpattern implements non-IR pattern matching for SyntaxFlow.
//
// OpFileFilterReg in sfvm delegates here when the feed root is a PatternRoot.
// Results are sfvm.SimpleValue hits — no ssaapi / Program dependency.
package sfpattern

import (
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/minirehs"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	regexp_utils "github.com/yaklang/yaklang/common/utils/regexp-utils"
)

func init() {
	// Default matcher for PatternRoot created via NewRoot.
	defaultMatcher = MatchFileFilter
}

var defaultMatcher sfvm.FileFilterFunc

// MatchFileFilter is the sfvm.FileFilterFunc implementation for regexp (and
// rejects unsupported match types so xpath/json stay on other backends).
func MatchFileFilter(root *sfvm.PatternRoot, files map[string]string, pathPattern, matchType string, paramMap map[string]string, patterns []string) (sfvm.Values, error) {
	// pattern_regex_not: first pattern is positive, remaining are negative
	// (Semgrep pattern-regex + pattern-not-regex combined in one call).
	if paramMap != nil && paramMap["__sf_pattern_not_list"] == "1" {
		if len(patterns) < 2 {
			return nil, utils.Error("sfpattern: pattern_regex_not requires at least one positive and one negative pattern")
		}
		return matchRegexpWithNegatives(root, files, pathPattern, patterns[0], patterns[1:])
	}
	switch strings.ToLower(matchType) {
	case "regexp", "re", "pattern_regex", "pattern-regex", "":
		return matchRegexp(root, files, pathPattern, patterns)
	default:
		return nil, utils.Errorf("sfpattern: unsupported match type %q (use regexp)", matchType)
	}
}

// MatchRegexpWithNegatives finds hits of the positive pattern and removes any
// hit whose range overlaps a match of a negative pattern in the same file.
// This mirrors Semgrep's pattern-regex + pattern-not-regex semantics.
func MatchRegexpWithNegatives(files map[string]string, pathPattern string, positive string, negatives []string) (sfvm.Values, error) {
	root := sfvm.NewPatternRoot(files)
	return matchRegexpWithNegatives(root, files, pathPattern, positive, negatives)
}

func matchRegexpWithNegatives(root *sfvm.PatternRoot, files map[string]string, pathPattern string, positive string, negatives []string) (sfvm.Values, error) {
	hits, err := MatchRegexpHits(files, pathPattern, []string{positive})
	if err != nil {
		return nil, err
	}
	if len(negatives) == 0 {
		root.SetSourceHits(sourceHitKey(pathPattern, []string{positive}), hits)
		return sourceHitsToValues(root, hits)
	}

	negativeMatcher := compileContentMatchers(negatives)

	byFile := make(map[string][]Hit)
	for _, h := range hits {
		byFile[h.Path] = append(byFile[h.Path], h)
	}

	var kept []Hit
	for filePath, fileHits := range byFile {
		content := files[filePath]
		negRanges := negativeMatcher(content)
		for _, h := range fileHits {
			overlap := false
			for _, nr := range negRanges {
				if h.Start < nr.End && nr.Start < h.End {
					overlap = true
					break
				}
			}
			if !overlap {
				kept = append(kept, h)
			}
		}
	}
	if len(kept) == 0 {
		return nil, utils.Errorf("no file contains data matching rule %v (after negative filter)", []string{positive})
	}
	root.SetSourceHits(sourceHitKey(pathPattern, append([]string{positive}, negatives...)), kept)
	return sourceHitsToValues(root, kept)
}

// NewRoot builds a PatternRoot with files and registers the regexp matcher.
func NewRoot(files map[string]string) *sfvm.PatternRoot {
	root := sfvm.NewPatternRoot(files)
	root.SetFileFilterMatcher(MatchFileFilter)
	return root
}

// NewRootFromFS loads all files from fsys into a PatternRoot.
func NewRootFromFS(fsys fi.FileSystem) (*sfvm.PatternRoot, error) {
	files, err := LoadFilesFromFS(fsys)
	if err != nil {
		return nil, err
	}
	return NewRoot(files), nil
}

// Hit is a raw match before wrapping as SimpleValue.
type Hit = sfvm.SourceHit

// MatchRegexp scans files whose path matches pathPattern with content regexes.
// defaultSourceHitBatchSize bounds the number of raw regex hits materialized as
// SFVM values for one execution. Source rules can match tens of thousands of
// locations in large repositories; materializing every hit at once builds
// millions of values/goroutines and exhausts memory before a customer sees any
// result. QuerySyntaxflow repeats the rule once per bounded window.
const DefaultSourceHitBatchSize = 512

func MatchRegexp(files map[string]string, pathPattern string, patterns []string) (sfvm.Values, error) {
	return matchRegexp(sfvm.NewPatternRoot(files), files, pathPattern, patterns)
}

func matchRegexp(root *sfvm.PatternRoot, files map[string]string, pathPattern string, patterns []string) (sfvm.Values, error) {
	key := sourceHitKey(pathPattern, patterns)
	hits, cached := root.SourceHits(key)
	if !cached {
		var err error
		hits, err = MatchRegexpHits(files, pathPattern, patterns)
		if err != nil {
			return nil, err
		}
		root.SetSourceHits(key, hits)
	}
	return sourceHitsToValues(root, hits)
}

func sourceHitsToValues(root *sfvm.PatternRoot, hits []Hit) (sfvm.Values, error) {
	offset, limit, _ := root.SourceHitBatch()
	if limit <= 0 {
		offset = 0
		limit = len(hits)
	}
	if offset >= len(hits) {
		return sfvm.NewEmptyValues(), nil
	}
	end := offset + limit
	if end > len(hits) {
		end = len(hits)
	}
	window := hits[offset:end]
	vals := make([]sfvm.ValueOperator, 0, len(window))
	for _, h := range window {
		sv := sfvm.NewSimpleValueWithEditor(
			h.Text,
			h.Path,
			h.Start,
			h.End,
			root.SourceHitEditor(h.Path),
		)
		sv.SetFiles(root.Files())
		vals = append(vals, sv)
	}
	return sfvm.NewValues(vals), nil
}

func sourceHitKey(pathPattern string, patterns []string) string {
	return pathPattern + "\x00" + strings.Join(patterns, "\x00")
}

// MatchRegexpHits returns raw hits (for tests / callers that skip ValueOperator).
func MatchRegexpHits(files map[string]string, pathPattern string, patterns []string) ([]Hit, error) {
	if pathPattern == "" {
		return nil, utils.Error("sfpattern: empty path pattern")
	}
	matchFile := compilePathMatcher(pathPattern)
	matchContent := compileContentMatchers(patterns)
	if matchContent == nil {
		return nil, utils.Error("sfpattern: no content patterns")
	}

	var hits []Hit
	matchedFile := false
	for filePath, content := range files {
		if !matchFile(filePath) {
			continue
		}
		matchedFile = true
		for _, idx := range matchContent(content) {
			if idx.Start < 0 || idx.End > len(content) || idx.End < idx.Start {
				continue
			}
			hits = append(hits, Hit{
				Path:  filePath,
				Text:  content[idx.Start:idx.End],
				Start: idx.Start,
				End:   idx.End,
			})
		}
	}
	if len(hits) == 0 {
		if matchedFile {
			return nil, utils.Errorf("no file contains data matching rule %v", patterns)
		}
		return nil, utils.Errorf("no file matched by path %s", pathPattern)
	}
	return hits, nil
}

type byteIndex struct{ Start, End int }

func compilePathMatcher(file string) func(string) bool {
	var matchers []func(string) bool
	matchers = append(matchers, func(s string) bool { return s == file })
	if file == "*" || file == "*.*" {
		matchers = append(matchers, func(string) bool { return true })
	}
	if strings.Contains(file, "*") {
		if g, err := glob.Compile(file); err == nil {
			matchers = append(matchers, func(s string) bool {
				base := path.Base(filepath.ToSlash(s))
				return g.Match(s) || g.Match(base) || g.Match(filepath.ToSlash(s))
			})
		}
	}
	if re, err := regexp.Compile(file); err == nil {
		matchers = append(matchers, func(s string) bool {
			return re.MatchString(s) || re.MatchString(path.Base(s))
		})
	}
	if g, err := glob.Compile(file); err == nil {
		matchers = append(matchers, func(s string) bool {
			return g.Match(s) || g.Match(path.Base(filepath.ToSlash(s)))
		})
	}
	return func(s string) bool {
		for _, f := range matchers {
			if f(s) {
				return true
			}
		}
		return false
	}
}

func compileContentMatchers(patterns []string) func(string) []byteIndex {
	var cleaned []string
	for _, s := range patterns {
		if strings.TrimSpace(s) != "" {
			cleaned = append(cleaned, s)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	if len(cleaned) >= 2 {
		if fn, ok := tryMinirehsMatcher(cleaned); ok {
			return fn
		}
	}
	var matchers []func(string) []byteIndex
	for _, rule := range cleaned {
		rule := rule
		yak := regexp_utils.NewYakRegexpUtils(rule)
		matchers = append(matchers, func(data string) []byteIndex {
			indexs, err := yak.FindAllSubmatchIndex(data)
			if err != nil {
				log.Warnf("sfpattern regexp match error: %s", err)
				return nil
			}
			out := make([]byteIndex, 0, len(indexs))
			for _, index := range indexs {
				if len(index) >= 2 {
					out = append(out, byteIndex{Start: index[0], End: index[1]})
				}
			}
			return out
		})
	}
	return func(data string) []byteIndex {
		var all []byteIndex
		for _, m := range matchers {
			all = append(all, m(data)...)
		}
		return all
	}
}

func tryMinirehsMatcher(patterns []string) (func(string) []byteIndex, bool) {
	ps := make([]minirehs.Pattern, 0, len(patterns))
	for i, expr := range patterns {
		ps = append(ps, minirehs.Pattern{
			ID:    minirehs.PatternID(i),
			Expr:  expr,
			Flags: minirehs.FlagMultiline,
		})
	}
	db, err := minirehs.Compile(ps)
	if err != nil {
		log.Debugf("sfpattern: minirehs compile fallback: %v", err)
		return nil, false
	}
	return func(data string) []byteIndex {
		var out []byteIndex
		sc, err := db.NewScratch()
		if err != nil {
			return nil
		}
		defer sc.Close()
		_ = db.Scan([]byte(data), sc, func(m minirehs.Match) bool {
			if m.From >= 0 && m.To >= m.From {
				out = append(out, byteIndex{Start: m.From, End: m.To})
			}
			return true
		})
		return out
	}, true
}

// LoadFilesFromFS recursively reads text files into path→content.
// No extension filter — used by embedded verify FS and already-scoped callers.
func LoadFilesFromFS(fsys fi.FileSystem) (map[string]string, error) {
	if fsys == nil {
		return nil, utils.Error("nil filesystem")
	}
	out := make(map[string]string)
	err := filesys.Recursive(".", filesys.WithFileSystem(fsys), filesys.WithFileStat(func(name string, info fs.FileInfo) error {
		if info.IsDir() {
			return nil
		}
		if info.Size() > 8<<20 {
			return nil
		}
		raw, err := fsys.ReadFile(name)
		if err != nil {
			return nil
		}
		out[filepath.ToSlash(name)] = string(raw)
		return nil
	}))
	return out, err
}

// LoadOptions controls which files enter a source-mode scan.
type LoadOptions struct {
	// MaxFileBytes skips files larger than this (0 = 2MiB default).
	MaxFileBytes int64
	// SkipDirNames are directory base names to prune (node_modules, .git, …).
	SkipDirNames map[string]struct{}
	// ExtAllow is an optional allow-list of lowercase extensions (".java").
	// Empty means allow common source/config text extensions.
	ExtAllow map[string]struct{}
}

// DefaultLoadOptions skips heavy VCS/build trees and caps file size.
func DefaultLoadOptions() LoadOptions {
	skip := map[string]struct{}{}
	for _, d := range []string{
		".git", ".svn", ".hg", "node_modules", "vendor", "dist", "build",
		"target", ".idea", ".vscode", "__pycache__", ".tox", "coverage",
		"bower_components", ".gradle", ".mvn", "out", "bin", "obj",
	} {
		skip[d] = struct{}{}
	}
	ext := map[string]struct{}{}
	for _, e := range []string{
		".java", ".kt", ".go", ".py", ".js", ".jsx", ".ts", ".tsx", ".vue",
		".php", ".rb", ".rs", ".c", ".cc", ".cpp", ".h", ".hpp", ".cs",
		".xml", ".yml", ".yaml", ".json", ".properties", ".env", ".ini",
		".toml", ".conf", ".cfg", ".txt", ".md", ".gradle", ".sql",
		".sh", ".bash", ".zsh", ".pem", ".key", ".crt", ".jsp", ".asp",
		".aspx", ".html", ".htm", ".css", ".scss", ".tf", ".hcl",
	} {
		ext[e] = struct{}{}
	}
	return LoadOptions{MaxFileBytes: 2 << 20, SkipDirNames: skip, ExtAllow: ext}
}

// LoadFilesFromFSWithOptions is LoadFilesFromFS with skip/size filters.
func LoadFilesFromFSWithOptions(fsys fi.FileSystem, opt LoadOptions) (map[string]string, error) {
	if fsys == nil {
		return nil, utils.Error("nil filesystem")
	}
	if opt.MaxFileBytes <= 0 {
		opt.MaxFileBytes = 2 << 20
	}
	out := make(map[string]string)
	err := filesys.Recursive(".",
		filesys.WithFileSystem(fsys),
		filesys.WithDirStat(func(name string, info fs.FileInfo) error {
			base := path.Base(filepath.ToSlash(name))
			if _, skip := opt.SkipDirNames[base]; skip {
				return filesys.SkipDir
			}
			return nil
		}),
		filesys.WithFileStat(func(name string, info fs.FileInfo) error {
			if info.IsDir() {
				return nil
			}
			if info.Size() > opt.MaxFileBytes {
				return nil
			}
			ext := strings.ToLower(filepath.Ext(name))
			if len(opt.ExtAllow) > 0 {
				if _, ok := opt.ExtAllow[ext]; !ok {
					// also allow extension-less env/config names
					base := strings.ToLower(path.Base(name))
					if !(strings.HasPrefix(base, ".") || strings.Contains(base, "env") ||
						strings.Contains(base, "dockerfile") || base == "makefile") {
						return nil
					}
				}
			}
			raw, err := fsys.ReadFile(name)
			if err != nil {
				return nil
			}
			out[filepath.ToSlash(name)] = string(raw)
			return nil
		}),
	)
	return out, err
}

// LoadFilesFromDir loads a local directory into path→content for source scans.
func LoadFilesFromDir(dir string) (map[string]string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, utils.Error("empty source dir")
	}
	return LoadFilesFromFSWithOptions(filesys.NewRelLocalFs(dir), DefaultLoadOptions())
}
