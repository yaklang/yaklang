package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

func TestIsMCPServersAllowed_NilInvoker(t *testing.T) {
	assert.True(t, IsMCPServersAllowed(nil))
}

func TestIsMCPServersAllowed_DefaultMockInvoker(t *testing.T) {
	inv := mock.NewMockInvoker(context.Background())
	assert.True(t, IsMCPServersAllowed(inv))
}

func TestIsMCPServersAllowed_DisallowMCPServers(t *testing.T) {
	ctx := context.Background()
	inv := mock.NewMockInvoker(ctx)
	invWithPolicy := &mcpPolicyTestInvoker{
		MockInvoker: inv,
		config: aicommon.NewConfig(
			ctx,
			aicommon.WithDisallowMCPServers(true),
		),
	}
	assert.False(t, IsMCPServersAllowed(invWithPolicy))
}

type mcpPolicyTestInvoker struct {
	*mock.MockInvoker
	config aicommon.AICallerConfigIf
}

func (i *mcpPolicyTestInvoker) GetConfig() aicommon.AICallerConfigIf {
	return i.config
}
