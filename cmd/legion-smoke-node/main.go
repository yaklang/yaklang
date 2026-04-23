package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
	cli "github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils/diagnostics"
	"github.com/yaklang/yaklang/scannode"
)

func main() {
	if err := run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(args []string) error {
	if shouldRunDistYak(args) {
		return runDistYak(args)
	}
	return runNode(args[1:])
}

func shouldRunDistYak(args []string) bool {
	return len(args) > 1 && args[1] == scannode.DistYakCommand.Name
}

func runDistYak(args []string) error {
	app := cli.NewApp()
	app.Commands = []cli.Command{scannode.DistYakCommand}
	return app.Run(args)
}

func runNode(args []string) error {
	flags := flag.NewFlagSet("legion-smoke-node", flag.ContinueOnError)
	apiURL := flags.String("api-url", "http://127.0.0.1:8080", "Legion platform HTTP API base URL")
	enrollmentToken := flags.String("enrollment-token", "", "Legion node enrollment token")
	nodeID := flags.String("id", "", "Legacy node ID fallback; canonical node_id is assigned by platform")
	displayName := flags.String("name", "smoke-node", "Display name reported to Legion")
	agentInstallationID := flags.String("agent-installation-id", "", "Override persisted agent installation ID")
	baseDir := flags.String("base-dir", "", "Node local state base directory")
	version := flags.String("version", "smoke", "Node version")
	heartbeatInterval := flags.Duration(
		"heartbeat-interval",
		node.DefaultHeartbeatInterval,
		"Heartbeat interval",
	)
	pprofAddr := flags.String("pprof-addr", "", "Optional pprof HTTP listen address, e.g. 127.0.0.1:18080")
	heapMonitorInterval := flags.Duration(
		"heap-monitor-interval",
		0,
		"Optional heap monitor interval; zero disables periodic heap logging/dumps",
	)
	heapDumpThresholdMB := flags.Uint64(
		"heap-dump-threshold-mb",
		0,
		"Heap dump/log threshold in MB; zero means log every monitor interval",
	)
	heapDumpDir := flags.String(
		"heap-dump-dir",
		"",
		"Optional directory for periodic heap profile dumps; empty means log-only monitoring",
	)
	heapDumpCount := flags.Int(
		"heap-dump-count",
		0,
		"Optional maximum number of periodic heap dumps; zero means unlimited while monitoring stays enabled",
	)
	heapMonitorRuntimeGC := flags.Bool(
		"heap-monitor-runtime-gc",
		true,
		"Whether heap monitor snapshots should force runtime.GC before the second sample",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if addr := strings.TrimSpace(*pprofAddr); addr != "" {
		diagnostics.StartPprofServer(addr)
	}

	var stopHeapMonitor context.CancelFunc
	if *heapMonitorInterval > 0 {
		options := []diagnostics.HeapDumpOption{
			diagnostics.WithName(strings.TrimSpace(*displayName)),
			diagnostics.WithHTTPServer(strings.TrimSpace(*pprofAddr)),
			diagnostics.WithDumpCount(*heapDumpCount),
			diagnostics.WithRuntimeGC(*heapMonitorRuntimeGC),
		}
		if *heapDumpThresholdMB > 0 {
			options = append(options, diagnostics.WithHeapLimit(*heapDumpThresholdMB*1024*1024))
		}
		if dir := strings.TrimSpace(*heapDumpDir); dir != "" {
			options = append(options, diagnostics.WithDumpDir(dir))
		} else {
			options = append(options, diagnostics.WithDisable(true))
		}
		stopHeapMonitor = diagnostics.StartHeapMonitor(*heapMonitorInterval, options...)
		defer stopHeapMonitor()
	}

	scanNode, err := scannode.NewScanNode(node.BaseConfig{
		NodeType:            spec.NodeType_Scanner,
		NodeID:              *nodeID,
		DisplayName:         *displayName,
		AgentInstallationID: *agentInstallationID,
		BaseDir:             *baseDir,
		EnrollmentToken:     *enrollmentToken,
		PlatformAPIBaseURL:  *apiURL,
		Version:             *version,
		HeartbeatInterval:   *heartbeatInterval,
	})
	if err != nil {
		return err
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		scanNode.Run()
	}()

	<-ctx.Done()
	scanNode.Shutdown()
	<-done
	return nil
}
