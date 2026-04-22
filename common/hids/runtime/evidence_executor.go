//go:build hids && linux

package runtime

import (
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/hids/enrich"
	"github.com/yaklang/yaklang/common/hids/model"
)

const (
	evidenceProcessLineageLimit  = 8
	evidenceProcessChildrenLimit = 16
)

func (p *pipeline) enrichAlertEvidence(event model.Event, alert model.Alert) model.Alert {
	if p == nil || len(alert.Detail) == 0 {
		return alert
	}

	requests := parseEvidenceRequests(alert.Detail["evidence_requests"])
	if len(requests) == 0 {
		return alert
	}

	results := make([]map[string]any, 0, len(requests))
	errors := make([]map[string]any, 0)
	for _, request := range requests {
		result, err := p.executeEvidenceRequest(event, request)
		if err != nil {
			errors = append(errors, evidenceErrorMap(request, err))
			continue
		}
		if len(result) == 0 {
			continue
		}
		results = append(results, result)
	}

	if len(results) > 0 {
		alert.Detail["evidence_results"] = results
	}
	if len(errors) > 0 {
		alert.Detail["evidence_errors"] = errors
	}
	return p.promoteAlertFromEvidence(alert)
}

func (p *pipeline) executeEvidenceRequest(event model.Event, request map[string]any) (map[string]any, error) {
	kind := strings.ToLower(strings.TrimSpace(readEvidenceString(request, "kind")))
	switch kind {
	case "file":
		return p.executeFileEvidence(event, request)
	case "file_scan", "single_file_scan":
		return p.executeSingleFileScanEvidence(event, request)
	case "directory", "directory_scan":
		return p.executeDirectoryScanEvidence(event, request)
	case "process_tree":
		return p.executeProcessTreeEvidence(event, request)
	case "process_memory":
		return p.executeProcessMemoryEvidence(event, request)
	default:
		return nil, fmt.Errorf("unsupported evidence kind %q", kind)
	}
}

func (p *pipeline) executeFileEvidence(event model.Event, request map[string]any) (map[string]any, error) {
	path, err := resolveEvidenceFilePath(event, request)
	if err != nil {
		return nil, err
	}

	artifact, snapshotErr := p.snapshotEvidenceArtifact(path)
	if artifact == nil && snapshotErr != nil {
		return nil, snapshotErr
	}
	if artifact == nil {
		return nil, fmt.Errorf("no artifact snapshot available for %q", path)
	}

	result := evidenceResultBase(request)
	result["resolved_target"] = path
	result["artifact"] = artifactDetailMap(artifact)
	if snapshotErr != nil {
		result["warning"] = snapshotErr.Error()
	}
	return result, nil
}

func (p *pipeline) executeSingleFileScanEvidence(event model.Event, request map[string]any) (map[string]any, error) {
	path, err := resolveEvidenceFilePath(event, request)
	if err != nil {
		return nil, err
	}

	artifact, snapshotErr := p.snapshotEvidenceArtifact(path)
	if artifact == nil && snapshotErr != nil {
		return nil, snapshotErr
	}
	if artifact == nil {
		return nil, fmt.Errorf("no artifact snapshot available for %q", path)
	}
	if artifact.FileType == "directory" {
		return nil, fmt.Errorf("single_file_scan target %q is a directory", path)
	}

	result := evidenceResultBase(request)
	result["resolved_target"] = path
	scanSummary, err := p.applyScanPostFilters(request, buildSingleFileScanSummary(path, artifact))
	if err != nil {
		return nil, err
	}
	result["scan"] = scanSummary
	if snapshotErr != nil {
		result["warning"] = snapshotErr.Error()
	}
	return result, nil
}

func (p *pipeline) executeDirectoryScanEvidence(event model.Event, request map[string]any) (map[string]any, error) {
	path, err := resolveEvidenceDirectoryPath(event, request)
	if err != nil {
		return nil, err
	}

	scanResult, scanErr := p.scanDirectory(path, normalizeDirectoryScanOptions(request))
	if scanErr != nil {
		return nil, scanErr
	}

	result := evidenceResultBase(request)
	result["resolved_target"] = path
	scanSummary, err := p.applyScanPostFilters(request, buildDirectoryScanSummary(scanResult))
	if err != nil {
		return nil, err
	}
	result["scan"] = scanSummary
	return result, nil
}

