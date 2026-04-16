//go:build hids && linux

package runtime

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	defaultProcessMemoryMapLimit = 16
	maxProcessMemoryMapLimit     = 64
)

type processMemoryEvidenceOptions struct {
	MapLimit       int
	IncludeAllMaps bool
}

type procMapEntry struct {
	AddressRange string
	Permissions  string
	Offset       string
	Device       string
	Inode        string
	Path         string
	FileBacked   bool
	Executable   bool
	Anonymous    bool
}

func normalizeProcessMemoryEvidenceOptions(request map[string]any) processMemoryEvidenceOptions {
	metadata := readEvidenceMetadata(request)
	limit, ok := anyToInt(firstNonNilEvidence(
		metadata["map_limit"],
		request["map_limit"],
		metadata["max_maps"],
		request["max_maps"],
	))
	if !ok || limit <= 0 {
		limit = defaultProcessMemoryMapLimit
	}
	if limit > maxProcessMemoryMapLimit {
		limit = maxProcessMemoryMapLimit
	}
	return processMemoryEvidenceOptions{
		MapLimit: limit,
		IncludeAllMaps: anyToBool(firstNonNilEvidence(
			metadata["include_all_maps"],
			request["include_all_maps"],
		)),
	}
}

func captureProcessMemoryEvidence(
	process map[string]any,
	pid int,
	options processMemoryEvidenceOptions,
) (map[string]any, error) {
	status, err := readProcStatusSummary(pid)
	if err != nil {
		return nil, err
	}

	mapEntries, mapSummary, err := readProcMapsSummary(pid, options)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"process":           cloneEvidenceMap(process),
		"status":            status,
		"maps":              mapEntries,
		"map_count":         mapSummary.totalCount,
		"sampled_map_count": len(mapEntries),
		"sample_mode":       mapSummary.sampleMode,
		"truncated":         mapSummary.truncated,
	}
	if mapSummary.interestingCount > 0 {
		result["interesting_map_count"] = mapSummary.interestingCount
	}
	if mapSummary.executableCount > 0 {
		result["executable_map_count"] = mapSummary.executableCount
	}
	if mapSummary.fileBackedCount > 0 {
		result["file_backed_map_count"] = mapSummary.fileBackedCount
	}
	return result, nil
}

func readProcStatusSummary(pid int) (map[string]any, error) {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return nil, fmt.Errorf("read /proc/%d/status: %w", pid, err)
	}

	status := map[string]any{}
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Name":
			status["name"] = value
		case "State":
			status["state"] = value
		case "Threads":
			if parsed, ok := parseProcIntValue(value); ok {
				status["threads"] = parsed
			}
		case "FDSize":
			if parsed, ok := parseProcIntValue(value); ok {
				status["fd_size"] = parsed
			}
		case "VmSize":
			if parsed, ok := parseProcKBValue(value); ok {
				status["vm_size_kb"] = parsed
			}
		case "VmRSS":
			if parsed, ok := parseProcKBValue(value); ok {
				status["vm_rss_kb"] = parsed
			}
		case "RssAnon":
			if parsed, ok := parseProcKBValue(value); ok {
				status["rss_anon_kb"] = parsed
			}
		case "RssFile":
			if parsed, ok := parseProcKBValue(value); ok {
				status["rss_file_kb"] = parsed
			}
		case "RssShmem":
			if parsed, ok := parseProcKBValue(value); ok {
				status["rss_shmem_kb"] = parsed
			}
		case "VmSwap":
			if parsed, ok := parseProcKBValue(value); ok {
				status["vm_swap_kb"] = parsed
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan /proc/%d/status: %w", pid, err)
	}
	return status, nil
}

type procMapsSummary struct {
	totalCount       int
	interestingCount int
	executableCount  int
	fileBackedCount  int
	truncated        bool
	sampleMode       string
}

func readProcMapsSummary(
	pid int,
	options processMemoryEvidenceOptions,
) ([]map[string]any, procMapsSummary, error) {
	content, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, procMapsSummary{}, fmt.Errorf("read /proc/%d/maps: %w", pid, err)
	}

	summary := procMapsSummary{sampleMode: "interesting"}
	if options.IncludeAllMaps {
		summary.sampleMode = "all"
	}

	selected := make([]map[string]any, 0, options.MapLimit)
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		entry, ok := parseProcMapEntry(scanner.Text())
		if !ok {
			continue
		}
		summary.totalCount++
		if entry.Executable {
			summary.executableCount++
		}
		if entry.FileBacked {
			summary.fileBackedCount++
		}
		isInteresting := entry.Executable || entry.FileBacked
		if isInteresting {
			summary.interestingCount++
		}

		if !options.IncludeAllMaps && !isInteresting {
			continue
		}
		if len(selected) < options.MapLimit {
			selected = append(selected, procMapEntryMap(entry))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, procMapsSummary{}, fmt.Errorf("scan /proc/%d/maps: %w", pid, err)
	}

	eligibleCount := summary.interestingCount
	if options.IncludeAllMaps {
		eligibleCount = summary.totalCount
	}
	summary.truncated = eligibleCount > len(selected)
	return selected, summary, nil
}

func parseProcMapEntry(line string) (procMapEntry, bool) {
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) < 5 {
		return procMapEntry{}, false
	}

	entry := procMapEntry{
		AddressRange: fields[0],
		Permissions:  fields[1],
		Offset:       fields[2],
		Device:       fields[3],
		Inode:        fields[4],
	}
	if len(fields) > 5 {
		entry.Path = strings.Join(fields[5:], " ")
	}
	entry.Anonymous = entry.Path == "" || strings.HasPrefix(entry.Path, "[")
	entry.FileBacked = !entry.Anonymous
	entry.Executable = strings.Contains(entry.Permissions, "x")
	return entry, true
}

func procMapEntryMap(entry procMapEntry) map[string]any {
	result := map[string]any{
		"address_range": entry.AddressRange,
		"permissions":   entry.Permissions,
		"offset":        entry.Offset,
		"device":        entry.Device,
		"inode":         entry.Inode,
		"file_backed":   entry.FileBacked,
		"executable":    entry.Executable,
		"anonymous":     entry.Anonymous,
	}
	if entry.Path != "" {
		result["path"] = entry.Path
	}
	return result
}

func parseProcKBValue(value string) (int64, bool) {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return 0, false
	}
	parsed, err := strconv.ParseInt(fields[0], 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func parseProcIntValue(value string) (int64, bool) {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}
