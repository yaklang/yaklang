package aicommon

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
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

type SessionArtifactTaskGroupSnapshot struct {
	TaskDir   string
	Files     []SessionArtifactEntry
	LatestMod int64
}

type SessionArtifactsPromptBlocks struct {
	Frozen string
	Open   string
}

type SessionArtifactsRenderState struct {
	m sync.Mutex

	WorkDir string

	LastFrozenTimeUnix int64
	LastFrozenRendered string

	FrozenGroups map[string]SessionArtifactTaskGroupSnapshot
}

var sessionArtifactsStateByWorkDir sync.Map

func NewSessionArtifactsRenderState() *SessionArtifactsRenderState {
	return &SessionArtifactsRenderState{
		FrozenGroups: make(map[string]SessionArtifactTaskGroupSnapshot),
	}
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
	sortSessionArtifactTaskGroups(groups)
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

func RenderSessionArtifactsFrozenOpen(config AICallerConfigIf, frozenTimeUnix int64) SessionArtifactsPromptBlocks {
	entries, workDir := CollectSessionArtifactEntries(config)
	groups := GroupSessionArtifactsByTask(entries)
	state := getSessionArtifactsRenderState(config, workDir)
	if state == nil {
		state = NewSessionArtifactsRenderState()
	}

	state.m.Lock()
	defer state.m.Unlock()

	if state.WorkDir != strings.TrimSpace(workDir) {
		resetSessionArtifactsRenderState(state, workDir)
	}
	if frozenTimeUnix < state.LastFrozenTimeUnix {
		resetSessionArtifactsRenderState(state, workDir)
	}

	frozen := state.LastFrozenRendered
	if frozenTimeUnix > state.LastFrozenTimeUnix {
		UpdateSessionArtifactsFrozenState(state, workDir, groups, frozenTimeUnix)
		state.LastFrozenTimeUnix = frozenTimeUnix
		frozen = RenderSessionArtifactsFrozenFromState(state)
		state.LastFrozenRendered = frozen
	}

	return SessionArtifactsPromptBlocks{
		Frozen: frozen,
		Open:   RenderSessionArtifactsOpenFromLiveGroups(state, workDir, groups, frozenTimeUnix),
	}
}

func RenderSessionArtifactsListing(config AICallerConfigIf) string {
	entries, workDir := CollectSessionArtifactEntries(config)
	groups := GroupSessionArtifactsByTask(entries)
	return RenderSessionArtifactGroups(workDir, groups)
}

func UpdateSessionArtifactsFrozenState(
	state *SessionArtifactsRenderState,
	workDir string,
	groups []SessionArtifactTaskGroup,
	frozenTimeUnix int64,
) {
	if state == nil {
		return
	}
	if state.FrozenGroups == nil {
		state.FrozenGroups = make(map[string]SessionArtifactTaskGroupSnapshot)
	}
	workDir = strings.TrimSpace(workDir)
	if state.WorkDir != workDir {
		resetSessionArtifactsRenderState(state, workDir)
	}
	if frozenTimeUnix <= 0 {
		return
	}
	for _, group := range groups {
		normalizeSessionArtifactTaskGroup(&group)
		taskDir := strings.TrimSpace(group.TaskDir)
		if taskDir == "" {
			continue
		}
		if _, exists := state.FrozenGroups[taskDir]; exists {
			continue
		}
		if group.LatestMod < frozenTimeUnix {
			state.FrozenGroups[taskDir] = snapshotSessionArtifactTaskGroup(group)
		}
	}
}

func RenderSessionArtifactsFrozenFromState(state *SessionArtifactsRenderState) string {
	if state == nil || len(state.FrozenGroups) == 0 {
		return ""
	}
	groups := make([]SessionArtifactTaskGroup, 0, len(state.FrozenGroups))
	for _, snapshot := range state.FrozenGroups {
		groups = append(groups, SessionArtifactTaskGroup{
			TaskDir:   snapshot.TaskDir,
			Files:     append([]SessionArtifactEntry{}, snapshot.Files...),
			LatestMod: snapshot.LatestMod,
		})
	}
	sortSessionArtifactTaskGroups(groups)
	return renderSessionArtifactPromptGroups(
		state.WorkDir,
		groups,
		state.LastFrozenTimeUnix,
		nil,
	)
}

func RenderSessionArtifactsOpenFromLiveGroups(
	state *SessionArtifactsRenderState,
	workDir string,
	groups []SessionArtifactTaskGroup,
	frozenTimeUnix int64,
) string {
	if len(groups) == 0 {
		return ""
	}
	if state == nil {
		state = NewSessionArtifactsRenderState()
	}
	openGroups := make([]SessionArtifactTaskGroup, 0, len(groups))
	headerSuffix := make(map[string]string)

	for _, group := range groups {
		normalizeSessionArtifactTaskGroup(&group)
		taskDir := strings.TrimSpace(group.TaskDir)
		if taskDir == "" {
			openGroups = append(openGroups, group)
			continue
		}

		if snapshot, frozen := state.FrozenGroups[taskDir]; frozen {
			delta := diffSessionArtifactTaskGroup(snapshot, group)
			if len(delta.Files) > 0 {
				openGroups = append(openGroups, delta)
				headerSuffix[taskDir] = " (updates after frozen snapshot)"
			}
			continue
		}

		// A non-sealed group is always kept open. In the normal path, eligible
		// groups are sealed when FrozenTimeUnix advances; this also prevents a
		// newly-created/backdated group from disappearing while FrozenTimeUnix is
		// unchanged.
		openGroups = append(openGroups, group)
	}
	sortSessionArtifactTaskGroups(openGroups)
	return renderSessionArtifactPromptGroups(workDir, openGroups, frozenTimeUnix, headerSuffix)
}

func getSessionArtifactsRenderState(config AICallerConfigIf, workDir string) *SessionArtifactsRenderState {
	if cfg, ok := config.(*Config); ok && cfg != nil {
		if cfg.SessionPromptState == nil {
			if cfg.m != nil {
				cfg.m.Lock()
				if cfg.SessionPromptState == nil {
					cfg.SessionPromptState = NewSessionPromptState()
				}
				cfg.m.Unlock()
			} else {
				cfg.SessionPromptState = NewSessionPromptState()
			}
		}
		return cfg.SessionPromptState.GetOrCreateSessionArtifactsRenderState()
	}

	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return NewSessionArtifactsRenderState()
	}
	state, _ := sessionArtifactsStateByWorkDir.LoadOrStore(workDir, NewSessionArtifactsRenderState())
	if typed, ok := state.(*SessionArtifactsRenderState); ok {
		return typed
	}
	return NewSessionArtifactsRenderState()
}

