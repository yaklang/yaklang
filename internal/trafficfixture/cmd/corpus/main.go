// Command corpus verifies and runs archived traffic fixture tools.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

func run(ctx context.Context, directory, source string, args ...string) error {
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = directory
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1", "YAK_TRAFFIC_REPO_ROOT="+source, "GOTOOLCHAIN=local")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func execute(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: corpus verify | list | export NEW_DIRECTORY | exec [--workspace] -- COMMAND [@ARCHIVED_PATH ...] | test")
	}
	source, err := trafficfixture.RepositoryRoot()
	if err != nil {
		return err
	}
	switch args[0] {
	case "verify":
		return trafficfixture.Verify()
	case "list":
		names, err := trafficfixture.Names()
		if err != nil {
			return err
		}
		for _, name := range names {
			fmt.Println(name)
		}
		return nil
	case "export":
		if len(args) != 2 {
			return fmt.Errorf("export requires a new output directory")
		}
		if err := os.Mkdir(args[1], 0700); err != nil {
			return err
		}
		if err := trafficfixture.Export(args[1]); err != nil {
			os.RemoveAll(args[1]) // Only the new directory created above is owned here.
			return err
		}
		return nil
	case "exec", "test":
	default:
		return fmt.Errorf("unknown corpus command %q", args[0])
	}
	workspace, err := os.MkdirTemp("", "yak-traffic-tools-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(workspace)
	if err := trafficfixture.Export(workspace); err != nil {
		return err
	}
	if args[0] == "test" {
		// Compile/validate archived sources and exercise their unit tests. Never
		// invoke capture generators, Docker, downloads, or privileged tools.
		if err := run(ctx, workspace, source, "python3", "-c", "import ast,pathlib; files=list(pathlib.Path('.').rglob('*.py')); [ast.parse(p.read_bytes(),filename=str(p)) for p in files]; print('Python syntax checked:',len(files))"); err != nil {
			return err
		}
		err := filepath.WalkDir(workspace, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			switch filepath.Ext(path) {
			case ".sh":
				return run(ctx, workspace, source, "bash", "-n", path)
			case ".mjs":
				return run(ctx, workspace, source, "node", "--check", path)
			}
			return nil
		})
		if err != nil {
			return err
		}
		commands := [][]string{
			{"python3", "-m", "unittest", "discover", "-s", "scripts/protocol-tests", "-p", "test_*.py"},
			{"node", "--test", "common/bin-parser/testdata/protocol-corpus/tools/audit-pr5023-reproduction.test.mjs"},
			{"go", "test", "-v", "-count=1", "-timeout=60s", "./common/bin-parser/testdata/protocol-corpus/tools/generate", "./scripts/protocol-tests/generate-m1"},
			// These standalone recipes intentionally use go:build ignore.
			{"go", "test", "-count=1", "-timeout=60s", "scripts/protocol-tests/generate-m2b/mysql_client.go"},
			{"go", "test", "-count=1", "-timeout=60s", "scripts/protocol-tests/generate-reliability/tls.go"},
		}
		for _, command := range commands {
			if err := run(ctx, workspace, source, command...); err != nil {
				return err
			}
		}
		return nil
	}
	command := args[1:]
	directory, err := os.Getwd()
	if err != nil {
		return err
	}
	if len(command) > 0 && command[0] == "--workspace" {
		directory, command = workspace, command[1:]
	}
	if len(command) == 0 || command[0] != "--" || len(command) < 2 {
		return fmt.Errorf("exec requires -- COMMAND; @path arguments refer to verified workspace files")
	}
	command = append([]string(nil), command[1:]...)
	for i, arg := range command {
		if !strings.HasPrefix(arg, "@") {
			continue
		}
		name := strings.TrimPrefix(arg, "@")
		if !fs.ValidPath(name) || strings.ContainsAny(name, "\\:") {
			return fmt.Errorf("invalid archived argument %q", arg)
		}
		command[i] = filepath.Join(workspace, filepath.FromSlash(name))
		if _, err := os.Stat(command[i]); err != nil {
			return err
		}
	}
	return run(ctx, directory, source, command...)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := execute(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() > 0 {
			os.Exit(exit.ExitCode())
		}
		os.Exit(1)
	}
}
