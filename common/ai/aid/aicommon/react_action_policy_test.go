package aicommon

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestReActActionPolicyInheritance(t *testing.T) {
	p := NewConfig(context.Background(), WithDisableAutoSkills(true), WithReActActionPolicy(func(loop, action string) bool { return loop == "plan" && action == "read_file" }))
	c := NewConfig(context.Background(), ConvertConfigToOptions(p)...)
	require.True(t, IsReActActionAllowed(c, "plan", "read_file"))
	require.False(t, IsReActActionAllowed(c, "plan", "search_knowledge"))
	require.True(t, IsReActActionAllowed(struct{}{}, "any", "any"))
	require.True(t, IsReActActionAllowed((*Config)(nil), "any", "any"))
}
