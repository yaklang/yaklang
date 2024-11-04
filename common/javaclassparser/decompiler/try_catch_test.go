package decompiler

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestTryCatch(t *testing.T) {
	builder := newGraphBuilder()
	startNode := builder.NewNode("start")
	tryNode := builder.NewTry("try")
	tryBody1 := builder.NewNode("tryBody1")
	catchBody1 := builder.NewNode("catchBody1")
	mergeNode := builder.NewNode("mergeNode")
	tryNode.AddNext(tryBody1)
	tryNode.AddNext(catchBody1)
	tryBody1.AddNext(mergeNode)
	catchBody1.AddNext(mergeNode)
	startNode.AddNext(tryNode)
	sourceCode, err := dumpGraph(startNode)
	if err != nil {
		t.Fatal(err)
	}
	println(sourceCode)
	assert.Equal(t, `start
if (try){
tryBody1
}else{
catchBody1
}
mergeNode`, sourceCode)
}