func (p *pipeline) executeProcessTreeEvidence(event model.Event, request map[string]any) (map[string]any, error) {
	process, err := p.resolveEvidenceProcess(event, request)
	if err != nil {
		return nil, err
	}

	lineage, lineageTruncated := p.buildEvidenceLineage(*process)
	children, childrenTruncated := p.buildEvidenceChildren(process.PID)

	tree := map[string]any{
		"process":           p.processDetailMap(*process),
		"lineage":           lineage,
		"lineage_truncated": lineageTruncated,
	}
	if p.evidencePolicy.CaptureProcessTree {
		tree["children"] = children
		tree["children_truncated"] = childrenTruncated
	}

	result := evidenceResultBase(request)
	result["resolved_target"] = process.PID
	result["process_tree"] = tree
	return result, nil
}

func (p *pipeline) executeProcessMemoryEvidence(event model.Event, request map[string]any) (map[string]any, error) {
	if p == nil || !p.evidencePolicy.CaptureProcessMemory {
		return nil, fmt.Errorf("process_memory evidence requires evidence_policy.capture_process_memory")
	}

	process, err := p.resolveEvidenceProcess(event, request)
	if err != nil {
		return nil, err
	}

	processDetail := p.processDetailMap(*process)
	memoryEvidence, err := captureProcessMemoryEvidence(
		processDetail,
		process.PID,
		normalizeProcessMemoryEvidenceOptions(request),
	)
	if err != nil {
		return nil, err
	}

	result := evidenceResultBase(request)
	result["resolved_target"] = process.PID
	result["process_memory"] = memoryEvidence
	return result, nil
}

func (p *pipeline) snapshotEvidenceArtifact(path string) (*model.Artifact, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("artifact path is required")
	}
	if p != nil && p.artifacts != nil {
		return p.artifacts.snapshotWithError(path)
	}
	return enrich.SnapshotArtifact(path, enrich.ArtifactSnapshotOptions{
		CaptureHashes: p != nil && p.evidencePolicy.CaptureFileHash,
	})
}

func (p *pipeline) resolveEvidenceProcess(event model.Event, request map[string]any) (*model.Process, error) {
	pid, err := resolveEvidenceProcessPID(event, request)
	if err != nil {
		return nil, err
	}

	if event.Process != nil && event.Process.PID == pid {
		cloned := *event.Process
		normalizeProcessContext(&cloned)
		p.attachProcessArtifact(&cloned)
		return &cloned, nil
	}

	if p != nil && p.processes != nil {
		if cached, ok := p.processes.byPID[pid]; ok {
			cloned := cached
			normalizeProcessContext(&cloned)
			p.attachProcessArtifact(&cloned)
			return &cloned, nil
		}
	}
	return nil, fmt.Errorf("process pid %d is not available in current runtime state", pid)
}

func (p *pipeline) buildEvidenceLineage(process model.Process) ([]map[string]any, bool) {
	lineage := []model.Process{process}
	visited := map[int]struct{}{}
	if process.PID > 0 {
		visited[process.PID] = struct{}{}
	}

	truncated := false
	current := process
	for len(lineage) < evidenceProcessLineageLimit && current.ParentPID > 0 {
		if _, seen := visited[current.ParentPID]; seen {
			truncated = true
			break
		}
		parent, ok := p.lookupTrackedProcess(current.ParentPID)
		if !ok {
			break
		}
		visited[parent.PID] = struct{}{}
		current = parent
		lineage = append(lineage, current)
	}
	if current.ParentPID > 0 && len(lineage) >= evidenceProcessLineageLimit {
		truncated = true
	}

	reversed := make([]map[string]any, 0, len(lineage))
	for index := len(lineage) - 1; index >= 0; index-- {
		reversed = append(reversed, p.processDetailMap(lineage[index]))
	}
	return reversed, truncated
}

func (p *pipeline) buildEvidenceChildren(pid int) ([]map[string]any, bool) {
	if p == nil || p.processes == nil || pid <= 0 {
		return nil, false
	}

	children := make([]model.Process, 0)
	for _, candidate := range p.processes.byPID {
		if candidate.ParentPID != pid {
			continue
		}
		cloned := candidate
		normalizeProcessContext(&cloned)
		p.attachProcessArtifact(&cloned)
		children = append(children, cloned)
	}
	if len(children) == 0 {
		return nil, false
	}

	sort.Slice(children, func(left, right int) bool {
		if children[left].PID == children[right].PID {
			return children[left].Image < children[right].Image
		}
		return children[left].PID < children[right].PID
	})

	truncated := false
	if len(children) > evidenceProcessChildrenLimit {
		children = children[:evidenceProcessChildrenLimit]
		truncated = true
	}

	results := make([]map[string]any, 0, len(children))
	for _, child := range children {
		results = append(results, p.processDetailMap(child))
	}
	return results, truncated
}

