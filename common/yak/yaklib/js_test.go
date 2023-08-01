package yaklib

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javascript/otto/parser"
	"strconv"
	"testing"
)

func TestWalk(t *testing.T) {
	code := `
	setTimeout(function() {
	window.location.replace(` + strconv.Quote("http://baidu.com") + `);
	console.log("1111")
	}, 3000)
console.log("1111")
for (var i=0; i<5; i++)
{
	console.log("1111")
}
var a = 1
var b = 2
var a = 2 
if (a == b){
	console.log("1111")
}
	`
	res, err := parser.ParseFile(nil, "", code, 0)
	if err == nil {
		fmt.Printf("%v", res)
	}
}
