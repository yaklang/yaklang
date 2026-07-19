package loop_fast_context

import (
	"bufio"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	loopVarFileIndex          = "fastcontext_file_index"
	grepFilesWithMatchesLimit = 200
	findFileMaxResults        = 200
	grepBatchConcurrency      = 10
	loopVarGrepBatchSearches  = "fastcontext_grep_batch_searches"
)

// Allow optional log prefixes (e.g. "[info]   ") before "[file N]".
var grepFileLinePattern = regexp.MustCompile(`\[file\s+\d+\]\s+(.+?)\s+\(\d+\s+matches\)\s*$`)

// toolOutputString returns the bounded ToolResult.Data preview. Full output is
// intentionally available only through the artifact paths embedded in it.
func toolOutputString(data any) string {
	if data == nil {
		return ""
	}
	return utils.InterfaceToString(data)
}

func mergePathsIntoFileIndex(loop interface {
	Get(string) string
	Set(string, any)
}, paths ...string) (added int) {
	index := loadFileIndex(loop.Get(loopVarFileIndex))
	before := len(index)
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil && abs != "" {
			p = abs
		}
		index[p] = struct{}{}
	}
	added = len(index) - before
	if added < 0 {
		added = 0
	}
	saveFileIndex(loop, index)
	return added
}

func loadFileIndex(raw string) map[string]struct{} {
	index := make(map[string]struct{})
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			index[line] = struct{}{}
		}
	}
	return index
}

func saveFileIndex(loop interface{ Set(string, any) }, index map[string]struct{}) {
	paths := make([]string, 0, len(index))
	for p := range index {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	loop.Set(loopVarFileIndex, strings.Join(paths, "\n"))
}

func listFileIndex(loop interface{ Get(string) string }) []string {
	raw := strings.TrimSpace(loop.Get(loopVarFileIndex))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func parseGrepFilesWithMatchesOutput(content string) []string {
	var paths []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if m := grepFileLinePattern.FindStringSubmatch(line); len(m) == 2 {
			paths = append(paths, strings.TrimSpace(m[1]))
			continue
		}
		if strings.Contains(line, "===") {
			continue
		}
		candidate := line
		if idx := strings.Index(line, " ("); idx > 0 {
			candidate = line[:idx]
		}
		candidate = strings.TrimSpace(candidate)
		// Unix (/...) and Windows (C:\...) absolute paths.
		if filepath.IsAbs(candidate) || strings.HasPrefix(candidate, "/") {
			paths = append(paths, candidate)
		}
	}
	return paths
}

func parseFindFileOutput(content string) []string {
	var paths []string
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "===") || strings.HasPrefix(line, "...") {
			continue
		}
		if strings.HasPrefix(line, "/") || strings.Contains(line, string(filepath.Separator)) {
			paths = append(paths, line)
		}
	}
	return paths
}

func compactSearchFeedback(toolName string, added, total int, sample []string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[%s] +%d new paths, total %d unique files in index.", toolName, added, total))
	if len(sample) > 0 {
		b.WriteString("\nSample:\n")
		for _, p := range sample {
			b.WriteString("  - ")
			b.WriteString(p)
			b.WriteByte('\n')
		}
	}
	b.WriteString("\n(Stdout is not kept in parent context — only deduplicated paths are delivered.)")
	return b.String()
}

func samplePaths(paths []string, n int) []string {
	if len(paths) <= n {
		return paths
	}
	return paths[:n]
}

func locationsFromFileIndex(loop interface{ Get(string) string }) []LocationHit {
	paths := listFileIndex(loop)
	out := make([]LocationHit, 0, len(paths))
	for _, p := range paths {
		out = append(out, LocationHit{Path: p, Reason: "files_with_matches index"})
	}
	return out
}
