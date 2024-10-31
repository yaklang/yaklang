package decompiler

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/utils"
	"testing"
)

func TestIfElse(t *testing.T) {
	builder := newGraphBuilder()
	startNode := builder.NewNode("start")
	ifNode := builder.NewIf("if1")
	body1Node := builder.NewNode("body1")
	body2Node := builder.NewNode("body2")
	endNode := builder.NewNode("end")
	end2Node := builder.NewNode("end2")

	startNode.AddNext(ifNode)
	ifNode.AddNext(body1Node)
	ifNode.AddNext(body2Node)
	body1Node.AddNext(endNode)
	body2Node.AddNext(endNode)
	endNode.AddNext(end2Node)
	println(utils.DumpNodesToDotExp(startNode))
	source, err := dumpGraph(startNode)
	if err != nil {
		t.Fatal(err)
	}
	println(source)
	assert.Equal(t, `start
if (if1){
body1
}else{
body2
}
end
end2`, source)
}
func TestIf(t *testing.T) {
	builder := newGraphBuilder()
	startNode := builder.NewNode("start")
	ifNode := builder.NewIf("if1")
	body1Node := builder.NewNode("body1")
	endNode := builder.NewNode("end")
	end2Node := builder.NewNode("end2")

	startNode.AddNext(ifNode)
	ifNode.AddNext(body1Node)
	ifNode.AddNext(endNode)
	body1Node.AddNext(endNode)
	endNode.AddNext(end2Node)
	println(utils.DumpNodesToDotExp(startNode))
	source, err := dumpGraph(startNode)
	if err != nil {
		t.Fatal(err)
	}
	println(source)
	assert.Equal(t, `start
if (if1){
body1
}else{

}
end
end2`, source)
}
