package ssaconfig

// DebugConfig controls pprof/debug output directory creation and periodic profiling.
type DebugConfig struct {
	// DebugDir is the output directory for debug artifacts. When set, the scan
	// will create a structured directory inside it with:
	//   <DebugDir>/
	//   ├── ssadb.db          (SSA database)
	//   ├── log               (scan log file)
	//   ├── report            (scan report output)
	//   ├── cmd.txt           (saved launch command)
	//   ├── cpu-pprof/        (CPU pprof snapshots)
	//   ├── memory-pprof/     (Memory pprof snapshots)
	//   └── goroutine-pprof/  (Goroutine pprof snapshots)
	DebugDir string `json:"debug_dir"`
}

// --- Debug config Get/Set methods ---

func (c *Config) GetDebugDir() string {
	if c == nil {
		return ""
	}
	if vals, ok := c.GetExtraInfo("debug_dir"); ok && len(vals) > 0 {
		if s, ok := vals[0].(string); ok {
			return s
		}
	}
	return ""
}

func (c *Config) SetDebugDir(dir string) {
	if c == nil {
		return
	}
	c.SetExtraInfo("debug_dir", dir)
}

// --- Debug config Options ---

// WithDebugDir sets the debug/pprof output directory.
// When set, the scan creates a structured directory with ssadb.db, log, report,
// cmd.txt, and pprof subdirectories.
func WithDebugDir(dir string) Option {
	return func(c *Config) error {
		c.SetDebugDir(dir)
		return nil
	}
}
