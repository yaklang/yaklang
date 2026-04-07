// Package plugin defines types for referencing LLVM pass plugins and tools.
package plugin

// Kind classifies how an external LLVM extension is loaded.
type Kind int

const (
	// KindNewPM is a new-PassManager plugin (.so) loaded via --load-pass-plugin.
	KindNewPM Kind = iota

	// KindLegacy is a legacy-PassManager plugin (.so) loaded via -load.
	KindLegacy

	// KindTool is a standalone tool/adapter invoked as a subprocess.
	KindTool
)

func (k Kind) String() string {
	switch k {
	case KindNewPM:
		return "new-pm"
	case KindLegacy:
		return "legacy"
	case KindTool:
		return "tool"
	default:
		return "unknown"
	}
}

// Descriptor describes a single external LLVM plugin or tool.
type Descriptor struct {
	// Name is a short human-readable identifier (e.g. "ollvm-flattening").
	Name string

	// Kind classifies the plugin type.
	Kind Kind

	// Path is the filesystem path to the .so plugin or tool binary.
	Path string

	// Passes lists the textual pass pipeline to run (new PM syntax, e.g.
	// "function(flatten),function(bcf)"). For KindTool this may be empty
	// if the tool uses its own CLI.
	Passes []string

	// Args are additional CLI arguments passed to opt or the tool.
	Args []string
}

// Validate checks that the descriptor has the minimum required fields.
func (d *Descriptor) Validate() error {
	if d.Name == "" {
		return &ValidationError{Field: "Name", Reason: "must not be empty"}
	}
	if d.Path == "" {
		return &ValidationError{Field: "Path", Reason: "must not be empty"}
	}
	return nil
}

// ValidationError reports a missing or invalid field.
type ValidationError struct {
	Field  string
	Reason string
}

func (e *ValidationError) Error() string {
	return "plugin descriptor: " + e.Field + " " + e.Reason
}
