// Package verify provides validation helpers that check whether an
// external LLVM pass produced valid output and diagnose common failures.
package verify

import (
	"fmt"
	"os"
	"strings"
)

// Result captures the outcome of a verification check.
type Result struct {
	// Valid is true if the output passed all checks.
	Valid bool

	// Errors lists any validation failures.
	Errors []string
}

// CheckOutputFile verifies that the LLVM tool produced a valid output file.
func CheckOutputFile(path string) *Result {
	result := &Result{Valid: true}

	info, err := os.Stat(path)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, fmt.Sprintf("output file not found: %v", err))
		return result
	}

	if info.IsDir() {
		result.Valid = false
		result.Errors = append(result.Errors, "output path is a directory, not a file")
		return result
	}

	if info.Size() == 0 {
		result.Valid = false
		result.Errors = append(result.Errors, "output file is empty")
		return result
	}

	return result
}

// CheckIRValidity performs a lightweight check on LLVM IR text content.
// It does not replace `opt -verify` but catches common gross failures.
func CheckIRValidity(ir string) *Result {
	result := &Result{Valid: true}

	if strings.TrimSpace(ir) == "" {
		result.Valid = false
		result.Errors = append(result.Errors, "IR is empty")
		return result
	}

	// Check for at least one function definition or declaration.
	if !strings.Contains(ir, "define ") && !strings.Contains(ir, "declare ") {
		result.Valid = false
		result.Errors = append(result.Errors, "IR contains no function definitions or declarations")
	}

	return result
}

// DiagnoseFailure examines the stderr output from an LLVM tool and
// returns human-readable diagnostic messages.
func DiagnoseFailure(stderr string, exitCode int) []string {
	var diagnostics []string

	if exitCode != 0 {
		diagnostics = append(diagnostics, fmt.Sprintf("LLVM tool exited with code %d", exitCode))
	}

	if strings.Contains(stderr, "error: unable to open input file") {
		diagnostics = append(diagnostics, "Input file could not be opened by the LLVM tool")
	}

	if strings.Contains(stderr, "Cannot register pass") || strings.Contains(stderr, "Unable to find") {
		diagnostics = append(diagnostics, "LLVM pass plugin could not be loaded (check path and LLVM version compatibility)")
	}

	if strings.Contains(stderr, "LLVM ERROR") {
		// Extract the error message.
		for _, line := range strings.Split(stderr, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "LLVM ERROR:") {
				diagnostics = append(diagnostics, strings.TrimSpace(line))
			}
		}
	}

	if strings.Contains(stderr, "Assertion") || strings.Contains(stderr, "UNREACHABLE") {
		diagnostics = append(diagnostics, "LLVM internal assertion failure (likely plugin incompatibility)")
	}

	if len(diagnostics) == 0 && exitCode != 0 {
		diagnostics = append(diagnostics, "Unknown failure; check full stderr output")
	}

	return diagnostics
}
