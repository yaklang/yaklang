package yaklib

import (
	"fmt"
	"testing"

	"github.com/davecgh/go-spew/spew"
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
	v := _xmlloads(s)
	spew.Dump(v)
	fmt.Println(string(_xmldumps(v)))
}
