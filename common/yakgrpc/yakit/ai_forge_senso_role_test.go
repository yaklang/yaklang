package yakit

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestFilterAIForge_TagMatchesCommaJoinedField(t *testing.T) {
	db := newAIForgeTestDB(t)

	forgeName := newAIForgeTestName("tag-filter")
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName: forgeName,
		ForgeType: schema.FORGE_TYPE_Config,
		Tags:      "安全运营,报告生成,senso-role:operations-manager",
	}))

	cases := []struct {
		tag  string
		want bool
	}{
		{"senso-role:operations-manager", true},
		{"安全运营", true},
		{"报告生成", true},
		{"报告生成,安全运营", false},
		{"senso-role:ciso", false},
		{"senso-role:operations-manage", false},
		{"运营", false},
	}
	for _, c := range cases {
		var forges []*schema.AIForge
		err := FilterAIForge(db, &ypb.AIForgeFilter{Tag: []string{c.tag}}).Find(&forges).Error
		require.NoError(t, err)
		if c.want {
			require.Len(t, forges, 1, "tag %q should match exactly one forge", c.tag)
			require.Equal(t, forgeName, forges[0].ForgeName)
		} else {
			require.Empty(t, forges, "tag %q should match nothing", c.tag)
		}
	}
}

func TestCreateOrUpdateAIForgeByName_PreservesUserRoleTag(t *testing.T) {
	db := newAIForgeTestDB(t)

	forgeName := newAIForgeTestName("preserve-role-tag")
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName: forgeName,
		ForgeType: schema.FORGE_TYPE_Config,
		Tags:      "主机安全,研判,senso-role:threat-analyst",
	}))

	// 用户在前端把角色改成 ciso
	require.NoError(t, UpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName: forgeName,
		Tags:      "主机安全,研判,senso-role:ciso",
	}))

	// 内置同步再次 upsert，默认角色是 threat-analyst，不应覆盖用户选择
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName: forgeName,
		ForgeType: schema.FORGE_TYPE_Config,
		Tags:      "主机安全,研判,senso-role:threat-analyst",
	}))

	got, err := GetAIForgeByName(db, forgeName)
	require.NoError(t, err)
	require.Contains(t, got.Tags, "senso-role:ciso")
	require.NotContains(t, got.Tags, "senso-role:threat-analyst")
	require.Contains(t, got.Tags, "主机安全")
}

func TestCreateOrUpdateAIForgeByName_FillsRoleTagWhenAbsent(t *testing.T) {
	db := newAIForgeTestDB(t)

	forgeName := newAIForgeTestName("fill-role-tag")
	// 存量数据：没有角色标签
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName: forgeName,
		ForgeType: schema.FORGE_TYPE_Config,
		Tags:      "基线,合规",
	}))

	// 内置同步带来默认角色标签，应补入而不是丢弃
	require.NoError(t, CreateOrUpdateAIForgeByName(db, forgeName, &schema.AIForge{
		ForgeName: forgeName,
		ForgeType: schema.FORGE_TYPE_Config,
		Tags:      "基线,合规,senso-role:ciso",
	}))

	got, err := GetAIForgeByName(db, forgeName)
	require.NoError(t, err)
	require.Contains(t, got.Tags, "senso-role:ciso")
	require.Contains(t, got.Tags, "基线")
}
