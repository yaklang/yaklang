package loop_ssa_api_discovery

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

func controllerStemFromEntryFile(entryFile string) string {
	base := strings.TrimSuffix(filepath.Base(entryFile), ".java")
	base = strings.TrimSuffix(base, "Controller")
	base = strings.TrimSuffix(base, "Admin")
	if base == "" {
		return ""
	}
	runes := []rune(base)
	return strings.ToLower(string(runes[0])) + string(runes[1:])
}

func frontendCallsForControllerJob(workDir string, job FeatureWorkJob) []FrontendAPICall {
	inv, err := loadFrontendAPIInventory(workDir)
	if err != nil || inv == nil {
		harvest, herr := loadFrontendAPIHarvest(workDir)
		if herr != nil || harvest == nil {
			return nil
		}
		return filterFrontendCallsForJob(harvest.Calls, job)
	}
	return filterFrontendCallsForJob(inv.Calls, job)
}

func filterFrontendCallsForJob(calls []FrontendAPICall, job FeatureWorkJob) []FrontendAPICall {
	stem := strings.ToLower(controllerStemFromEntryFile(job.EntryFile))
	if stem == "" {
		return nil
	}
	var out []FrontendAPICall
	for _, c := range calls {
		path := strings.ToLower(firstNonEmpty(c.PathResolved, c.PathRaw))
		if path == "" {
			continue
		}
		if strings.Contains(path, "/"+stem+"/") || strings.HasSuffix(path, "/"+stem) || strings.Contains(path, stem+"/") {
			out = append(out, c)
			continue
		}
		if hint := strings.ToLower(c.LinkedHandlerHint); hint != "" {
			entryBase := strings.ToLower(strings.TrimSuffix(filepath.Base(job.EntryFile), ".java"))
			if strings.Contains(hint, entryBase) || strings.Contains(entryBase, strings.TrimSuffix(hint, "controller")) {
				out = append(out, c)
			}
		}
	}
	return out
}

func buildFrontendAPIHintsBlock(rt *Runtime, job FeatureWorkJob) string {
	if rt == nil {
		return ""
	}
	calls := frontendCallsForControllerJob(rt.WorkDir, job)
	if len(calls) == 0 {
		return ""
	}
	if len(calls) > 25 {
		calls = calls[:25]
	}
	b, _ := json.MarshalIndent(calls, "", "  ")
	return "## frontend_api_hints (hint only — from templates/JS)\n" +
		"Use for S2 path confirmation and S3 param names (_csrf, form fields). Not verified.\n```json\n" +
		string(b) + "\n```"
}