func (p *pipeline) lookupTrackedProcess(pid int) (model.Process, bool) {
	if p == nil || p.processes == nil || pid <= 0 {
		return model.Process{}, false
	}
	process, ok := p.processes.byPID[pid]
	if !ok {
		return model.Process{}, false
	}
	normalizeProcessContext(&process)
	p.attachProcessArtifact(&process)
	return process, true
}

func (p *pipeline) attachProcessArtifact(process *model.Process) {
	if process == nil || process.Artifact != nil {
		return
	}
	image := strings.TrimSpace(process.Image)
	if image == "" {
		return
	}
	artifact, _ := p.snapshotEvidenceArtifact(image)
	if artifact != nil {
		process.Artifact = artifact
	}
}

func (p *pipeline) processDetailMap(process model.Process) map[string]any {
	normalizeProcessContext(&process)
	p.attachProcessArtifact(&process)

	detail := map[string]any{
		"pid":                       process.PID,
		"parent_pid":                process.ParentPID,
		"name":                      process.Name,
		"username":                  process.Username,
		"image":                     process.Image,
		"command":                   process.Command,
		"parent_name":               process.ParentName,
		"parent_image":              process.ParentImage,
		"parent_command":            process.ParentCommand,
		"parent_start_time_unix_ms": process.ParentStartTimeUnixMillis,
	}
	if process.Artifact != nil {
		detail["artifact"] = artifactDetailMap(process.Artifact)
	}
	return detail
}

func evidenceResultBase(request map[string]any) map[string]any {
	result := map[string]any{
		"kind": strings.TrimSpace(readEvidenceString(request, "kind")),
	}
	if target := strings.TrimSpace(readEvidenceString(request, "target")); target != "" {
		result["target"] = target
	}
	if reason := strings.TrimSpace(readEvidenceString(request, "reason")); reason != "" {
		result["reason"] = reason
	}
	if metadata := readEvidenceMetadata(request); len(metadata) > 0 {
		result["metadata"] = metadata
	}
	return result
}

func evidenceErrorMap(request map[string]any, err error) map[string]any {
	result := evidenceResultBase(request)
	result["error"] = err.Error()
	return result
}

func resolveEvidenceFilePath(event model.Event, request map[string]any) (string, error) {
	metadata := readEvidenceMetadata(request)
	for _, key := range []string{"path", "file_path", "image"} {
		if value := strings.TrimSpace(readEvidenceString(metadata, key)); value != "" {
			return value, nil
		}
	}

	target := strings.TrimSpace(readEvidenceString(request, "target"))
	switch target {
	case "", "file", "file.path":
		if event.File != nil && strings.TrimSpace(event.File.Path) != "" {
			return strings.TrimSpace(event.File.Path), nil
		}
	case "process", "process.image":
		if event.Process != nil && strings.TrimSpace(event.Process.Image) != "" {
			return strings.TrimSpace(event.Process.Image), nil
		}
	default:
		if target != "" {
			return target, nil
		}
	}

	if event.File != nil && strings.TrimSpace(event.File.Path) != "" {
		return strings.TrimSpace(event.File.Path), nil
	}
	if event.Process != nil && strings.TrimSpace(event.Process.Image) != "" {
		return strings.TrimSpace(event.Process.Image), nil
	}
	return "", fmt.Errorf("file evidence request did not resolve to a path")
}

func resolveEvidenceProcessPID(event model.Event, request map[string]any) (int, error) {
	metadata := readEvidenceMetadata(request)
	for _, raw := range []any{metadata["pid"], request["pid"]} {
		if pid, ok := anyToInt(raw); ok && pid > 0 {
			return pid, nil
		}
	}

	target := strings.TrimSpace(readEvidenceString(request, "target"))
	switch strings.ToLower(target) {
	case "", "process":
		if event.Process != nil && event.Process.PID > 0 {
			return event.Process.PID, nil
		}
	case "parent":
		if event.Process != nil && event.Process.ParentPID > 0 {
			return event.Process.ParentPID, nil
		}
	default:
		if pid, err := strconv.Atoi(target); err == nil && pid > 0 {
			return pid, nil
		}
	}

	if event.Process != nil && event.Process.PID > 0 {
		return event.Process.PID, nil
	}
	return 0, fmt.Errorf("process evidence request did not resolve to a pid")
}

