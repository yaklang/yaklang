package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type caseResult struct {
	Behavior      string `json:"behavior"`
	BehaviorClose string `json:"behaviorClose"`
}

type report map[string]map[string]caseResult

var hardFailures = map[string]bool{
	"FAILED":        true,
	"MISSING":       true,
	"UNIMPLEMENTED": true,
}

func statusCounts(cases map[string]caseResult, selectStatus func(caseResult) string) string {
	counts := make(map[string]int)
	for _, result := range cases {
		counts[selectStatus(result)]++
	}
	statuses := make([]string, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	parts := make([]string, 0, len(statuses))
	for _, status := range statuses {
		parts = append(parts, fmt.Sprintf("%s=%d", status, counts[status]))
	}
	return strings.Join(parts, ", ")
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: check_report.go <Autobahn index.json>")
		os.Exit(2)
	}

	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read Autobahn report: %v\n", err)
		os.Exit(2)
	}
	var parsed report
	if err := json.Unmarshal(raw, &parsed); err != nil {
		fmt.Fprintf(os.Stderr, "parse Autobahn report: %v\n", err)
		os.Exit(2)
	}
	if len(parsed) == 0 {
		fmt.Fprintln(os.Stderr, "Autobahn report contains no agents")
		os.Exit(1)
	}

	agents := make([]string, 0, len(parsed))
	for agent := range parsed {
		agents = append(agents, agent)
	}
	sort.Strings(agents)
	failures := make([]string, 0)
	for _, agent := range agents {
		cases := parsed[agent]
		fmt.Printf("%s behavior: %s\n", agent, statusCounts(cases, func(result caseResult) string { return result.Behavior }))
		fmt.Printf("%s close: %s\n", agent, statusCounts(cases, func(result caseResult) string { return result.BehaviorClose }))
		for caseID, result := range cases {
			// Gorilla is a differential reference, not the implementation under
			// test. Some core UTF-8 cases are intentionally stricter than Gorilla.
			if agent != autobahnGorillaDirectAgent && (hardFailures[result.Behavior] || hardFailures[result.BehaviorClose]) {
				failures = append(failures, fmt.Sprintf("%s case %s: behavior=%s close=%s", agent, caseID, result.Behavior, result.BehaviorClose))
			}
		}
	}

	direct, hasDirect := parsed[autobahnGorillaDirectAgent]
	viaMITM, hasMITM := parsed[autobahnGorillaMITMAgent]
	if hasDirect && hasMITM {
		for caseID, directResult := range direct {
			mitmResult, ok := viaMITM[caseID]
			if !ok {
				failures = append(failures, fmt.Sprintf("MITM report is missing Gorilla baseline case %s", caseID))
				continue
			}
			if !hardFailures[directResult.Behavior] && hardFailures[mitmResult.Behavior] ||
				!hardFailures[directResult.BehaviorClose] && hardFailures[mitmResult.BehaviorClose] {
				failures = append(failures, fmt.Sprintf("MITM regression in case %s: direct=%s/%s mitm=%s/%s", caseID, directResult.Behavior, directResult.BehaviorClose, mitmResult.Behavior, mitmResult.BehaviorClose))
			}
		}
	}

	if len(failures) > 0 {
		sort.Strings(failures)
		for _, failure := range failures {
			fmt.Fprintln(os.Stderr, failure)
		}
		os.Exit(1)
	}
}

const (
	autobahnGorillaDirectAgent = "gorilla-direct"
	autobahnGorillaMITMAgent   = "gorilla-via-yak-mitm"
)
