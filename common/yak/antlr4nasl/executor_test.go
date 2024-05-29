package antlr4nasl

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/utils"
	nasl "github.com/yaklang/yaklang/common/yak/antlr4nasl/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/visitors"
	"github.com/yaklang/yaklang/common/yak/antlr4nasl/vm"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"strconv"
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

func TestAssigment(t *testing.T) {
	engine := NewNaslEngine()
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
	err := engine.Exec(`
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
`, "test-file")
	if err != nil {
		t.Fatal(err)
	}
}

func TestGetword(t *testing.T) {
	code := `
include("byte_func.inc");
buf = getBuf();
dump(getword( blob:buf, pos:0));
# assert(getword( blob:buf, pos:0) == 1,"getword error");
# assert(getword( blob:buf, pos:2) == 2,"getword error");
`
	engine := NewNaslEngine()
	engine.vm.ImportLibs(map[string]interface{}{
		"__function__getBuf": func() any {
			res, _ := codec.DecodeHex("00010002")
			return res
		},
	})
	err := engine.Exec(code, "test-file")
	if err != nil {
		t.Fatal(err)
	}
}
func TestLoadSetting(t *testing.T) {
	port := utils.GetRandomAvailableTCPPort()
	l, err := net.Listen("tcp", spew.Sprintf(":%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				break
			}
			conn.Close()
		}
	}()
	engine := NewScriptEngine()
	engine.Debug()
	engine.SetCache(false)
	PatchEngine(engine)
	engine.LoadCategory("ACT_SETTINGS")
	//engine.LoadScript("snmp_default_communities.nasl")
	//engine.LoadScript("ids_evasion.nasl")
	//engine.LoadScript("compliance_tests.nasl")
	engine.ShowScriptTree()
	engine.SetPreferenceByScriptName("ids_evasion.nasl", "TCP evasion technique", "split")
	_, err = engine.Scan("127.0.0.1", strconv.Itoa(port))
	if err != nil {
		t.Fatal(err)
	}
	preferences := engine.GetAllPreference()
	for scirptName, preference := range preferences {
		if scirptName == "ids_evasion.nasl" {
			for _, p := range preference {
				if p.Name == "TCP evasion technique" {
					spew.Dump(p)
				}
			}
		}
	}
	spew.Dump(engine.Kbs.GetKB("NIDS/TCP/split"))
}
