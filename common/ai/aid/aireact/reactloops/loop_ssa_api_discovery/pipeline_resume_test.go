package loop_ssa_api_discovery

import "testing"

func TestResumeStageFromSessionPhase(t *testing.T) {
	cases := map[string]int{
		PhaseInitialized:        1,
		PhaseApiVerified:        2,
		PhaseVulnScanned:        4,
		PhaseVulnVerified:       5,
		PhasePipelineReport:     6,
		PhaseStep1AuthDone:      4,
		"unknown_phase":         1,
	}
	for phase, want := range cases {
		if got := ResumeStageFromSessionPhase(phase); got != want {
			t.Fatalf("phase %q: got %d want %d", phase, got, want)
		}
	}
}

func TestParsePipelineResume(t *testing.T) {
	in := "Code path: /tmp/x\nSession UUID: abc\nPipeline resume: yes\nPipeline resume from stage: 3\n"
	parsed, err := ParseUserInputLenient(in)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.PipelineResume {
		t.Fatal("expected PipelineResume")
	}
	if parsed.PipelineResumeFromStage != 3 {
		t.Fatalf("stage=%d", parsed.PipelineResumeFromStage)
	}
}

func TestResolvePipelineStartStageExplicit(t *testing.T) {
	parsed := &ParsedUserInput{PipelineResumeFromStage: 4}
	got := ResolvePipelineStartStage(parsed, nil)
	if got != 4 {
		t.Fatalf("got %d", got)
	}
}
