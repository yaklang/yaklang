package asyncexecmanager

// Config bounds active work and total retained records. Zero values select
// defaults. MaxActiveExecutions limits pending, running and stopping executions
// and is the only limit that can reject admission. MaxRetainedExecutions limits
// active plus terminal records; when it is full, the oldest terminal record is
// evicted before a new execution is registered. MaxRetainedExecutions must be at
// least MaxActiveExecutions.
type Config struct {
	MaxActiveExecutions   int
	MaxRetainedExecutions int
}
