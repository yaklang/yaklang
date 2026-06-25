//go:build api_arch_test_live

package loop_ssa_api_discovery

import (
	"os"
	"strings"
	"testing"
)

// TestLiveApiArchDiscovery_AllVariants 已迁移至独立 benchmark：
//
//	go run ./common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/cmd/run_api_arch_prompt_benchmark
//
// 需 Yak gRPC（默认 127.0.0.1:33225）与 API_ARCH_CODE_ROOT / API_ARCH_TARGET 环境变量。
func TestLiveApiArchDiscovery_AllVariants(t *testing.T) {
	if os.Getenv("API_ARCH_TEST") != "1" {
		t.Skip("deprecated: use cmd/run_api_arch_prompt_benchmark instead")
	}
	if strings.TrimSpace(os.Getenv("API_ARCH_CODE_ROOT")) == "" {
		t.Skip("need API_ARCH_CODE_ROOT")
	}
	t.Skip("use: go run ./common/ai/aid/aireact/reactloops/loop_ssa_api_discovery/cmd/run_api_arch_prompt_benchmark")
}
