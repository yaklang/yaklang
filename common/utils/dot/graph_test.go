package dot

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
	"testing"
)

func TestGraph(t *testing.T) {
	g := New()
	n1 := g.AddNode("1")
	n2 := g.AddNode("2")
	g.AddEdge(n1, n2, "edge 1->2")
	var buf bytes.Buffer
	g.GenerateDOT(&buf)
	data := buf.String()
	fmt.Println(data)
	data = strings.ReplaceAll(data, " ", "")
	data = strings.ReplaceAll(data, "\n", "")
	data = strings.ReplaceAll(data, "\r", "")
	spew.Dump(data)
	if !utils.MatchAllOfSubString(`label="1"`, `label="2"`) {
		t.Errorf("label not match")
		t.Fail()
	}
}
