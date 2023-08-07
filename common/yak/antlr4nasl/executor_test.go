package antlr4nasl

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/davecgh/go-spew/spew"
	nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/visitors"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/vm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"testing"
)

func TestCode(t *testing.T) {
	Exec(`

a = 0;

	foreach item([1,2,3]){
		a += item;
	}

dump(a);
assert(a == 6,"a != 6");

res = [];
if( ! isnull( res ) ) {
      res = make_list( res );
      foreach entry( res ) {
        # both CPE and free-form entries can be registered under the "OS" banner
        if( "cpe:/" >< entry )
          return entry;
      }
    }
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
	engine.InitBuildInLib()
	engine.GetVirtualMachine().ImportLibs(map[string]interface{}{
		"__function__dump": func(i interface{}) {
			spew.Dump(i)
		},
		"__function__getMap": func() *vm.NaslArray {
			array, _ := vm.NewNaslArray(map[string]string{
				"a": "b",
			})
			return array
		},
	})

	engine.GetCompiler().RegisterVisitHook("a", func(compiler *visitors.Compiler, ctx antlr.ParserRuleContext) {
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
assert(c["a"] == "1","c[a]!=1");
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
dump(a);
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
func TestPlusEq(t *testing.T) {
	DebugExec(`
a += "123";
assert(a == "123","a!=123");
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
func TestString(t *testing.T) {
	DebugExec(`
a =string("a\nb\nc");
res = split(a,sep:"\n");
assert(res[0]=="a","res[0]!=a");
`)
}
func TestEregmatch(t *testing.T) {
	DebugExec(`
if (a = eregmatch(string:"a",pattern:"aaa")){
	assert(0,"a!=NULL");
}

`)
}
func TestEgrep(t *testing.T) {
	DebugExec(`
a = egrep( pattern:"^User-Agent:.+", string:"User-Agent: aaa", icase:TRUE );
dump(a);
`)
}

func TestStrStr(t *testing.T) {
	DebugExec(`
assert(strstr("asdfasdCVE 2023","CVE ") == "CVE 2023","strstr error");
`)
}

func TestSubString(t *testing.T) {
	DebugExec(`
assert("aaa<b>aaa"-"<b>" == "aaaaaa","sub string error");
`)
}
func TestMapElement(t *testing.T) {
	DebugExec(`
array = make_array("a",1);
assert(array[NULL] == NULL,"array[NULL] != NULL");
`)
}

func TestMkword(t *testing.T) {
	DebugExec(`
function mkword(){
	return _FCT_ANON_ARGS[0];
}
dump(mkword(100));
assert(mkword(100) == 100,"mkword error");
`)
}

func TestGetword(t *testing.T) {
	code := `
include("byte_func.inc");
buf = getBuf();
assert(getword( blob:buf, pos:0) == 1,"getword error");
assert(getword( blob:buf, pos:2) == 2,"getword error");
`
	engine := New()
	engine.InitBuildInLib()
	engine.vm.ImportLibs(map[string]interface{}{
		"__function__getBuf": func() any {
			res, _ := codec.DecodeHex("00010002")
			return res
		},
	})
	err := engine.Eval(code)
	if err != nil {
		t.Fatal(err)
	}
}
