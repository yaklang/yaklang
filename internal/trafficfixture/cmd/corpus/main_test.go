package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"testing"
)

func TestCorpusChild(t *testing.T) {
	if os.Getenv("YAK_CORPUS_TEST_CHILD") == "1" {
		os.Exit(7)
	}
}

func TestCorpusExecPropagatesFailureAndCleansWorkspace(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	t.Setenv("TMP", temporary)
	t.Setenv("TEMP", temporary)
	t.Setenv("YAK_CORPUS_TEST_CHILD", "1")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	err = execute(context.Background(), []string{"exec", "--", binary, "-test.run=^TestCorpusChild$"})
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 7 {
		t.Fatalf("lost child failure: %v", err)
	}
	entries, err := os.ReadDir(temporary)
	if err != nil || len(entries) != 0 {
		t.Fatalf("temporary workspace leaked: %v, %v", entries, err)
	}
}

func TestCorpusExecRejectsEscapingArgument(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("YAK_CORPUS_TEST_CHILD", "1")
	err = execute(context.Background(), []string{"exec", "--", binary, "@../go.mod"})
	var exit *exec.ExitError
	if err == nil || errors.As(err, &exit) {
		t.Fatalf("unsafe argument reached the child: %v", err)
	}
}
