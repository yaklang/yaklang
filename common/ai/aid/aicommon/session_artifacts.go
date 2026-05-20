package aicommon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

type SessionArtifactEntry struct {
	RelPath string
	Size    int64
	ModUnix int64
}

type SessionArtifactTaskGroup struct {
	TaskDir   string
	Files     []SessionArtifactEntry
	LatestMod int64
}

type SessionArtifactsPromptBlocks struct {
	Frozen string
	Open   string
}

func CollectSessionArtifactEntries(config AICallerConfigIf) ([]SessionArtifactEntry, string) {
	if config == nil {
		return nil, ""
	}
	workDir := config.GetOrCreateWorkDir()
	if workDir == "" {
		return nil, ""
	}
	info, err := os.Stat(workDir)
	if err != nil || !info.IsDir() {
		return nil, workDir
	}

	var entries []SessionArtifactEntry
	walkErr := filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			log.Warnf("[SessionArtifacts] walk error for %s: %v", path, err)
			return nil
		}
		if d == nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			log.Warnf("[SessionArtifacts] stat error for %s: %v", path, err)
			return nil
		}
		relPath, err := filepath.Rel(workDir, path)
		if err != nil {
			relPath = path
		}
		entries = append(entries, SessionArtifactEntry{
			RelPath: filepath.ToSlash(relPath),
			Size:    info.Size(),
			ModUnix: info.ModTime().Unix(),
		})
		return nil
	})
	if walkErr != nil {
		log.Warnf("session artifacts: walk error: %v", walkErr)
	}
	return entries, workDir
}

func GroupSessionArtifactsByTask(entries []SessionArtifactEntry) []SessionArtifactTaskGroup {
	if len(entries) == 0 {
		return nil
	}
	groupByTask := make(map[string][]SessionArtifactEntry)
	var rootFiles []SessionArtifactEntry
	for _, entry := range entries {
		entry.RelPath = filepath.ToSlash(strings.TrimSpace(entry.RelPath))
		if entry.RelPath == "" {
			continue
		}
		taskDir, ok := sessionArtifactTaskDir(entry.RelPath)
		if !ok {
			rootFiles = append(rootFiles, entry)
			continue
		}
		groupByTask[taskDir] = append(groupByTask[taskDir], entry)
	}

	groups := make([]SessionArtifactTaskGroup, 0, len(groupByTask)+1)
	for taskDir, files := range groupByTask {
		groups = append(groups, buildSessionArtifactTaskGroup(taskDir, files))
	}
	sort.SliceStable(groups, func(i, j int) bool {
		return compareSessionArtifactTaskDir(groups[i].TaskDir, groups[j].TaskDir) < 0
	})
	if len(rootFiles) > 0 {
		groups = append(groups, buildSessionArtifactTaskGroup("", rootFiles))
	}
	return groups
}

func SplitSessionArtifactGroups(groups []SessionArtifactTaskGroup) (frozen []SessionArtifactTaskGroup, open []SessionArtifactTaskGroup) {
	if len(groups) == 0 {
		return nil, nil
	}
	var taskGroups []SessionArtifactTaskGroup
	var rootGroups []SessionArtifactTaskGroup
	for _, group := range groups {
		if strings.TrimSpace(group.TaskDir) == "" {
			rootGroups = append(rootGroups, group)
			continue
		}
		taskGroups = append(taskGroups, group)
	}
	if len(taskGroups) > 1 {
		frozen = append(frozen, taskGroups[:len(taskGroups)-1]...)
		open = append(open, taskGroups[len(taskGroups)-1])
	} else if len(taskGroups) == 1 {
		open = append(open, taskGroups[0])
	}
	open = append(open, rootGroups...)
	return frozen, open
}

