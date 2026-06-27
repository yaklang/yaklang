package loop_ssa_api_discovery

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const defaultSearchMaxResults = 50

type fileSearchOpts struct {
	Glob         string
	Suffix       string
	NameContains string
	MaxResults   int
}

func searchFilesUnderCodeRoot(codeRoot string, opts fileSearchOpts) ([]string, error) {
	codeRoot = filepath.Clean(strings.TrimSpace(codeRoot))
	if codeRoot == "" {
		return nil, utils.Error("code root empty")
	}
	max := opts.MaxResults
	if max <= 0 {
		max = defaultSearchMaxResults
	}
	globPat := strings.TrimSpace(opts.Glob)
	suffix := strings.ToLower(strings.TrimSpace(opts.Suffix))
	nameContains := strings.ToLower(strings.TrimSpace(opts.NameContains))

	var out []string
	err := filepath.Walk(codeRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			base := strings.ToLower(info.Name())
			if skipDirForHarvest(base) || skipDirForRouteScan(base) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(out) >= max {
			return filepath.SkipAll
		}
		rel, err := filepath.Rel(codeRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		base := strings.ToLower(filepath.Base(rel))
		if suffix != "" && !strings.HasSuffix(strings.ToLower(rel), suffix) {
			return nil
		}
		if nameContains != "" && !strings.Contains(base, nameContains) {
			return nil
		}
		if globPat != "" && !fileMatchesSimpleGlob(rel, globPat) {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	return out, err
}

func fileMatchesSimpleGlob(rel, pattern string) bool {
	pattern = filepath.ToSlash(strings.TrimSpace(pattern))
	rel = filepath.ToSlash(rel)
	if pattern == "" {
		return true
	}
	if strings.Contains(pattern, "**") {
		parts := strings.Split(pattern, "**")
		if len(parts) == 2 {
			prefix := strings.Trim(strings.TrimSpace(parts[0]), "/")
			suffix := strings.Trim(strings.TrimSpace(parts[1]), "/")
			if prefix != "" && !strings.HasPrefix(rel, prefix) {
				return false
			}
			if suffix == "" {
				return true
			}
			return strings.Contains(rel, suffix) || strings.HasSuffix(rel, suffix)
		}
	}
	if matched, _ := filepath.Match(pattern, rel); matched {
		return true
	}
	if matched, _ := filepath.Match(pattern, filepath.Base(rel)); matched {
		return true
	}
	return strings.Contains(rel, strings.Trim(pattern, "*"))
}

type grepMatch struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Text    string `json:"text"`
}

func grepFilesUnderCodeRoot(codeRoot, pattern, glob string, maxMatches int) ([]grepMatch, error) {
	codeRoot = filepath.Clean(strings.TrimSpace(codeRoot))
	if codeRoot == "" {
		return nil, utils.Error("code root empty")
	}
	pat := strings.TrimSpace(pattern)
	if pat == "" {
		return nil, utils.Error("pattern required")
	}
	re, err := regexp.Compile(pat)
	if err != nil {
		return nil, utils.Wrap(err, "invalid pattern")
	}
	if maxMatches <= 0 {
		maxMatches = 100
	}
	var matches []grepMatch
	_ = filepath.Walk(codeRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || len(matches) >= maxMatches {
			return nil
		}
		if info.IsDir() {
			if skipDirForHarvest(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(codeRoot, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if glob != "" && !fileMatchesSimpleGlob(rel, glob) {
			return nil
		}
		if !isTextFileForGrep(rel) {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for sc.Scan() {
			lineNo++
			if len(matches) >= maxMatches {
				break
			}
			line := sc.Text()
			if re.MatchString(line) {
				matches = append(matches, grepMatch{
					File: rel,
					Line: lineNo,
					Text: strings.TrimSpace(line),
				})
			}
		}
		return nil
	})
	return matches, nil
}

func isTextFileForGrep(rel string) bool {
	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".java", ".go", ".py", ".php", ".kt", ".xml", ".yml", ".yaml", ".json", ".properties", ".gradle", ".toml", ".mod", ".html", ".ftl", ".jsp", ".js", ".ts", ".vue":
		return true
	default:
		return ext == "" && !strings.Contains(filepath.Base(rel), ".")
	}
}

func readRepoRelativeFile(rt *Runtime, rel string) ([]byte, string, error) {
	if rt == nil || rt.Session == nil || !rt.Session.CodePathOK {
		return nil, "", utils.Error("code path not available")
	}
	canon := normalizePlanFileRef(rt, rel)
	if canon == "" {
		return nil, "", utils.Error("file path empty")
	}
	abs := filepath.Join(rt.Session.CodeRootPath, filepath.FromSlash(canon))
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, canon, err
	}
	return b, canon, nil
}
