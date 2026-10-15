package asyncexecmanager

// Config bounds active work and retained records. Zero values select defaults.
// There is no worker queue: capacity exhaustion is reported at submission.
type Config struct {
	MaxConcurrent int
	MaxExecutions int
}
