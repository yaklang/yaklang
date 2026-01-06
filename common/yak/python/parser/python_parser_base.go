package pythonparser

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
)

// PythonParserBase provides base functionality for the Python parser,
// handling Python version detection and checking.
// This matches the Java implementation: PythonParserBase.java
type PythonParserBase struct {
	*antlr.BaseParser

	// Version specifies the Python version to use for parsing.
	// Default is PythonVersionAutodetect.
	// This is public (uppercase) to match Java's public field.
	Version PythonVersion
}

// NewPythonParserBase creates a new PythonParserBase instance.
// This matches the Java constructor: protected PythonParserBase(TokenStream input)
func NewPythonParserBase(input antlr.TokenStream) *PythonParserBase {
	base := &PythonParserBase{
		BaseParser: antlr.NewBaseParser(input),
		Version:     PythonVersionAutodetect,
	}
	return base
}

// CheckVersion checks if the given version matches the configured version.
// Returns true if Version is Autodetect or if the version matches.
// This matches the Java method: protected boolean CheckVersion(int version)
func (p *PythonParserBase) CheckVersion(version int) bool {
	return p.Version == PythonVersionAutodetect || version == p.Version.GetValue()
}

// SetVersion sets the Python version based on the required version number.
// This matches the Java method: protected void SetVersion(int requiredVersion)
func (p *PythonParserBase) SetVersion(requiredVersion int) {
	if requiredVersion == 2 {
		p.Version = PythonVersion2
	} else if requiredVersion == 3 {
		p.Version = PythonVersion3
	}
}