func resetSessionArtifactsRenderState(state *SessionArtifactsRenderState, workDir string) {
	if state == nil {
		return
	}
	state.WorkDir = strings.TrimSpace(workDir)
	state.LastFrozenTimeUnix = 0
	state.LastFrozenRendered = ""
	state.FrozenGroups = make(map[string]SessionArtifactTaskGroupSnapshot)
}

func snapshotSessionArtifactTaskGroup(group SessionArtifactTaskGroup) SessionArtifactTaskGroupSnapshot {
	normalizeSessionArtifactTaskGroup(&group)
	return SessionArtifactTaskGroupSnapshot{
		TaskDir:   strings.TrimSpace(group.TaskDir),
		Files:     append([]SessionArtifactEntry{}, group.Files...),
		LatestMod: group.LatestMod,
	}
}

func diffSessionArtifactTaskGroup(
	snapshot SessionArtifactTaskGroupSnapshot,
	live SessionArtifactTaskGroup,
) SessionArtifactTaskGroup {
	normalizeSessionArtifactTaskGroup(&live)
	seen := make(map[string]SessionArtifactEntry, len(snapshot.Files))
	for _, file := range snapshot.Files {
		seen[file.RelPath] = file
	}

	delta := SessionArtifactTaskGroup{TaskDir: live.TaskDir}
	for _, file := range live.Files {
		prev, ok := seen[file.RelPath]
		if ok && prev.Size == file.Size && prev.ModUnix == file.ModUnix {
			continue
		}
		delta.Files = append(delta.Files, file)
	}
	normalizeSessionArtifactTaskGroup(&delta)
	return delta
}

func renderSessionArtifactPromptGroups(
	workDir string,
	groups []SessionArtifactTaskGroup,
	frozenTimeUnix int64,
	headerSuffix map[string]string,
) string {
	if len(groups) == 0 {
		return ""
	}

	normalized := make([]SessionArtifactTaskGroup, 0, len(groups))
	for _, group := range groups {
		normalizeSessionArtifactTaskGroup(&group)
		if len(group.Files) == 0 {
			continue
		}
		normalized = append(normalized, group)
	}
	if len(normalized) == 0 {
		return ""
	}

	var sb strings.Builder
	if workDir = strings.TrimSpace(workDir); workDir != "" {
		sb.WriteString(fmt.Sprintf("artifacts_dir: %s\n", workDir))
	}
	sb.WriteString(fmt.Sprintf("frozen_time: %s\n\n", formatArtifactModTime(frozenTimeUnix, "2006-01-02 15:04:05")))

	for _, group := range normalized {
		taskDir := strings.TrimSpace(group.TaskDir)
		if taskDir == "" {
			sb.WriteString("### [root files]\n")
		} else {
			sb.WriteString(fmt.Sprintf("### %s%s\n", taskDir, headerSuffix[taskDir]))
		}
		for _, file := range group.Files {
			displayPath := file.RelPath
			if taskDir != "" {
				displayPath = strings.TrimPrefix(displayPath, taskDir+"/")
			}
			sb.WriteString(fmt.Sprintf("- %s (%s, %s)\n",
				displayPath,
				formatFileSize(file.Size),
				formatArtifactModTime(file.ModUnix, "2006-01-02 15:04:05"),
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

func sortSessionArtifactTaskGroups(groups []SessionArtifactTaskGroup) {
	sort.SliceStable(groups, func(i, j int) bool {
		leftRoot := strings.TrimSpace(groups[i].TaskDir) == ""
		rightRoot := strings.TrimSpace(groups[j].TaskDir) == ""
		if leftRoot != rightRoot {
			return !leftRoot
		}
		return compareSessionArtifactTaskDir(groups[i].TaskDir, groups[j].TaskDir) < 0
	})
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
	group.TaskDir = strings.TrimSpace(group.TaskDir)
	group.LatestMod = 0
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
