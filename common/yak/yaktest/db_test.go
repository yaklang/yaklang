package yaktest

import (
	"fmt"
	"testing"
)

func TestMisc_DatabaseTest(t *testing.T) {

	cases := []YakTestCase{
		{
			Name: "测试数据库",
			Src: fmt.Sprintf(`
for result = range db.QueryHTTPFlowsByID(1443) {
	dump(result)
}
`),
		},
	}

	Run("测试数据库链接", t, cases...)
}
