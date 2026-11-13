package differential

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Options struct {
	External ExternalConfig
	Timeout  time.Duration
	WorkDir  string
	Keep     bool
}

type CaseResult struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	RuleSHA256  string        `json:"rule_sha256"`
	PCAPSHA256  string        `json:"pcap_sha256"`
	Results     []Result      `json:"results"`
	Local       *LocalResult  `json:"local,omitempty"`
	Comparison  Comparison    `json:"comparison"`
	Artifacts   *CaseArtifact `json:"artifacts,omitempty"`
}

type CaseArtifact struct {
	Rules string `json:"rules"`
	PCAP  string `json:"pcap"`
}

type Comparison struct {
	Equal         bool              `json:"equal"`
	LocalOnly     []AlertIdentity   `json:"local_only,omitempty"`
	ReferenceOnly []AlertIdentity   `json:"reference_only,omitempty"`
	CountMismatch []CountMismatch   `json:"count_mismatch,omitempty"`
	Failures      map[Engine]string `json:"failures,omitempty"`
}

type AlertIdentity struct {
	GID int `json:"gid"`
	SID int `json:"sid"`
	Rev int `json:"rev"`
}

type CountMismatch struct {
	Engine    Engine        `json:"engine"`
	Identity  AlertIdentity `json:"identity"`
	Local     int           `json:"local"`
	Reference int           `json:"reference"`
}

func RunPaths(ctx context.Context, rulesPath, pcapPath string, engines []Engine, options Options) CaseResult {
	result := CaseResult{
		Name:        filepath.Base(pcapPath),
		Description: "user-supplied rules and capture",
		RuleSHA256:  fileSHA256(rulesPath),
		PCAPSHA256:  fileSHA256(pcapPath),
		Artifacts:   &CaseArtifact{Rules: rulesPath, PCAP: pcapPath},
	}
	for _, current := range engines {
		runCtx := ctx
		cancel := func() {}
		if options.Timeout > 0 {
			runCtx, cancel = context.WithTimeout(ctx, options.Timeout)
		}
		switch current {
		case EngineYaklang:
			local := RunYaklang(rulesPath, pcapPath)
			result.Local = &local
			result.Results = append(result.Results, local.Result)
		case EngineSuricata:
			result.Results = append(result.Results, options.External.RunSuricata(runCtx, rulesPath, pcapPath))
		case EngineSnort:
			result.Results = append(result.Results, options.External.RunSnort(runCtx, rulesPath, pcapPath))
		}
		cancel()
	}
	result.Comparison = compareResults(result.Results)
	return result
}

