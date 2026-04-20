//go:build hids && linux

package runtime

import (
	"context"
	"strings"

	"github.com/yaklang/yaklang/common/hids/model"
	"github.com/yaklang/yaklang/common/hids/rule"
	"github.com/yaklang/yaklang/common/yak"
)

type pipeline struct {
	engine         *rule.Engine
	scanSandbox    *yak.Sandbox
	alerts         chan model.Alert
	observations   chan model.Event
	evidencePolicy model.EvidencePolicy
	processes      *processTracker
	networks       *networkTracker
	files          *fileTracker
	artifacts      *artifactEnricher
}

func newPipeline(engine *rule.Engine) *pipeline {
	return &pipeline{
		engine:       engine,
		scanSandbox:  rule.NewSandbox(),
		alerts:       make(chan model.Alert, 64),
		observations: make(chan model.Event, runtimeObservationBufferSize),
		processes:    newProcessTracker(),
		networks:     newNetworkTracker(),
		files:        newFileTracker(),
	}
}

func (p *pipeline) withArtifactEnricher(enricher *artifactEnricher) *pipeline {
	if p == nil {
		return nil
	}
	p.artifacts = enricher
	return p
}

func (p *pipeline) withEvidencePolicy(policy model.EvidencePolicy) *pipeline {
	if p == nil {
		return nil
	}
	p.evidencePolicy = policy
	return p
}

func (p *pipeline) Run(ctx context.Context, events <-chan model.Event) {
	defer close(p.alerts)
	defer close(p.observations)

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			event = p.prepareEvent(event)
			if !shouldPublishObservation(event) {
				continue
			}
			select {
			case p.observations <- cloneEvent(event):
			case <-ctx.Done():
				return
			default:
			}
			if p.engine == nil || !shouldEvaluateRules(event) {
				continue
			}
			for _, alert := range p.engine.Evaluate(event) {
				alert = p.enrichAlertEvidence(event, alert)
				select {
				case p.alerts <- alert:
				case <-ctx.Done():
					return
				default:
				}
			}
		}
	}
}

func (p *pipeline) prepareEvent(event model.Event) model.Event {
	if p == nil {
		return event
	}
	if p.processes != nil {
		event = p.processes.Apply(event)
	}
	if p.networks != nil {
		event = p.networks.Apply(event)
	}
	if p.files != nil {
		event = p.files.Apply(event)
	}
	if p.artifacts != nil {
		event = p.artifacts.Apply(event)
	}
	return event
}

func (p *pipeline) Alerts() <-chan model.Alert {
	return p.alerts
}

func (p *pipeline) Observations() <-chan model.Event {
	return p.observations
}

func cloneEvent(event model.Event) model.Event {
	cloned := event
	cloned.Tags = cloneStringSlice(event.Tags)
	cloned.Labels = cloneStringMap(event.Labels)
	cloned.Data = cloneAnyMap(event.Data)
	if event.Process != nil {
		process := *event.Process
		process.Artifact = model.CloneArtifact(event.Process.Artifact)
		cloned.Process = &process
	}
	if event.Network != nil {
		network := *event.Network
		cloned.Network = &network
	}
	if event.File != nil {
		file := *event.File
		file.Artifact = model.CloneArtifact(event.File.Artifact)
		cloned.File = &file
	}
	if event.Audit != nil {
		audit := *event.Audit
		audit.RecordTypes = cloneStringSlice(event.Audit.RecordTypes)
		cloned.Audit = &audit
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneAnyMap(values map[string]any) map[string]any {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func shouldEvaluateRules(event model.Event) bool {
	for _, tag := range event.Tags {
		if strings.EqualFold(strings.TrimSpace(tag), "inventory") {
			return false
		}
	}
	return true
}

func shouldPublishObservation(event model.Event) bool {
	switch event.Type {
	case model.EventTypeNetworkAccept, model.EventTypeNetworkClose, model.EventTypeNetworkState:
		if event.Network == nil {
			return false
		}
		if strings.TrimSpace(event.Network.Protocol) == "" &&
			strings.TrimSpace(event.Network.DestAddress) == "" &&
			event.Network.DestPort == 0 {
			return false
		}
	}
	return true
}
