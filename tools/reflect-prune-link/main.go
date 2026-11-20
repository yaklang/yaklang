// Command reflect-prune-link builds and wraps a non-invasive patched Go linker
// (via `go build -overlay` + `-toolexec`) to selectively prune unused
// reflect-only exported methods on internal packages during release builds.
//
// It never modifies GOROOT and works on Linux (including read-only GOROOT in
// manylinux2014 containers), macOS, and Windows.
//
// Usage:
//
//	# 1. Build the patched Go linker once:
//	reflect-prune-link prepare -o /tmp/yak-link
//
//	# 2. Pass as -toolexec to `go build`:
//	go build -toolexec="/tmp/reflect-prune-link exec --link /tmp/yak-link --profile tier2 --" ...
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "prepare":
		if err := runPrepare(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "reflect-prune-link prepare: %v\n", err)
			os.Exit(1)
		}
	case "exec":
		os.Exit(runExec(os.Args[2:]))
	case "profiles":
		for _, name := range []string{"tier1", "tier2"} {
			fmt.Printf("%s:\n", name)
			for _, p := range DefaultProfiles[name] {
				fmt.Printf("  %s\n", p)
			}
		}
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  reflect-prune-link prepare -o <patched-link-path>")
	fmt.Fprintln(os.Stderr, "  reflect-prune-link exec --link <patched-link-path> [--profile tier2] [--verbose] -- <tool> <args...>")
	fmt.Fprintln(os.Stderr, "  reflect-prune-link profiles")
}

func goEnv(key string) (string, error) {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return "", fmt.Errorf("go env %s: %w", key, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func runPrepare(args []string) error {
	fs := flag.NewFlagSet("prepare", flag.ContinueOnError)
	outPath := fs.String("o", "", "output path for the patched Go linker binary")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*outPath) == "" {
		return errors.New("-o <patched-link-path> is required")
	}
	absOut, err := filepath.Abs(*outPath)
	if err != nil {
		return err
	}

	goroot, err := goEnv("GOROOT")
	if err != nil {
		return err
	}
	hostOS, err := goEnv("GOHOSTOS")
	if err != nil {
		return err
	}
	hostArch, err := goEnv("GOHOSTARCH")
	if err != nil {
		return err
	}

	deadcodePath := filepath.Join(goroot, "src", "cmd", "link", "internal", "ld", "deadcode.go")
	orig, err := os.ReadFile(deadcodePath)
	if err != nil {
		return fmt.Errorf("read %s: %w", deadcodePath, err)
	}
	patched, err := PatchDeadcodeSource(orig)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "yak-reflect-prune-overlay-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	patchedFile := filepath.Join(tmpDir, "deadcode.go")
	if err := os.WriteFile(patchedFile, patched, 0o600); err != nil {
		return err
	}

	overlay := struct {
		Replace map[string]string `json:"Replace"`
	}{
		Replace: map[string]string{
			deadcodePath: patchedFile,
		},
	}
	overlayBytes, err := json.Marshal(overlay)
	if err != nil {
		return err
	}
	overlayFile := filepath.Join(tmpDir, "overlay.json")
	if err := os.WriteFile(overlayFile, overlayBytes, 0o600); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(absOut), 0o755); err != nil {
		return err
	}

	cmd := exec.Command("go", "build", "-overlay="+overlayFile, "-o", absOut, "cmd/link")
	cmd.Dir = filepath.Join(goroot, "src")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS="+hostOS,
		"GOARCH="+hostArch,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build patched cmd/link: %w", err)
	}
	return nil
}

func runExec(args []string) int {
	// Parse wrapper flags up to "--" (or first non-flag argument).
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	linkPath := fs.String("link", "", "path to the patched Go linker binary")
	profile := fs.String("profile", "tier2", "built-in reflect-prune profile (off, tier1, tier2)")
	verbose := fs.Bool("verbose", false, "print pruned method summary to stderr")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "reflect-prune-link exec: %v\n", err)
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fmt.Fprintln(os.Stderr, "reflect-prune-link exec: missing tool invocation")
		return 2
	}

	tool := rest[0]
	toolArgs := rest[1:]
	targetTool := tool
	env := os.Environ()

	base := strings.ToLower(filepath.Base(tool))
	isLinker := base == "link" || base == "link.exe"
	if isLinker && *linkPath != "" && !isVersionProbe(toolArgs) {
		// Respect explicit caller override in YAK_REFLECT_PRUNE_EXCLUDE / FILE;
		// otherwise resolve from --profile.
		hasExplicit := strings.TrimSpace(os.Getenv("YAK_REFLECT_PRUNE_EXCLUDE")) != "" ||
			strings.TrimSpace(os.Getenv("YAK_REFLECT_PRUNE_EXCLUDE_FILE")) != ""
		if hasExplicit {
			targetTool = *linkPath
		} else {
			patterns, err := ResolveProfilePatterns(*profile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "reflect-prune-link exec: %v\n", err)
				return 2
			}
			if patterns != "" {
				targetTool = *linkPath
				env = append(env, "YAK_REFLECT_PRUNE_EXCLUDE="+patterns)
			}
		}
		if *verbose {
			env = append(env, "YAK_REFLECT_PRUNE_VERBOSE=1")
		}
	}

	cmd := exec.Command(targetTool, toolArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "reflect-prune-link exec: run %s: %v\n", targetTool, err)
		return 1
	}
	return 0
}

func isVersionProbe(args []string) bool {
	for _, a := range args {
		if a == "-V" || strings.HasPrefix(a, "-V=") {
			return true
		}
	}
	return false
}
