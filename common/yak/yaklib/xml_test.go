package yaklib

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
)

func TestXMLloadsAndDumps(t *testing.T) {
	s := `
<a>
	<b>
		<c>
			1
		</c>
		<d>
			2
		</d>
	</b>
</a>
`
	v := utils.XmlLoads(s)
	spew.Dump(v)
	fmt.Println(string(utils.XmlDumps(v)))
}
