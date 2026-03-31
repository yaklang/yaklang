package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/yaklang/yaklang/common/node"
	"github.com/yaklang/yaklang/common/spec"
	cli "github.com/yaklang/yaklang/common/urfavecli"
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
	nodeID := flags.String("id", "smoke-node", "Node ID")
	version := flags.String("version", "smoke", "Node version")
	heartbeatInterval := flags.Duration(
		"heartbeat-interval",
		node.DefaultHeartbeatInterval,
		"Heartbeat interval",
	)
	if err := flags.Parse(args); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	scanNode, err := scannode.NewScanNode(node.BaseConfig{
		NodeType:           spec.NodeType_Scanner,
		NodeID:             *nodeID,
		EnrollmentToken:    *enrollmentToken,
		PlatformAPIBaseURL: *apiURL,
		Version:            *version,
		HeartbeatInterval:  *heartbeatInterval,
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
