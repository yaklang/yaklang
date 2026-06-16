package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func main() {
	addr := "127.0.0.1:8087"
	if len(os.Args) > 1 {
		addr = os.Args[1]
	}

	conn, err := grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(100*1024*1024)),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to %s: %v\n", addr, err)
		os.Exit(1)
	}
	defer conn.Close()

	client := ypb.NewYakClient(conn)
	ctx := context.Background()

	// Get current config first.
	current, err := client.GetAIGlobalConfig(ctx, &ypb.Empty{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to get current config: %v\n", err)
	} else {
		fmt.Printf("Current config: Enabled=%v, IntelligentModels=%d, LightweightModels=%d\n",
			current.Enabled, len(current.IntelligentModels), len(current.LightweightModels))
	}

	// Configure Minimax via aibalance.
	apiKey := "mf-aef7706d-c5a4-4d3e-8eaf-e53a82d622d2"

	cfg := &ypb.AIGlobalConfig{
		Enabled:       true,
		RoutingPolicy: "performance",
		IntelligentModels: []*ypb.AIModelConfig{
			{
				ModelName: "memfit-minimax-m3-thinking",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "aibalance",
					APIKey: apiKey,
				},
			},
		},
		LightweightModels: []*ypb.AIModelConfig{
			{
				ModelName: "memfit-minimax-m3",
				Provider: &ypb.ThirdPartyApplicationConfig{
					Type:   "aibalance",
					APIKey: apiKey,
				},
			},
		},
	}

	// Preserve existing vision models if any.
	if current != nil && len(current.VisionModels) > 0 {
		cfg.VisionModels = current.VisionModels
	}

	_, err = client.SetAIGlobalConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set AI config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("AI Provider configured successfully!")
	fmt.Println("  Intelligent model: memfit-minimax-m3-thinking (aibalance)")
	fmt.Println("  Lightweight model: memfit-minimax-m3 (aibalance)")

	// Verify.
	updated, err := client.GetAIGlobalConfig(ctx, &ypb.Empty{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to verify config: %v\n", err)
		return
	}
	fmt.Printf("\nVerified: Enabled=%v, Policy=%s\n", updated.Enabled, updated.RoutingPolicy)
	for i, m := range updated.IntelligentModels {
		fmt.Printf("  Intelligent[%d]: %s (provider: %s)\n", i, m.ModelName, m.Provider.Type)
	}
	for i, m := range updated.LightweightModels {
		fmt.Printf("  Lightweight[%d]: %s (provider: %s)\n", i, m.ModelName, m.Provider.Type)
	}
}
