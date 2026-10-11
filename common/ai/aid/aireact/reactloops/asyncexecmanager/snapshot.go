package asyncexecmanager

type Snapshot struct {
	Summary
	Observation Observation `json:"observation"`
}
