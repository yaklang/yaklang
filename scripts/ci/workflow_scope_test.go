package ci

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Exercise the actual workflow configuration so moving packages between jobs
// cannot silently put the expensive corpus suite back into every PR's matrix.
func TestTrafficWorkflowIsolation(t *testing.T) {
	type workflow struct {
		On map[string]struct {
			Paths []string `yaml:"paths"`
		} `yaml:"on"`
		Jobs map[string]struct {
			If       string `yaml:"if"`
			Strategy struct {
				Matrix struct {
					Include []struct {
						Configs string `yaml:"test_configs"`
					} `yaml:"include"`
				} `yaml:"matrix"`
			} `yaml:"strategy"`
			Steps []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	read := func(name string) workflow {
		t.Helper()
		b, err := os.ReadFile("../../.github/workflows/" + name + ".yml")
		if err != nil {
			t.Fatal(err)
		}
		var w workflow
		if err := yaml.Unmarshal(b, &w); err != nil {
			t.Fatal(err)
		}
		return w
	}
	essential := read("essential-tests")
	for jobName, job := range essential.Jobs {
		for _, suite := range job.Strategy.Matrix.Include {
			if suite.Configs == "" {
				continue
			}
			var configs []struct{ Package string }
			if err := json.Unmarshal([]byte(suite.Configs), &configs); err != nil {
				t.Fatalf("%s: %v", jobName, err)
			}
			for _, config := range configs {
				if strings.HasPrefix(config.Package, "./common/bin-parser") || strings.HasPrefix(config.Package, "./common/pcapx") {
					t.Errorf("unconditional suite %s contains traffic tests: %s", jobName, config.Package)
				}
			}
		}
	}
	traffic := read("bin-parser-tests")
	paths := traffic.On["pull_request"].Paths
	if len(paths) != 1 || paths[0] != "common/bin-parser/**" {
		t.Fatalf("automatic traffic scope must be bin-parser only: %v", paths)
	}
	if _, ok := traffic.On["workflow_dispatch"]; !ok {
		t.Fatal("traffic suite must remain available for explicit manual validation")
	}
	job, ok := traffic.Jobs["traffic-tests"]
	if !ok {
		t.Fatal("missing traffic test job")
	}
	for _, guard := range []string{"workflow_dispatch", "!github.event.pull_request.draft", "'wip'", "'work in progress'", "'do-not-merge'", "'rfc'"} {
		if !strings.Contains(job.If, guard) {
			t.Errorf("missing traffic job opt-in/WIP guard: %s", guard)
		}
	}
	var run string
	for _, step := range job.Steps {
		run += step.Run + "\n"
	}
	if !strings.Contains(run, "go test -count=1 -timeout=5m ./common/bin-parser/... ./common/pcapx/...") {
		t.Fatal("isolated traffic job must run complete parser and pcapx packages, including capture CLI")
	}
}
