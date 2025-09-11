package test

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

func TestFunctionCallTypeCheck(t *testing.T) {
	t.Run("normal function", func(t *testing.T) {
		check(t, `
		codec.DecodeBase64(1)
		`, []string{
			ssa4analyze.ArgumentTypeError(1, "number", "string", "codec.DecodeBase64"),
		})
	})

	t.Run("variadic function call", func(t *testing.T) {
		check(t, `
		ssa.Parse(1)
		`, []string{
			ssa4analyze.ArgumentTypeError(1, "number", "string", "ssa.Parse"),
		})
	})

	// TODO: check this parameter type
	t.Run("variadic function call, error type in variadic parament", func(t *testing.T) {
		check(t, `
		ssa.Parse("a", 1)
		`, []string{
			ssa4analyze.ArgumentTypeError(2, "number", "ssaapi.Option", "ssa.Parse"),
		})
	})

	t.Run("variadic function call, error type both", func(t *testing.T) {
		check(t, `
		ssa.Parse(1, 1) 
		`, []string{
			ssa4analyze.ArgumentTypeError(1, "number", "string", "ssa.Parse"),
			ssa4analyze.ArgumentTypeError(2, "number", "ssaapi.Option", "ssa.Parse"),
		})
	})
}

func TestFunctionCallParameterLength(t *testing.T) {
	t.Run("normal function", func(t *testing.T) {
		check(t, `
		codec.DecodeBase64()
		`, []string{
			ssa4analyze.NotEnoughArgument("codec.DecodeBase64", "", "string"),
		})
	})

	t.Run("variadic function call, not enough min length", func(t *testing.T) {
		check(t, `
		ssa.Parse()
		`, []string{
			ssa4analyze.NotEnoughArgument("ssa.Parse", "", "string, ...ssaapi.Option"),
		})
	})

	t.Run("variadic function call", func(t *testing.T) {
		check(t, `
		ssa.Parse("a")
		`, []string{})
	})

	t.Run("variadic function call, has more parament", func(t *testing.T) {
		check(t, `
		ssa.Parse("a", ssa.withLanguage(ssa.Javascript))
		`, []string{})
	})

	t.Run("variadic function call, has ellipsis", func(t *testing.T) {
		check(t, `
		opt = [ssa.withLanguage(ssa.Javascript)]
		ssa.Parse("a", opt...)
		`, []string{})
	})

	t.Run("no-variadic function call, but has ellipsis", func(t *testing.T) {
		check(t, `
		a = ["a", "b"]
		codec.DecodeBase64(a...)
		`, []string{
			ssa4analyze.ArgumentTypeError(1, "[]string", "string", "codec.DecodeBase64"),
		})
	})
}

func TestFunctionCallGeneric(t *testing.T) {
	t.Run("test append generic", func(t *testing.T) {
		check(t, `
slice1 = []
slice2 = []
slice1 = append(slice1, "a")
slice2 = append(slice2, 1)
		`, []string{})
	})
}

func TestMakeByte(t *testing.T) {
	t.Run("make byte", func(t *testing.T) {
		check(t, `
		a = make([]byte, 1)
		`, []string{})
	})
	t.Run("make slice, without size cap", func(t *testing.T) {
		check(t, `
		make([]int)`, []string{})
	})
	t.Run("make slice, without cap ", func(t *testing.T) {
		check(t, `
		make([]int, 1)`, []string{})
	})
	t.Run("make slice", func(t *testing.T) {
		check(t, `
		make([]int, 1, 1)`, []string{})
	})
	t.Run("make slice more argument", func(t *testing.T) {
		check(t, `
		make([]int, 1, 1, 3)`, []string{
			yak2ssa.MakeArgumentTooMuch("slice"),
		})
	})

	t.Run("make chan more argument", func(t *testing.T) {
		check(t, `
		make(chan int, 1, 3)`, []string{
			yak2ssa.MakeArgumentTooMuch("chan"),
		})
	})
}

