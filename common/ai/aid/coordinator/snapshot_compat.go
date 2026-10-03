package coordinator

import (
	"encoding/json"
	"fmt"
)

// Old persisted snapshots are translated only here. Runtime and model schemas
// contain no plan versions or parallel draft/approved plans.
func (s *Snapshot) UnmarshalJSON(data []byte) error {
	type current Snapshot
	var header struct {
		Schema int `json:"schema"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return err
	}
	if header.Schema != 1 {
		var v current
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*s = Snapshot(v)
		return nil
	}
	var old struct {
		Revision         uint64             `json:"revision"`
		DraftVersion     uint64             `json:"draft_version"`
		ApprovedVersion  uint64             `json:"approved_version"`
		SubmittedVersion uint64             `json:"submitted_version"`
		Draft            *Plan              `json:"draft"`
		Approved         *Plan              `json:"approved"`
		Attempts         map[string]Attempt `json:"attempts"`
		NextAttempt      uint64             `json:"next_attempt"`
		UserRevision     uint64             `json:"user_revision"`
		Finished         bool               `json:"finished"`
	}
	if err := json.Unmarshal(data, &old); err != nil {
		return err
	}
	if old.DraftVersion == 0 || old.ApprovedVersion > old.DraftVersion || old.SubmittedVersion > old.DraftVersion {
		return fmt.Errorf("invalid legacy plan snapshot")
	}
	phase, p := PhasePlan, old.Draft
	if old.Approved != nil {
		if old.ApprovedVersion != old.DraftVersion {
			return fmt.Errorf("legacy snapshot has an executing replacement draft; start a new coordinator after cancelling its tasks")
		}
		phase, p = PhaseExec, old.Approved
	}
	if p == nil {
		return fmt.Errorf("legacy snapshot has no plan")
	}
	// Re-derive the DAG and discard frontend state fields at this boundary.
	plan, err := ParsePlan(string(p.Tree), p.Document, p)
	if err != nil {
		return err
	}
	*s = Snapshot{Schema: 2, Phase: phase, Plan: plan, ReviewPending: phase == PhasePlan && old.SubmittedVersion > 0, Revision: old.Revision, Attempts: old.Attempts, NextAttempt: old.NextAttempt, UserRevision: old.UserRevision, Finished: old.Finished}
	return nil
}