func RenderSessionArtifactGroups(workDir string, groups []SessionArtifactTaskGroup) string {
	if len(groups) == 0 {
		return ""
	}
	workDir = strings.TrimSpace(workDir)
	totalFiles := 0
	for i := range groups {
		normalizeSessionArtifactTaskGroup(&groups[i])
		totalFiles += len(groups[i].Files)
	}
	if totalFiles == 0 {
		return ""
	}

	var sb strings.Builder
	if workDir != "" {
		sb.WriteString(fmt.Sprintf("artifacts_dir: %s\n", workDir))
	}
	sb.WriteString(fmt.Sprintf("total_files: %d\n\n", totalFiles))
	for _, group := range groups {
		if len(group.Files) == 0 {
			continue
		}
		if strings.TrimSpace(group.TaskDir) == "" {
			sb.WriteString("### [root files]\n")
		} else {
			sb.WriteString(fmt.Sprintf("### %s (modified: %s)\n",
				group.TaskDir,
				formatArtifactModTime(group.LatestMod, "2006-01-02 15:04:05"),
			))
		}
		for _, file := range group.Files {
			displayPath := file.RelPath
			if group.TaskDir != "" {
				displayPath = strings.TrimPrefix(displayPath, group.TaskDir+"/")
			}
			sb.WriteString(fmt.Sprintf("- %s (%s, %s)\n",
				displayPath,
				formatFileSize(file.Size),
				formatArtifactModTime(file.ModUnix, "15:04:05"),
			))
		}
		sb.WriteString("\n")
	}
	result := sb.String()
	if MeasureTokens(result) > ArtifactsContextMaxTokens {
		result = ShrinkTextBlockByTokens(result, ArtifactsContextMaxTokens)
	}
	return result
}

func RenderSessionArtifactsFrozenOpen(config AICallerConfigIf) SessionArtifactsPromptBlocks {
	entries, workDir := CollectSessionArtifactEntries(config)
	groups := GroupSessionArtifactsByTask(entries)
	frozenGroups, openGroups := SplitSessionArtifactGroups(groups)
	return SessionArtifactsPromptBlocks{
		Frozen: RenderSessionArtifactGroups(workDir, frozenGroups),
		Open:   RenderSessionArtifactGroups(workDir, openGroups),
	}
}

func RenderSessionArtifactsListing(config AICallerConfigIf) string {
	entries, workDir := CollectSessionArtifactEntries(config)
	groups := GroupSessionArtifactsByTask(entries)
	return RenderSessionArtifactGroups(workDir, groups)
}

func sessionArtifactTaskDir(relPath string) (string, bool) {
	parts := strings.SplitN(filepath.ToSlash(relPath), "/", 2)
	if len(parts) < 2 {
		return "", false
	}
	taskDir := strings.TrimSpace(parts[0])
	if !strings.HasPrefix(taskDir, "task_") {
		return "", false
	}
	return taskDir, true
}

func buildSessionArtifactTaskGroup(taskDir string, files []SessionArtifactEntry) SessionArtifactTaskGroup {
	group := SessionArtifactTaskGroup{TaskDir: taskDir, Files: append([]SessionArtifactEntry{}, files...)}
	normalizeSessionArtifactTaskGroup(&group)
	return group
}

func normalizeSessionArtifactTaskGroup(group *SessionArtifactTaskGroup) {
	if group == nil {
		return
	}
	for i := range group.Files {
		group.Files[i].RelPath = filepath.ToSlash(strings.TrimSpace(group.Files[i].RelPath))
		if group.Files[i].ModUnix > group.LatestMod {
			group.LatestMod = group.Files[i].ModUnix
		}
	}
	sort.SliceStable(group.Files, func(i, j int) bool {
		return group.Files[i].RelPath < group.Files[j].RelPath
	})
}

func compareSessionArtifactTaskDir(a string, b string) int {
	aIdx, aOK := parseSessionArtifactTaskIndex(a)
	bIdx, bOK := parseSessionArtifactTaskIndex(b)
	if aOK && bOK {
		for i := 0; i < len(aIdx) || i < len(bIdx); i++ {
			var av, bv int
			if i < len(aIdx) {
				av = aIdx[i]
			}
			if i < len(bIdx) {
				bv = bIdx[i]
			}
			if av < bv {
				return -1
			}
			if av > bv {
				return 1
			}
		}
	}
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

func parseSessionArtifactTaskIndex(taskDir string) ([]int, bool) {
	taskDir = strings.TrimPrefix(strings.TrimSpace(taskDir), "task_")
	if taskDir == "" {
		return nil, false
	}
	end := len(taskDir)
	for i, r := range taskDir {
		if (r >= '0' && r <= '9') || r == '-' || r == '.' {
			continue
		}
		end = i
		break
	}
	index := strings.Trim(taskDir[:end], "-.")
	if index == "" {
		return nil, false
	}
	parts := strings.FieldsFunc(index, func(r rune) bool {
		return r == '-' || r == '.'
	})
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, len(out) > 0
}

func formatArtifactModTime(unix int64, layout string) string {
	if unix <= 0 {
		return "unknown"
	}
	return time.Unix(unix, 0).Format(layout)
}
