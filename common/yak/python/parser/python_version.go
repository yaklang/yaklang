package pythonparser

// PythonVersion represents the Python version for parsing.
// This matches the Java enum: PythonVersion.java
type PythonVersion int

const (
	// PythonVersionAutodetect automatically detects the Python version
	// Matches: Autodetect(0)
	PythonVersionAutodetect PythonVersion = 0
	// PythonVersion2 represents Python 2
	// Matches: Python2(2)
	PythonVersion2 PythonVersion = 2
	// PythonVersion3 represents Python 3
	// Matches: Python3(3)
	PythonVersion3 PythonVersion = 3
)

// GetValue returns the integer value of the Python version.
// This matches the Java method: public int getValue()
func (v PythonVersion) GetValue() int {
	return int(v)
}

