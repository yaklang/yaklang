package yakit

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestCURD_GeneralRule_base(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	generalRule := &schema.GeneralRule{
		CPE: &schema.CPE{
			Part:    utils.RandStringBytes(10),
			Vendor:  "microsoft",
			Product: "windows",
			Version: "10",
			Update:  "1809",
			Edition: "pro",
		},
		WebPath:         "https://www.microsoft.com",
		ExtInfo:         "windows",
		MatchExpression: "windows",
		RuleName:        "abc",
	}

	generalRule2 := &schema.GeneralRule{
		CPE: &schema.CPE{
			Part:    utils.RandStringBytes(10),
			Vendor:  "microsoft",
			Product: "windows",
			Version: "10",
			Update:  "1809",
			Edition: "pro",
		},
		WebPath:         "https://www.microsoft.com",
		ExtInfo:         "windows",
		MatchExpression: "windows",
		RuleName:        "cba",
	}
	err := CreateGeneralRule(db, generalRule)
	require.NoError(t, err)

	err = CreateGeneralRule(db, generalRule2)
	require.NoError(t, err)

	id := int64(generalRule.ID)
	token := utils.RandStringBytes(10)
	generalRule.CPE.Part = token

	err = CreateOrUpdateGeneralRule(db, generalRule.RuleName, generalRule)
	require.NoError(t, err)

	rule, err := GetGeneralRuleByID(db, id)
	require.NoError(t, err)
	require.Equal(t, token, rule.CPE.Part)

	count, err := DeleteGeneralRuleByFilter(db, &ypb.FingerprintFilter{IncludeId: []int64{id, int64(generalRule2.ID)}})
	require.NoError(t, err)
	require.Equal(t, int64(2), count)
}
