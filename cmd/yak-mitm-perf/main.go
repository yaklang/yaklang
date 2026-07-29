package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "capture":
		err = runCapture(os.Args[2:])
	case "compare":
		err = runCompare(os.Args[2:])
	case "help", "-h", "--help":
		printUsage()
		return
	default:
		printUsage()
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "yak-mitm-perf: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, `Usage:
  go run ./cmd/yak-mitm-perf capture [options]
  go run ./cmd/yak-mitm-perf compare -baseline before.json -candidate after.json [options]

Capture profiles:
  smoke     Fast, one low-resource repetition. Suitable before every local change.
  standard  Three repetitions and a larger seeded DB. Suitable before/after a PR.
  stress    Explicit higher load. Still capped by -gomaxprocs and -memory-limit.

Run "capture -h" or "compare -h" for command options.`)
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}
