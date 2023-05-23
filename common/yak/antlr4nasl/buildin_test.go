package antlr4nasl

import "testing"

func TestKeys(t *testing.T) {
	engine := New()
	engine.InitBuildInLib()
	engine.Eval(`
a = make_list(1,2,3);
sum = 0;
foreach k(keys(a)){
	sum+= k;
}
assert(sum == 3, "keys error");

a = make_array("a", "b", "c","d");
sum1 = "";
foreach k(keys(a)){
	sum1 += k;
}
assert(sum1 == "ac", "keys error");

`)
}
