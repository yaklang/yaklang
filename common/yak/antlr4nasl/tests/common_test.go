package tests

import (
	"testing"

	"github.com/yaklang/yaklang/common/yak/antlr4nasl/script_core"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

func TestOperator(t *testing.T) {
	Exec(`
a = 1+1;
assert(a==2,"operator '+' error");
a += 1;
assert(a==3,"operator '+=' error");
a = 1-1;
assert(a==0,"operator '-' error");
a -= 1;
assert(a==-1,"operator '-=' error");
a = 2*2;
assert(a==4,"operator '*' error");
a *= 2;
assert(a==8,"operator '*=' error");
a = 2/2;
assert(a==1,"operator '/' error");
a /= 2;
assert(a==0,"operator '/=' error");
a = 3%2;
assert(a==1,"operator '%' error");
a %= 2;
assert(a==1,"operator '%=' error");
a = 3^2;
assert(a==1,"operator '^' error");
a = 3&2;
assert(a==2,"operator '&' error");
a = 3|2;
assert(a==3,"operator '|' error");
a = 3<<1;
assert(a==6,"operator '<<' error");
a <<= 1;
assert(a==12,"operator '<<=' error");
a = 3>>1;
assert(a==1,"operator '>>' error");
a >>= 1;
assert(a==0,"operator '>>=' error");
a = 3==3;
assert(a==TRUE,"operator '==' error");
a = 3!=3;
assert(a==FALSE,"operator '!=' error");
a = 3>3;
assert(a==FALSE,"operator '>' error");
a = 3>=3;
assert(a==TRUE,"operator '>=' error");
a = 3<3;
assert(a==FALSE,"operator '<' error");
a = 3<=3;
assert(a==TRUE,"operator '<=' error");
a = 0&&1;
assert(a==FALSE,"operator '&&' error");
a = 0||1;
assert(a==TRUE,"operator '||' error");
a = ~3;
assert(a==-4,"operator '~' error");
a = +3;
assert(a==3,"operator '+' error");
a = -3;
assert(a==-3,"operator '-' error");
a = 0;
a++;
assert(a==1,"operator '++' error");
a--;
assert(a==0,"operator '--' error");
++a;
assert(a==1,"operator '++' error");
--a;
assert(a==0,"operator '--' error");
a = 2**3;
assert(a==8,"operator '**' error");
a = "hello";
assert("ello" >< a,"operator '><' error");
assert("e1ll" >!< a,"operator '>!<' error");
assert(a =~ "h.llo","operator '=~' error");
assert(a !~ "h*l1lo","operator '!~' error");
a = -8 >>> 2;
assert(a==1073741822,"operator '>>>' error");
a = -8;
a >>>= 2;
assert(a==1073741822,"operator '>>>=' error");
a = -8 <<< 2;
assert(a==4294967264,"operator '<<<' error");
a = -8;
a <<<= 2;
assert(a==4294967264,"operator '<<=' error");
assert(!"a" == FALSE, "operator '!' error");
`)

}

func TestSetKB(t *testing.T) {
	Exec(`
set_kb_item( name:"unknown_os_or_service/available", value:TRUE );
assert(get_kb_item(name:"unknown_os_or_service/available")==TRUE,"set and get kb error");
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
assert(sum == 1, "test redefine error");
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

func TestExit1(t *testing.T) {
	Exec(`
dump(1);
exit(99);
a = 0/0;
`)
}
func TestExit2(t *testing.T) {
	Exec(`
dump(1);
exit(99,"exit with code 99");
a = 0/0;
`)

}
func TestXOperator(t *testing.T) {
	Exec(`
a = 1;
function multiply(){
	a *= 2;
}
multiply() x 3;
assert(a == 8, "x operator error");
`)
}
func Exec(code string, init ...bool) {
	_Exec(false, code, init...)
}

func DebugExec(code string, init ...bool) {
	_Exec(true, code, init...)
}

func _Exec(debug bool, code string, init ...bool) {
	engine := script_core.NewScriptEngine()
	//engine.vm.GetConfig().SetStopRecover(true)
	//if len(init) == 0 {
	//	engine.InitBuildInLib()
	//}
	_, err := engine.DescriptionExec(code, "test-code")
	if yakvm.GetUndefined().Value != nil {
		panic("undefined value")
	}

	if err != nil {
		panic(err)
	}
	return
}

func TestIf(t *testing.T) {
	Exec(`

if ( !NULL && 1)
dump("ok");
if ("")
assert(false, "empty string is true");
`)
}

func TestAutoCreateVar(t *testing.T) {
	DebugExec(`
function mkvar() {
	for(i =0; i < 3;i++){
		a[i] = 1;
	}
}
mkvar();
dump(a);
assert(a[1]==1,"test auto create var scope failed");

i = 0;
m[i] = 1;
dump(m);
assert(m[0] == 1,"auto create failed");

local_var a,b,c,d,e,f;
a[1] = 1;
c = [1,2,3];
assert(c[1]==2,"c[1]!=2");
dump(a);
assert(a[1]==1,"a[1]!=1");
dump(NULL);

dump(undefinedVar);
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

func TestVariableAssign(t *testing.T) {
	DebugExec(`
var a = 1;
assert(a==1,"a!=1");

var b;
b = 2;
assert(b==2,"b!=2");

local_var c = 3;
assert(c==3,"c!=3");

global_var d = 4;
assert(d==4,"d!=4");
`)
}
