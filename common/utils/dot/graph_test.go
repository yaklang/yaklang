package dot

import (
	"bytes"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	"os"
	"strings"
	"testing"
)

// This example shows how Graph can be used to display a simple linked list.
// The output can be piped to the dot tool to generate an image.
func TestlinkedList(t *testing.T) {
	G := New()
	G.MakeDirected()
	n1 := G.AddNode("Hello")
	n2 := G.AddNode("World")
	n3 := G.AddNode("Hi")
	n4 := G.AddNode("NULL")
	G.AddEdge(n1, n2, "next")
	G.AddEdge(n2, n3, "next")
	G.AddEdge(n3, n4, "next")
	G.MakeSameRank(n1, n2, n3, n4)

	G.GraphAttribute(NodeSep, "0.5")

	G.DefaultNodeAttribute(Shape, ShapeBox)
	G.DefaultNodeAttribute(FontName, "Courier")
	G.DefaultNodeAttribute(FontSize, "14")
	G.DefaultNodeAttribute(Style, StyleFilled+","+StyleRounded)
	G.DefaultNodeAttribute(FillColor, "yellow")

	G.NodeAttribute(n4, Shape, ShapeCircle)
	G.NodeAttribute(n4, Style, StyleDashed)

	G.DefaultEdgeAttribute(FontName, "Courier")
	G.DefaultEdgeAttribute(FontSize, "12")

	G.GenerateDOT(os.Stdout)
	// output:
	// strict digraph {
	//   nodesep = "0.5";
	//   node [ shape = "box" ]
	//   node [ fontname = "Courier" ]
	//   node [ fontsize = "14" ]
	//   node [ style = "filled,rounded" ]
	//   node [ fillcolor = "yellow" ]
	//   edge [ fontname = "Courier" ]
	//   edge [ fontsize = "12" ]
	//   n0 [label="Hello"]
	//   n1 [label="World"]
	//   n2 [label="Hi"]
	//   n3 [label="NULL", shape="circle", style="dashed"]
	//   {rank=same; n0; n1; n2; n3; }
	//   n0 -> n1 [label="next"]
	//   n1 -> n2 [label="next"]
	//   n2 -> n3 [label="next"]
	// }
}

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
	if !utils.MatchAllOfSubString(data, `label="1"`, `label="2"`) {
		t.Errorf("label not match")
		t.Fail()
	}
}
