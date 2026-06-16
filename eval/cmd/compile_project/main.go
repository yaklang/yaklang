package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8087", "yaklang gRPC server address")
	project := flag.String("project", "", "Path to project directory")
	flag.Parse()

	if *project == "" {
		fmt.Fprintln(os.Stderr, "Error: --project is required")
		flag.Usage()
		os.Exit(1)
	}

	conn, err := grpc.Dial(*addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer conn.Close()

	client := ypb.NewYakClient(conn)

	resp, err := client.QuerySSAPrograms(
		context.Background(),
		&ypb.QuerySSAProgramRequest{},
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to query programs: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("SSA Programs: %d\n", len(resp.GetPrograms()))
	for _, p := range resp.GetPrograms() {
		fmt.Printf("  - %s (lang: %s, id: %d)\n", p.Name, p.Language, p.Id)
	}

	fmt.Printf("\nTo compile and evaluate, run:\n")
	fmt.Printf("  go run ./cmd/eval --project %s --lang golang --case cases/ground_truth/cve-2026-54090.json\n", *project)
}
