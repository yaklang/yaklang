package antlr4nasl

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/davecgh/go-spew/spew"
	"testing"
	nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/visitors"
)

func TestCode(t *testing.T) {
	Exec(`

a = 0;

	foreach item([1,2,3]){
		a += item;
	}

dump(a);
assert(a == 6,"a != 6");
`)
}

func TestScope(t *testing.T) {
	Exec(`
	function a(t){
		local_var t;
	}

a(1);
`)
}

func TestRedefine(t *testing.T) {
	Exec(`

sum = 1;
local_var sum;
dump(sum);
`)
}

func TestFor(t *testing.T) {
	Exec(`

sum = 0;

	for(i=0;i<5;i++){
		if (i==1){
			continue;
		}
		if (i==3){
			break;
		}
		sum += i;
	}

assert(sum == 2,"sum != 2");
`)
}

func TestForEach(t *testing.T) {
	Exec(`

sum = 0;

	foreach item([1,2,3,4,5]){
		if (item==1){
			continue;
		}
		if (item==3){
			break;
		}
		sum += item;
	}

assert(sum == 2,"sum != 2");
`)
}

func TestRepeat(t *testing.T) {
	Exec(`

sum = 0;
i=0;

	repeat {
		i++;
		if (i==1){
			continue;
		}
		if (i==3){
			break;
		}
		sum += i;
	} until(i == 5);

assert(sum == 2,"sum != 2");
`)
}

func TestFunction(t *testing.T) {
	Exec(`

	function print(a,b,c,d){
		dump(a);
		dump(b);
		dump(c);
		dump(d);
		assert(a==111,"a != 111");
		assert(b=="123","b != 123");
		assert(c==NULL,"c != NULL");
		assert(d=="d","d != d");
	}

print(111,b:"123",d:"d");
`)
}

func TestFunction2(t *testing.T) {
	Exec(`

	function get_app_port(cpe){
		# dump(cpe);
		# return 123;
	}

if (!port = get_app_port(cpe: 1))dump("获取端口失败");
`)
}

func TestSetKB(t *testing.T) {
	Exec(`

set_kb_item( name:"unknown_os_or_service/available", value:TRUE );
assert(get_kb_item(name:"unknown_os_or_service/available")==TRUE,"set and get kb error");
`)
}

func TestExit(t *testing.T) {
	Exec(`

dump(1);
exit(99);
a = 0/0;
`)
}
func TestXOperator(t *testing.T) {
	Exec(`
dump(1) x 3;
`)
}
func TestAssigment(t *testing.T) {
	engine := New()
	engine.Init()
	engine.GetVirtualMachine().ImportLibs(map[string]interface{}{
		"dump": func(i interface{}) {
			spew.Dump(i)
		},
		"getMap": func() map[string]string {
			return map[string]string{
				"a": "b",
			}
		},
		"getStruct": func() interface{} {
			return &struct {
				A string
			}{
				A: "a",
			}
		},
	})
	engine.GetCompiler().AddVisitHook(func(compiler *visitors.Compiler, ctx antlr.ParserRuleContext) {
		if id, ok := ctx.(*nasl.IdentifierExpressionContext); ok {
			if id.GetText() == "__this__" {
				print()
			}
		}
	})
	err := engine.Eval(`
a = 1;
b = ["1","2","5"];
c = getMap();
c["a"] = "1";
b[0] = "0";
dump(a);
dump(b);
dump(c);
assert(a==1,"a!=1");
assert(b[0]=="0","b[0] != 0");
assert(c.a == "1","c[a]!=1");
assert(c["a"] == "1","c[a]!=1");

d = getStruct();
d.A = "1";
assert(d.A == "1","d.A!=1");
`)
	if err != nil {
		t.Fatal(err)
	}
}
func TestBuildInLib(t *testing.T) {
	Exec(`
s = "password: 1234567890";
res1 = ereg_replace(string:s,pattern:"[0-9]",replace:"*");

res2 = ereg_replace(replace:"*",pattern:"[0-9]",string:s);
assert(res1 == "password: **********","res1 != **********");
dump(res2);
assert(res2 == res1, "res1 != res2");

location = "123";
display( 'DEBUG: Location header is pointing to "' + location + '" on the same host/ip. Returning this location.\n' );

if (isnull(get_kb_item(name:"unknown_os_or_service/available")))
	display("isNULL");
`)
}

func TestPow(t *testing.T) {
	Exec(`

a = 2**2;
dump(a);
assert(a==4,"a!=4");
`)
}

func TestIf(t *testing.T) {
	Exec(`

if ( !NULL && 1)
dump("ok");
if ("")
assert(false, "empty string is true");
`)
}

func TestIncrease(t *testing.T) {
	Exec(`

a = 1;
dump(a++);
assert(a++ == 3,"a++ != 2");
`)
}

func TestUnInitedMap(t *testing.T) {
	DebugExec(`

local_var a,b,c,d,e,f;
a[1] = 1;
c = [1,2,3];
assert(c[1]==2,"c[1]!=2");
assert(a[1]==1,"a[1]!=1");
dump(NULL);
`)
}
func TestUnInitedMap1(t *testing.T) {
	DebugExec(`
local_var a;
a[1] = 1;
`)
}
func TestIterableVarCall(t *testing.T) {
	DebugExec(`
a = [1];
local_var b;
dump(b);
dump(a[1]);
dump(NULL);
assert(a[1]==NULL,"a[1]!=NULL");
assert(a[0]==1,"a[0]!=1");
a = "1";
assert(a[0]=="1","a[0]!=1");
assert(a[1]==NULL,"a[1]!=NULL");
`)
}
