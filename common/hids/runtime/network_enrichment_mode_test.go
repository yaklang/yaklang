//go:build hids && linux

package runtime

import (
	"testing"
	"time"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/rule"
)

func TestPipelineFromSpecUsesLightNetworkEnrichmentWhenSnapshotsAndRulesDisabled(t *testing.T) {
	t.Parallel()

	disabled := false
	engine, err := rule.NewEngine(model.DesiredSpec{})
	if err != nil {
		t.Fatalf("build empty rule engine: %v", err)
	}
	pipeline := newPipelineFromSpec(engine, model.DesiredSpec{
		Mode: model.ModeObserve,
		Collectors: model.Collectors{
			Network: model.CollectorSpec{
				Enabled: true,
				Backend: model.CollectorBackendEBPF,
			},
		},
		Reporting: model.ReportingPolicy{
			EmitSnapshotObservations: &disabled,
		},
	})

	event := pipeline.prepareEvent(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf.network",
		Timestamp: time.Unix(1_700_000_000, 0).UTC(),
		Process: &model.Process{
			PID:     123,
			Name:    "curl",
			Image:   "/usr/bin/curl",
			Command: "curl https://example.com",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			SourceAddress: "10.0.0.5",
			SourcePort:    41000,
			DestAddress:   "1.1.1.1",
			DestPort:      443,
		},
		Data: map[string]any{"fd": 7},
	})

	if event.Network == nil || event.Network.Direction != "outbound" {
		t.Fatalf("expected lightweight direction enrichment, got %#v", event.Network)
	}
	if _, exists := event.Data["dest_service"]; exists {
		t.Fatalf("did not expect detailed service enrichment in lightweight mode: %#v", event.Data)
	}
	if _, exists := event.Data["process_roles"]; exists {
		t.Fatalf("did not expect process role enrichment in lightweight mode: %#v", event.Data)
	}
	if _, exists := event.Data["connection_age_seconds"]; exists {
		t.Fatalf("did not expect lifecycle enrichment in lightweight mode: %#v", event.Data)
	}
}

func TestPipelineFromSpecKeepsDetailedNetworkEnrichmentWhenNetworkRuleEnabled(t *testing.T) {
	t.Parallel()

	disabled := false
	spec := model.DesiredSpec{
		Mode:            model.ModeObserve,
		BuiltinRuleSets: []string{"linux.network.baseline"},
		Collectors: model.Collectors{
			Network: model.CollectorSpec{
				Enabled: true,
				Backend: model.CollectorBackendEBPF,
			},
		},
		Reporting: model.ReportingPolicy{
			EmitSnapshotObservations: &disabled,
		},
	}
	engine, err := rule.NewEngine(spec)
	if err != nil {
		t.Fatalf("build network rule engine: %v", err)
	}
	pipeline := newPipelineFromSpec(engine, spec)

	event := pipeline.prepareEvent(model.Event{
		Type:      model.EventTypeNetworkConnect,
		Source:    "ebpf.network",
		Timestamp: time.Unix(1_700_000_000, 0).UTC(),
		Process: &model.Process{
			PID:     123,
			Name:    "curl",
			Image:   "/usr/bin/curl",
			Command: "curl https://example.com",
		},
		Network: &model.Network{
			Protocol:      "tcp",
			SourceAddress: "10.0.0.5",
			SourcePort:    41000,
			DestAddress:   "1.1.1.1",
			DestPort:      443,
		},
		Data: map[string]any{"fd": 7},
	})

	if got, _ := event.Data["dest_service"].(string); got != "https" {
		t.Fatalf("expected detailed service enrichment, got %#v", event.Data["dest_service"])
	}
	if _, exists := event.Data["connection_age_seconds"]; !exists {
		t.Fatalf("expected lifecycle enrichment: %#v", event.Data)
	}
	roles, ok := event.Data["process_roles"].([]string)
	if !ok {
		t.Fatalf("expected process roles enrichment: %#v", event.Data["process_roles"])
	}
	if len(roles) != 1 || roles[0] != "network_tool" {
		t.Fatalf("unexpected process roles: %#v", roles)
	}
}
