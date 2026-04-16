//go:build hids

package rule

import "github.com/yaklang/yaklang/common/hids/model"

func BuildScanEvaluationContext(
	scan map[string]any,
	request map[string]any,
	entry map[string]any,
	finding map[string]any,
) map[string]any {
	scanValue := cloneMapStringAny(scan)
	requestValue := cloneMapStringAny(request)
	entryValue := cloneMapStringAny(entry)
	findingValue := cloneMapStringAny(finding)

	ctx := map[string]any{
		"scan":    scanValue,
		"request": requestValue,
		"entry":   entryValue,
		"finding": findingValue,
	}
	if len(scanValue) > 0 {
		ctx["matched_rules"] = cloneValue(scanValue["matched_rules"])
		ctx["findings"] = cloneValue(scanValue["findings"])
		ctx["entries"] = cloneValue(scanValue["entries"])
	}
	if artifact := helperGeneralMap(firstNonNil(
		entryValue["artifact"],
		scanValue["artifact"],
		scanValue["target"],
	)); len(artifact) > 0 {
		ctx["artifact"] = cloneMapStringAny(artifact)
	} else {
		ctx["artifact"] = buildArtifactContext(nil)
	}
	return ctx
}

func buildScanValidationContext() map[string]any {
	target := buildArtifactContext(&model.Artifact{
		Path:       "/tmp",
		Exists:     true,
		FileType:   "directory",
		TypeSource: "fs",
	})
	artifact := buildArtifactContext(&model.Artifact{
		Path:       "/tmp/payload",
		Exists:     true,
		FileType:   "elf",
		TypeSource: "magic",
		Magic:      "7f454c4602010100",
		Hashes: &model.ArtifactHashes{
			SHA256: "deadbeef",
			MD5:    "cafebabe",
		},
		ELF: &model.ELFArtifact{
			Machine: "EM_X86_64",
		},
	})
	entry := map[string]any{
		"path":          "/tmp/payload",
		"relative_path": "payload",
		"depth":         1,
		"is_dir":        false,
		"artifact":      artifact,
	}
	finding := map[string]any{
		"rule_id":  "linux.scan.writable_tmp_elf_artifact",
		"severity": "high",
		"title":    "ELF artifact found under writable tmp path during bounded scan",
		"tags":     []string{"builtin", "scan", "file", "tmp", "artifact", "elf"},
		"detail": map[string]any{
			"path":      "/tmp/payload",
			"file_type": "elf",
			"sha256":    "deadbeef",
		},
	}
	scan := map[string]any{
		"mode":            "directory",
		"recursive":       true,
		"max_entries":     8,
		"max_depth":       2,
		"scanned_count":   1,
		"file_count":      1,
		"directory_count": 0,
		"truncated":       false,
		"matched_rules":   []string{"linux.scan.writable_tmp_elf_artifact"},
		"findings":        []map[string]any{finding},
		"finding_count":   1,
		"entries":         []map[string]any{entry},
		"target":          target,
	}
	request := map[string]any{
		"kind":   "directory_scan",
		"target": "/tmp",
		"reason": "bounded scan",
		"metadata": map[string]any{
			"scan_match":    "list.Contains(scan.matched_rules, 'linux.scan.writable_tmp_elf_artifact')",
			"entry_match":   "artifact.IsELF(entry.artifact)",
			"finding_match": "finding.rule_id == 'linux.scan.writable_tmp_elf_artifact'",
			"matched_only":  true,
		},
	}
	return BuildScanEvaluationContext(scan, request, entry, finding)
}
