package syntaxflow

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_AnnotationValueFilterKeepsContainerForRefSearch(t *testing.T) {
	code := `
@lokjasdgjlkassdfjlkjloasdfijloa("hk;aabbccddeeff;asdljk")
public class HomeDaoClassABC {
    List<PmsBrand> aaab(@Param("offset") Integer offset,@Param("limit") Integer limit) {
        return null;
    };
}

@ClassAnnotationTest
public class HomeDaoClassABC {
    List<PmsBrand> abasdfasdfasdfbar(@Param("offset") Integer offset,@Param("limit") Integer limit) {
        return null;
    };
}
`

	rule := `
.annotation.*?{.value<regexp('aabbccddeeff')>} as $anno
$anno.__ref__.*ab as $method
`

	ssatest.CheckSyntaxFlowContain(t, code, rule, map[string][]string{
		"method": {"Function-HomeDaoClassABC.aaab"},
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}