func RunBuiltin(ctx context.Context, options Options) ([]CaseResult, error) {
	root := options.WorkDir
	owned := false
	if root == "" {
		var err error
		root, err = os.MkdirTemp("", "yak-suricata-differential-")
		if err != nil {
			return nil, err
		}
		owned = true
	} else if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if owned && !options.Keep {
		defer os.RemoveAll(root)
	}

	var results []CaseResult
	for _, testCase := range BuiltinCases() {
		result, err := runCase(ctx, root, testCase, options)
		if err != nil {
			return nil, fmt.Errorf("run case %s: %w", testCase.Name, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func runCase(ctx context.Context, root string, testCase Case, options Options) (CaseResult, error) {
	caseDir := filepath.Join(root, testCase.Name)
	if err := os.MkdirAll(caseDir, 0o700); err != nil {
		return CaseResult{}, err
	}
	rulesPath := filepath.Join(caseDir, "rules.rules")
	pcapPath := filepath.Join(caseDir, "capture.pcap")
	if err := os.WriteFile(rulesPath, []byte(testCase.Rules+"\n"), 0o600); err != nil {
		return CaseResult{}, err
	}
	frames, err := testCase.Traffic.Frames()
	if err != nil {
		return CaseResult{}, err
	}
	if err := WritePCAP(pcapPath, frames); err != nil {
		return CaseResult{}, err
	}

	result := CaseResult{
		Name:        testCase.Name,
		Description: testCase.Description,
		RuleSHA256:  fileSHA256(rulesPath),
		PCAPSHA256:  fileSHA256(pcapPath),
	}
	if options.Keep || options.WorkDir != "" {
		result.Artifacts = &CaseArtifact{Rules: rulesPath, PCAP: pcapPath}
	}

	for _, current := range testCase.Engines {
		runCtx := ctx
		cancel := func() {}
		if options.Timeout > 0 {
			runCtx, cancel = context.WithTimeout(ctx, options.Timeout)
		}
		switch current {
		case EngineYaklang:
			local := RunYaklang(rulesPath, pcapPath)
			result.Local = &local
			result.Results = append(result.Results, local.Result)
		case EngineSuricata:
			result.Results = append(result.Results, options.External.RunSuricata(runCtx, rulesPath, pcapPath))
		case EngineSnort:
			result.Results = append(result.Results, options.External.RunSnort(runCtx, rulesPath, pcapPath))
		}
		cancel()
	}
	result.Comparison = compareResults(result.Results)
	return result, nil
}

func compareResults(results []Result) Comparison {
	comparison := Comparison{Equal: true, Failures: make(map[Engine]string)}
	var local *Result
	for index := range results {
		current := &results[index]
		if current.Error != "" {
			comparison.Equal = false
			comparison.Failures[current.Engine] = current.Error
		}
		if current.Engine == EngineYaklang {
			local = current
		}
	}
	if local == nil || local.Error != "" {
		comparison.Equal = false
		return comparison
	}

	localCounts := alertCounts(local.Alerts)
	for _, reference := range results {
		if reference.Engine == EngineYaklang || reference.Error != "" {
			continue
		}
		if local.RuleStats != nil && reference.RuleStats != nil && local.RuleStats.Loaded != reference.RuleStats.Loaded {
			comparison.Failures[reference.Engine] = fmt.Sprintf("rule load count differs: local=%d reference=%d", local.RuleStats.Loaded, reference.RuleStats.Loaded)
			comparison.Equal = false
			continue
		}
		referenceCounts := alertCounts(reference.Alerts)
		for identity, localCount := range localCounts {
			referenceCount, exists := referenceCounts[identity]
			if !exists {
				comparison.LocalOnly = appendUniqueIdentity(comparison.LocalOnly, identity)
				comparison.Equal = false
				continue
			}
			if localCount != referenceCount {
				comparison.CountMismatch = append(comparison.CountMismatch, CountMismatch{Engine: reference.Engine, Identity: identity, Local: localCount, Reference: referenceCount})
				comparison.Equal = false
			}
		}
		for identity := range referenceCounts {
			if _, exists := localCounts[identity]; !exists {
				comparison.ReferenceOnly = appendUniqueIdentity(comparison.ReferenceOnly, identity)
				comparison.Equal = false
			}
		}
	}
	sortIdentities(comparison.LocalOnly)
	sortIdentities(comparison.ReferenceOnly)
	if len(comparison.Failures) == 0 {
		comparison.Failures = nil
	}
	return comparison
}

func alertCounts(alerts []Alert) map[AlertIdentity]int {
	result := make(map[AlertIdentity]int)
	for _, alert := range alerts {
		result[AlertIdentity{GID: alert.GID, SID: alert.SID, Rev: alert.Rev}] += alert.Count
	}
	return result
}

func appendUniqueIdentity(items []AlertIdentity, identity AlertIdentity) []AlertIdentity {
	for _, item := range items {
		if item == identity {
			return items
		}
	}
	return append(items, identity)
}

func sortIdentities(items []AlertIdentity) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].GID != items[j].GID {
			return items[i].GID < items[j].GID
		}
		if items[i].SID != items[j].SID {
			return items[i].SID < items[j].SID
		}
		return items[i].Rev < items[j].Rev
	})
}

func fileSHA256(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
