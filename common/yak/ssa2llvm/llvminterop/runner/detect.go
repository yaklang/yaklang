package runner

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// VersionInfo holds the detected LLVM toolchain version information.
type VersionInfo struct {
	// Major is the LLVM major version (e.g. 14, 15, 16, 17, 18).
	Major int

	// Minor is the LLVM minor version.
	Minor int

	// Patch is the LLVM patch version.
	Patch int

	// RawOutput is the full version string from `opt --version`.
	RawOutput string

	// OptPath is the resolved path to the opt binary.
	OptPath string
}

func (v *VersionInfo) String() string {
	return fmt.Sprintf("LLVM %d.%d.%d (opt: %s)", v.Major, v.Minor, v.Patch, v.OptPath)
}

// DetectVersion probes the LLVM toolchain by running `opt --version` and
// parsing the output. If optBinary is empty, "opt" is resolved via PATH.
func DetectVersion(optBinary string) (*VersionInfo, error) {
	if optBinary == "" {
		optBinary = "opt"
	}

	resolvedPath, err := exec.LookPath(optBinary)
	if err != nil {
		return nil, fmt.Errorf("llvm detect: %q not found in PATH: %w", optBinary, err)
	}

	cmd := exec.Command(resolvedPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("llvm detect: %q --version failed: %w\nOutput: %s", resolvedPath, err, output)
	}

	rawOutput := string(output)
	info := &VersionInfo{
		RawOutput: rawOutput,
		OptPath:   resolvedPath,
	}

	if err := parseVersion(rawOutput, info); err != nil {
		return info, err
	}

	return info, nil
}

var versionRegex = regexp.MustCompile(`LLVM version (\d+)\.(\d+)\.(\d+)`)

func parseVersion(output string, info *VersionInfo) error {
	matches := versionRegex.FindStringSubmatch(output)
	if len(matches) < 4 {
		return fmt.Errorf("llvm detect: could not parse version from output: %s", strings.TrimSpace(output))
	}

	fmt.Sscanf(matches[1], "%d", &info.Major)
	fmt.Sscanf(matches[2], "%d", &info.Minor)
	fmt.Sscanf(matches[3], "%d", &info.Patch)

	return nil
}

// Capabilities describes what the detected LLVM toolchain supports.
type Capabilities struct {
	// HasNewPM indicates new PassManager support (LLVM ≥ 13).
	HasNewPM bool

	// HasLegacyPM indicates legacy PassManager support (LLVM ≤ 16).
	HasLegacyPM bool

	// SupportsLoadPassPlugin indicates --load-pass-plugin is available.
	SupportsLoadPassPlugin bool
}

// DetectCapabilities returns the feature set of the given LLVM version.
func DetectCapabilities(version *VersionInfo) *Capabilities {
	if version == nil {
		return &Capabilities{}
	}

	return &Capabilities{
		HasNewPM:               version.Major >= 13,
		HasLegacyPM:            version.Major <= 16,
		SupportsLoadPassPlugin: version.Major >= 13,
	}
}
