package yakgrpc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestCompare(t *testing.T) {
	compare := NewSsaCompare(NewCompareRiskItem("5d7d90b1-0947-43eb-aa8c-16c1404823cc")).WithGenerateHash(func(risk *schema.SSARisk) string {
		return utils.CalcMd5(risk.CodeFragment, risk.Variable, risk.FromRule)
	})
	for c := range compare.Compare(context.TODO(), NewCompareRiskItem("8950e9b6-4c0e-41b5-9bd0-59dfc1d47f86")) {
		marshal, err := json.Marshal(c)
		require.NoError(t, err)
		fmt.Println(string(marshal))
	}
}
