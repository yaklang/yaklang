package loop_ssa_api_discovery

import (
	"context"
	"testing"
	"time"
)

func TestPhase1ReactMaxDuration_Default(t *testing.T) {
	t.Setenv("YAK_SSA_API_DISCOVERY_PHASE1_REACT_TIMEOUT", "")
	if got := phase1ReactMaxDuration(); got != 90*time.Minute {
		t.Fatalf("default duration: got %v want %v", got, 90*time.Minute)
	}
}

func TestPhase1ReactMaxDuration_Parse(t *testing.T) {
	t.Setenv("YAK_SSA_API_DISCOVERY_PHASE1_REACT_TIMEOUT", "90m")
	if got := phase1ReactMaxDuration(); got != 90*time.Minute {
		t.Fatalf("parsed duration: got %v want %v", got, 90*time.Minute)
	}
}

func TestPhase1ReactMaxDuration_Floor(t *testing.T) {
	t.Setenv("YAK_SSA_API_DISCOVERY_PHASE1_REACT_TIMEOUT", "1m")
	if got := phase1ReactMaxDuration(); got != 5*time.Minute {
		t.Fatalf("floor duration: got %v want %v", got, 5*time.Minute)
	}
}

func TestDetachPhase1ReactContext_SurvivesParentCancel(t *testing.T) {
	parent, cancelParent := context.WithCancel(context.Background())
	detached, cancelDetached := detachPhase1ReactContext(parent)
	defer cancelDetached()

	cancelParent()
	select {
	case <-detached.Done():
		t.Fatal("detached context should not cancel when parent cancels")
	default:
	}
}