func resolveEvidenceDirectoryPath(event model.Event, request map[string]any) (string, error) {
	metadata := readEvidenceMetadata(request)
	for _, key := range []string{"dir", "path", "root"} {
		if value := strings.TrimSpace(readEvidenceString(metadata, key)); value != "" {
			return value, nil
		}
	}

	target := strings.TrimSpace(readEvidenceString(request, "target"))
	switch strings.ToLower(target) {
	case "", "file.parent":
		if event.File != nil && strings.TrimSpace(event.File.Path) != "" {
			return filepath.Dir(strings.TrimSpace(event.File.Path)), nil
		}
	case "process.image_dir", "process.dir":
		if event.Process != nil && strings.TrimSpace(event.Process.Image) != "" {
			return filepath.Dir(strings.TrimSpace(event.Process.Image)), nil
		}
	default:
		if target != "" {
			return target, nil
		}
	}

	if event.File != nil && strings.TrimSpace(event.File.Path) != "" {
		return filepath.Dir(strings.TrimSpace(event.File.Path)), nil
	}
	if event.Process != nil && strings.TrimSpace(event.Process.Image) != "" {
		return filepath.Dir(strings.TrimSpace(event.Process.Image)), nil
	}
	return "", fmt.Errorf("directory_scan evidence request did not resolve to a directory path")
}

func parseEvidenceRequests(value any) []map[string]any {
	if value == nil {
		return nil
	}

	switch typed := value.(type) {
	case []map[string]any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			if len(item) == 0 {
				continue
			}
			result = append(result, cloneEvidenceMap(item))
		}
		return result
	case []any:
		result := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			request := cloneEvidenceMap(readEvidenceMap(item))
			if len(request) == 0 {
				continue
			}
			result = append(result, request)
		}
		return result
	}

	reflected := reflect.ValueOf(value)
	if reflected.IsValid() && reflected.Kind() == reflect.Slice {
		result := make([]map[string]any, 0, reflected.Len())
		for index := 0; index < reflected.Len(); index++ {
			request := cloneEvidenceMap(readEvidenceMap(reflected.Index(index).Interface()))
			if len(request) == 0 {
				continue
			}
			result = append(result, request)
		}
		return result
	}

	request := cloneEvidenceMap(readEvidenceMap(value))
	if len(request) == 0 {
		return nil
	}
	return []map[string]any{request}
}

func readEvidenceMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[string]string:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			result[key] = item
		}
		return result
	default:
		reflected := reflect.ValueOf(value)
		if !reflected.IsValid() || reflected.Kind() != reflect.Map {
			return nil
		}
		result := make(map[string]any, reflected.Len())
		for _, key := range reflected.MapKeys() {
			if key.Kind() != reflect.String {
				continue
			}
			result[key.String()] = reflected.MapIndex(key).Interface()
		}
		return result
	}
}

func readEvidenceMetadata(request map[string]any) map[string]any {
	if len(request) == 0 {
		return nil
	}
	metadata := readEvidenceMap(request["metadata"])
	if len(metadata) == 0 {
		return nil
	}
	return cloneEvidenceMap(metadata)
}

func readEvidenceString(values map[string]any, key string) string {
	if len(values) == 0 {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func anyToInt(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int32:
		return int(typed), true
	case int64:
		return int(typed), true
	case uint32:
		return int(typed), true
	case uint64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func anyToBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case uint32:
		return typed != 0
	case uint64:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func artifactDetailMap(artifact *model.Artifact) map[string]any {
	if artifact == nil {
		return nil
	}
	detail := map[string]any{
		"path":        artifact.Path,
		"exists":      artifact.Exists,
		"size_bytes":  artifact.SizeBytes,
		"file_type":   artifact.FileType,
		"type_source": artifact.TypeSource,
		"magic":       artifact.Magic,
		"mime_type":   artifact.MimeType,
		"extension":   artifact.Extension,
	}
	if artifact.Hashes != nil {
		detail["hashes"] = map[string]any{
			"sha256": artifact.Hashes.SHA256,
			"md5":    artifact.Hashes.MD5,
		}
	}
	if artifact.ELF != nil {
		detail["elf"] = map[string]any{
			"class":         artifact.ELF.Class,
			"machine":       artifact.ELF.Machine,
			"byte_order":    artifact.ELF.ByteOrder,
			"entry_address": artifact.ELF.EntryAddress,
			"section_count": artifact.ELF.SectionCount,
			"segment_count": artifact.ELF.SegmentCount,
			"sections":      cloneStringSlice(artifact.ELF.Sections),
			"segments":      cloneStringSlice(artifact.ELF.Segments),
		}
	}
	return detail
}

func cloneEvidenceMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneEvidenceValue(value)
	}
	return cloned
}

func cloneEvidenceValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneEvidenceMap(typed)
	case []map[string]any:
		cloned := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneEvidenceMap(item))
		}
		return cloned
	case []any:
		cloned := make([]any, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneEvidenceValue(item))
		}
		return cloned
	case []string:
		return cloneStringSlice(typed)
	default:
		return typed
	}
}

func firstNonNilEvidence(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}