func TestBasicTypeCheck(t *testing.T) {
	t.Run("yak append", func(t *testing.T) {
		check(t, `
cookie = make([]var)
cookie = append(cookie)
		
		`, []string{})
	})

	t.Run("yak append map", func(t *testing.T) {
		check(t, `
cookie = make([]var)
cookie = append(cookie, {
    "cookie": codec.EncodeBase64(""),
    "key": "",
    "aes-mode": "",
}, {
    "cookie": codec.EncodeBase64(""),
    "key": "",
    "aes-mode": "",
})
		
		`, []string{})
	})

	t.Run("yak append ellipsis", func(t *testing.T) {
		check(t, `
list = ["a","b","c"]
newlist = []
newlist = append(newlist, list...)
		
		`, []string{})
	})

	t.Run("yak slice with Append call", func(t *testing.T) {
		check(t, `
opt = [
    syntaxflow.withContext(context.Background()), 
]
opt.Append(syntaxflow.withCache())
		`, []string{})
	})

	t.Run("yak slice with Append", func(t *testing.T) {
		check(t, `
replaceResults = []
emptyResults = []
replacedTmp = []
emptyTmp  = []

if len(replacedTmp) == 0 {
    replacedTmp.Append({"key": "", "kind": "", "value": ""})
    emptyResults.Append({"key": "", "kind": "", "value": ""})
} else {
    replaceResults.Append(replacedTmp)
    emptyResults.Append(emptyTmp)
}
		
		`, []string{})
	})

	t.Run("yak slice with Push", func(t *testing.T) {
		check(t, `
list = []
list.Push({"key": "", "kind": "", "value": ""})	
		`, []string{})
	})

	t.Run("yak slice with Pop", func(t *testing.T) {
		check(t, `
list = ["a","b","c"]
list.Pop(2,3)	
		`, []string{})
	})

	t.Run("yak slice with Sort", func(t *testing.T) {
		check(t, `
list = ["a","b","c"]
list.Sort(true)	
		`, []string{})
	})

	t.Run("yak string with Fuzz", func(t *testing.T) {
		check(t, `
s = ""
s.Fuzz( {
    "key": "",
})
		`, []string{})
	})

	t.Run("yak string with Ljust", func(t *testing.T) {
		check(t, `
s = ""
s.Ljust(1,"a","b")
		`, []string{})
	})

	t.Run("yak string with Trim", func(t *testing.T) {
		check(t, `
infos, err = file.ReadDirInfoInDirectory("")
die(err)

for i in infos {
	if !i.IsDir {
		continue
	}
	dir, name = file.Split(i.Path)
	name = name.Trim("/", "\\")
	if name == "engine-log" || name == "temp" {
		file.Walk(
			i.Path,
			logFile => {
				if !logFile.Path.HasSuffix(".txt") {return true}
				files.Push(logFile.Path)
				return true
			},
		)
	}
}
		`, []string{})
	})
}
func TestHandlerError(t *testing.T) {
	t.Run("test handler1", func(t *testing.T) {
		code := `func c(){
	a = randn(1, 2)
	_,err = poc.ParseBytesToHTTPRequest("")
	if(a!=2){
		println(err);
		return true;
	}
		println(err);
	if(a!=3){
		return true;
	}
	println(a);
}
`
		check(t, code, []string{})
	})
	t.Run("test handler2", func(t *testing.T) {
		code := `bb = str.Join([]string{"1","2"}, " ")
req,err = poc.ParseBytesToHTTPRequest("")
if err!=nil{
    return true
}
if bb{
    _,_,err = poc.HTTP("")
}else{
    _,_,err = poc.HTTP("")
}

if bb{
    return true
}`
		check(t, code, []string{"Error Unhandled ", "Error Unhandled "})
	})
	t.Run("map member access in complex situation", func(t *testing.T) {
		code := `
	# input your yak code
println("Hello Yak World!")
func getWays(){
    w = infoMap["Include_Key_Word"][0]
    results := {}
    if w.Way {
        if !results.Has("discard") {
            results.discard = make(map[string][]string)
        }
        return results
    }else{
        w.Include_Key_Word_Way = w.Has("Include_Key_Word_Way") ? w.Include_Key_Word_Way : ""
        if w.Include_Key_Word_Way == ""{
            return results
        }
        if  w.Include_Key_Word_Way!="" {
            if !results.Has("discard") {
                results.discard = make(map[string][]string)
            }
            results.discard.Header = make([]string, 0)
        }
    }
    
    return results
}
`
		check(t, code, []string{"Can't find definition of this variable infoMap both inside and outside the function.", "map literal not have map pairs"})
	})
	t.Run("simple map member access", func(t *testing.T) {
		code := `
results := {}
if !results.Has("discard") {
     results.discard = make(map[string][]string)
}
results.discard.Header = make([]string, 0)
results.discard.Header = append(results.discard.Header, "1")
`
		check(t, code, []string{"map literal not have map pairs"})
	})

	t.Run("panic: keywords cause freevalue fail generation", func(t *testing.T) {
		check(t, `
func assignParam(Packet,Pname,Pvalue,funcname){
    pap := "ReplaceHTTPPacketQueryParam"
    for k,v := range ParamsFull {
        if k == Pname {
            // 爆红没事
            Packet = poc[pap](Packet,Pname,Pvalue/*type: map[string]string*/)
            break
        }else{
            // 爆红没事
            Packet = poc[pap](Packet,Pname,Pvalue)
            break
        }
    }
    return Packet
}
p = ""
println(assignParam(p,"_method","123","当前方法"))
		`, []string{
			ssa.InvalidField("any", ""),
			ssa.BindingNotFoundInCall("ParamsFull"),
			ssa.BindingNotFound("ParamsFull", memedit.NewRange(memedit.NewPosition(18, 9), memedit.NewPosition(18, 46))),
		})
	})
}
