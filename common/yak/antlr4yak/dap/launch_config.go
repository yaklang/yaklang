package dap

import (
	"encoding/json"
	"fmt"
)

// LaunchConfig is the collection of launch request attributes recognized by DAP implementation.
type LaunchConfig struct {
	// Acceptable values are:
	//   "debug":
	//   "exec": executes a yak script and begins a debug session.
	// Default is "exec".
	Mode string `json:"mode,omitempty"`

	// Path to the program folder (or any go file within that folder)
	// when in `debug` or `test` mode, and to the pre-built binary file
	// to debug in `exec` mode.
	Program string `json:"program,omitempty"`

	// Command line arguments passed to the debugged program.
	// Relative paths used in Args will be interpreted as paths relative
	// to `cwd`.
	Args []string `json:"args,omitempty"`

	// Working directory of the program being debugged.
	// If a relative path is provided, it will be interpreted as
	// a relative path to Delve's working directory. This is
	Cwd string `json:"cwd,omitempty"`

	// NoDebug is used to run the program without debugging.
	NoDebug bool `json:"noDebug,omitempty"`

	// Env specifies optional environment variables for Delve server
	// in addition to the environment variables Delve initially
	// started with.
	// Variables with 'nil' values can be used to unset the named
	// environment variables.
	// Values are interpreted verbatim. Variable substitution or
	// reference to other environment variables is not supported.
	Env map[string]*string `json:"env,omitempty"`

	// The output mode specifies how to handle the program's output.
	OutputMode string `json:"outputMode,omitempty"`

	// Automatically stop program after launch or attach.
	StopOnEntry bool `json:"stopOnEntry,omitempty"`

	// StackTraceDepth is the maximum length of the returned list of stack frames.
	StackTraceDepth int `cfgName:"stackTraceDepth"`
}

var defaultArgs = LaunchConfig{
	StopOnEntry:     false,
	StackTraceDepth: 50,
}

func unmarshalLaunchConfig(input json.RawMessage, config any) error {
	if err := json.Unmarshal(input, config); err != nil {
		if uerr, ok := err.(*json.UnmarshalTypeError); ok {
			// Format json.UnmarshalTypeError error string in our own way. E.g.,
			//   "json: cannot unmarshal number into Go struct field LaunchArgs.substitutePath of type dap.SubstitutePath"
			//   => "cannot unmarshal number into 'substitutePath' of type {from:string, to:string}"
			//   "json: cannot unmarshal number into Go struct field LaunchArgs.program of type string" (go1.16)
			//   => "cannot unmarshal number into 'program' of type string"
			typ := uerr.Type.String()
			if uerr.Field == "substitutePath" {
				typ = `{"from":string, "to":string}`
			}
			return fmt.Errorf("cannot unmarshal %v into %q of type %v", uerr.Value, uerr.Field, typ)
		}
		return err
	}
	return nil
}
