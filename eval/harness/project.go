package harness

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// EnsureProject ensures a vulnerable project is available on disk.
// If projectPath is provided, it validates the directory exists.
// Otherwise, it clones projectUrl into projects/<cve_id>/ and checks out commitHash.
func EnsureProject(projectPath, projectUrl, commitHash, cveID string) (string, error) {
	if projectPath != "" {
		abs, err := filepath.Abs(projectPath)
		if err != nil {
			return "", fmt.Errorf("resolve project path %q: %w", projectPath, err)
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("project path %q does not exist: %w", abs, err)
		}
		return abs, nil
	}

	if projectUrl == "" {
		return "", fmt.Errorf("project path is empty and ground truth has no project_url")
	}
	if cveID == "" {
		return "", fmt.Errorf("project path is empty and no CVE ID available")
	}

	// Default location: projects/<cve_id>
	baseDir, err := getProjectsBaseDir()
	if err != nil {
		return "", err
	}
	targetDir := filepath.Join(baseDir, safeDirName(cveID))

	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir %q: %w", baseDir, err)
	}

	// Clone if not exists.
	gitDir := filepath.Join(targetDir, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		fmt.Printf("[project] Cloning %s into %s\n", projectUrl, targetDir)
		cmd := exec.Command("git", "clone", projectUrl, targetDir)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("git clone %s: %w", projectUrl, err)
		}
	} else {
		fmt.Printf("[project] Existing repo found at %s\n", targetDir)
	}

	// Fetch and checkout the vulnerable commit.
	if commitHash != "" {
		fmt.Printf("[project] Checking out commit %s\n", commitHash)
		fetchCmd := exec.Command("git", "-C", targetDir, "fetch", "origin", commitHash)
		fetchCmd.Stdout = os.Stdout
		fetchCmd.Stderr = os.Stderr
		_ = fetchCmd.Run() // may already be present; ignore error

		checkoutCmd := exec.Command("git", "-C", targetDir, "checkout", commitHash)
		checkoutCmd.Stdout = os.Stdout
		checkoutCmd.Stderr = os.Stderr
		if err := checkoutCmd.Run(); err != nil {
			return "", fmt.Errorf("git checkout %s in %s: %w", commitHash, targetDir, err)
		}
	}

	return targetDir, nil
}

// getProjectsBaseDir returns the absolute path to eval/projects.
func getProjectsBaseDir() (string, error) {
	// Try to locate relative to this source file via working directory.
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	// When running from yaklang_engine/eval, projects is ./projects.
	candidate := filepath.Join(wd, "projects")
	if _, err := os.Stat(candidate); err == nil {
		abs, err := filepath.Abs(candidate)
		return abs, err
	}
	// Fallback: derive from executable path.
	ex, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("executable path: %w", err)
	}
	exDir := filepath.Dir(ex)
	candidate = filepath.Join(exDir, "projects")
	abs, err := filepath.Abs(candidate)
	return abs, err
}

// safeDirName sanitizes a CVE ID for use as a directory name.
func safeDirName(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(name, "/", "_"), "\\", "_")
}
