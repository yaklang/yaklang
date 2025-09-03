package ssaapi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/java/template2java"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"

	_ "github.com/yaklang/yaklang/common/syntaxflow/sfbuildin"
)

const (
	// NativeCall_GetReturns is used to get the returns of a value
	NativeCall_GetReturns = "getReturns"

	// NativeCall_GetFormalParams is used to get the formal params of a value
	NativeCall_GetFormalParams = "getFormalParams"

	// NativeCall_GetFunc is used to get the function of a value
	// find current function instruction which contains the value
	NativeCall_GetFunc = "getFunc"

	// NativeCall_GetCall is used to get the call of a value, generally used to get the call of an opcode
	NativeCall_GetCall = "getCall"

	// NativeCall_GetCallee is used to get the caller of a value
	// find the caller instruction which contains the value
	NativeCall_GetCallee = "getCallee"

	// NativeCall_SearchFunc is used to search the call of a value, generally used to search the call of a function
	// if the input is a call already, check the 'call' 's method(function) 's other call(search mode)
	//
	// searchCall is not like getCall, search call will search all function name(from call) in the program
	NativeCall_SearchFunc = "searchFunc"

	// NativeCall_GetObject is used to get the object of a value
	NativeCall_GetObject = "getObject"

	// NativeCall_GetMembers is used to get the members of a value
	NativeCall_GetMembers = "getMembers"

	// NativeCall_GetMemberByKey is used to get the members of a value by key
	// example: <getMemberByKey(key="")>
	NativeCall_GetMemberByKey = "getMemberByKey"

	// NativeCall_GetSiblings is used to get the siblings of a value
	NativeCall_GetSiblings = "getSiblings"

	// NativeCall_TypeName is used to get the type name of a value
	NativeCall_TypeName = "typeName"

	// NativeCall_FullTypeName is used to get the full type name of a value
	NativeCall_FullTypeName = "fullTypeName"

	// NativeCall_Name is used to get the function name of a value
	NativeCall_Name = "name"

	// NativeCall_String is used to get the function name of a value
	NativeCall_String = "string"

	// NativeCall_Include is used to include a syntaxflow-rule
	NativeCall_Include = "include"

	// NativeCall_Eval is used to eval a new syntaxflow rule
	NativeCall_Eval = "eval"

	// NativeCall_Fuzztag is used to eval a new yaklang fuzztag template, the variables is in SFFrameResult
	NativeCall_Fuzztag = "fuzztag"

	// NativeCall_Show just show the value, do nothing
	NativeCall_Show = "show"

	// NativeCall_Slice just show the value, do nothing
	// example: <slice(start=0)>
	NativeCall_Slice = "slice"

	// NativeCall_Regexp is used to regexp, group is available
	//   you can use <regexp(`...`, group: 1)> to extract
	NativeCall_Regexp = "regexp"

	// NativeCall_StrLower is used to convert a string to lower case
	NativeCall_StrLower = "strlower"

	// NativeCall_StrUpper is used to convert a string to upper case
	NativeCall_StrUpper = "strupper"

	// NativeCall_Var is used to put vars to variables
	NativeCall_Var = "var"

	// NativeCall_MyBatisSink is used to find MyBatis Sink for default searching
	NativeCall_MyBatisSink = "mybatisSink"

	// NativeCall_FreeMarkerSink is used to find FreeMarker Sink for default searching
	NativeCall_FreeMarkerSink = "freeMarkerSink"

	// NativeCall_OpCodes is used to get the opcodes of a value
	NativeCall_OpCodes = "opcodes"

	// NativeCall_SourceCode is used to get the source code of a value
	NativeCall_SourceCode = "sourceCode"

	// NativeCall_ScanPrevious is used to scan previous opcode of a value
	NativeCall_ScanPrevious = "scanPrevious"

	// NativeCall_ScanNext is used to scan next
	NativeCall_ScanNext = "scanNext"

	//NativeCall_ScanInstruction is used to scan current block's instruction
	NativeCall_ScanInstruction = "scanInstruction"

	//NativeCall_DeleteVariable is used to delete a variable
	NativeCall_DeleteVariable = "delete"

	// NativeCall_Forbid is used to forbid a value, if values existed, report critical error.
	NativeCall_Forbid = "forbid"

	// NativeCall_Self is used to get self value
	NativeCall_Self = "self"

	// NativeCall_DataFlow is used to get data flow
	// if u want to fetch dataflow, call <dataflow...> after --> or #->
	// use it like: $data<dataflow(<<<CODE
	// *?{opcode: call && <getCallee><name>?{name} }
	// CODE)>
	NativeCall_DataFlow = "dataflow"

	// NativeCall_Const is used to search const value
	NativeCall_Const = "const"

	// NativeCall_VersionIn is used to get the version in
	NativeCall_VersionIn = "versionIn"

	// NativeCall_IsSanitizeName checks for potential sanitization function names
	NativeCall_IsSanitizeName = "isSanitizeName"

	// NativeCall_Java_UnEscape_Output  is used to show output in java template languages that has not been escape,
	// and is generally used to audit XSS vulnerabilities
	NativeCall_Java_UnEscape_Output = "javaUnescapeOutput"

	NativeCall_Foeach_Func_Inst = "foreach_function_inst"

	NativeCall_GetFilenameByContent = "FilenameByContent"

	NativeCall_GetFullFileName = "getFullFileName"

	NativeCall_GetUsers = "getUsers"

	NativeCall_GetPredecessors = "getPredecessors"

	NativeCall_GetActualParams = "getActualParams"

	NativeCall_GetActualParamLen = "getActualParamLen"

	//getCurrentBlueprint is used to get the current blueprint. only function can use it
	NativeCall_GetCurrentBlueprint = "getCurrentBlueprint"

	NativeCall_ExtendsBy = "extendsBy"

	NativeCall_GetBlurpint = "getBluePrint"

	NativeCall_GetParentBlueprint = "getParentsBlueprint"

	NativeCall_GetInterfaceBlueprint = "getInterfaceBlueprint"

	NativeCall_GetRootParentBlueprint = "getRootParentBlueprint"

	NativeCall_Length = "len"

	NativeCall_GetRoot = "root"
)

func init() {
	registerNativeCall(NativeCall_GetRoot, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var result []sfvm.ValueOperator
		prog, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		var getRoot func(value ssa.Value)
		getRoot = func(value ssa.Value) {
			if utils.IsNil(value) {
				return
			}
			call, isCall := ssa.ToCall(value)
			if isCall {
				if method, ok := call.GetValueById(call.Method); ok && method != nil {
					getRoot(method)
				}
				return
			}
			obj := value.GetObject()
			if utils.IsNil(obj) {
				newValue, err2 := prog.NewValue(value)
				if err2 != nil {
					return
				}
				result = append(result, newValue)
				return
			}
			getRoot(obj)
		}
		v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Program:
				return nil
			case *Value:
				getRoot(ret.innerValue)
			}
			return nil
		})
		return true, sfvm.NewValues(result), nil
	}))

	registerNativeCall(NativeCall_GetRootParentBlueprint, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var result []sfvm.ValueOperator
		prog, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		blueprints := getCurrentBlueprint(v)
		for _, blueprint := range blueprints {
			for _, parent := range blueprint.GetRootParentBlueprints() {
				if val, err := prog.NewValue(parent.Container()); err == nil {
					result = append(result, val)
				}
			}
		}
		if len(result) == 0 {
			return false, nil, utils.Errorf("no parents blueprint found")
		}
		return true, sfvm.NewValues(result), nil
	}))
	registerNativeCall(NativeCall_Length, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		count := 0
		v.Recursive(func(operator sfvm.ValueOperator) error {
			switch operator.(type) {
			case *Value, *Program:
				count++
			}
			return nil
		})
		return true, v.NewConst(count), nil
	}))

	registerNativeCall(NativeCall_GetInterfaceBlueprint, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var result []sfvm.ValueOperator
		prog, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		blueprints := getCurrentBlueprint(v)
		for _, blueprint := range blueprints {
			for _, parent := range blueprint.GetAllInterfaceBlueprints() {
				if val, err := prog.NewValue(parent.Container()); err == nil {
					result = append(result, val)
				}
			}
		}
		if len(result) == 0 {
			return false, nil, utils.Errorf("no parents blueprint found")
		}
		return true, sfvm.NewValues(result), nil
	}))

	registerNativeCall(NativeCall_GetParentBlueprint, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var result []sfvm.ValueOperator
		prog, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		blueprints := getCurrentBlueprint(v)
		for _, blueprint := range blueprints {
			for _, parent := range blueprint.GetAllParentsBlueprint() {
				if val, err := prog.NewValue(parent.Container()); err == nil {
					result = append(result, val)
				}
			}
		}
		if len(result) == 0 {
			return false, nil, utils.Errorf("no parents blueprint found")
		}
		return true, sfvm.NewValues(result), nil
	}))

	registerNativeCall(NativeCall_GetBlurpint, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var result []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Value:
				_, isBlueprint := ssa.ToClassBluePrintType(ret.innerValue.GetType())
				if isBlueprint {
					result = append(result, ret)
				}
			default:
				return nil
			}
			return nil
		})
		return true, sfvm.NewValues(result), nil
	}))
	registerNativeCall(NativeCall_ExtendsBy, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		/*
		*a<extendsBy($b)> 判断a是否继承自b
		 */
		var result []sfvm.ValueOperator
		var extends []*ssa.Blueprint
		name := params.GetString(0)
		val, ok := frame.GetSymbolByName(name)
		if !ok {
			return false, nil, utils.Errorf("can't find symbol %s", name)
		}
		val.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Value:
				typ, isBlueprint := ssa.ToClassBluePrintType(ret.innerValue.GetType())
				if isBlueprint {
					extends = append(extends, typ)
				}
			default:
				return nil
			}
			return nil
		})
		check := func(p ssa.Type) bool {
			typ, isBlueprint := ssa.ToClassBluePrintType(p)
			if isBlueprint {
				return false
			}
			for _, extend := range extends {
				if typ.CheckExtendedBy(extend) {
					return true
				}
			}
			return false
		}
		v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Value:
				node := ret.innerValue
				if check(node.GetType()) {
					result = append(result, ret)
				}
			default:
				return nil
			}
			return nil
		})
		return true, sfvm.NewValues(result), nil
	}))
	registerNativeCall(NativeCall_GetCurrentBlueprint, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var result []sfvm.ValueOperator
		prog, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		blueprints := getCurrentBlueprint(v)
		for _, blueprint := range blueprints {
			if val, err := prog.NewValue(blueprint.Container()); err == nil {
				result = append(result, val)
			}
		}
		if len(result) == 0 {
			return false, nil, utils.Errorf("no blueprint found")
		}
		return true, sfvm.NewValues(result), nil
	}))

	registerNativeCall(NativeCall_GetActualParams, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		result, err := v.GetCallActualParams(0, true)
		if err != nil {
			return false, nil, err
		}
		return true, result, nil
	}), nc_desc("获取实际参数"))
	registerNativeCall(NativeCall_GetUsers, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		depth := params.GetInt(0, "depth")
		var result []sfvm.ValueOperator
		v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Value:
				result = append(result, ret.GetUsers())
			case *Values:
				result = append(result, ret.GetUsers())
			}
			return nil
		})
		if depth > 0 {
			depth--
		}

		for ; depth > 0; depth-- {
			var temp []sfvm.ValueOperator
			for _, v := range result {
				switch ret := v.(type) {
				case *Value:
					temp = append(temp, ret.GetUsers())
				case Values:
					temp = append(temp, ret.GetUsers())
				case *sfvm.ValueList:
					values, err := SFValueListToValues(ret)
					if err == nil {
						values.ForEach(func(value *Value) {
							temp = append(temp, value.GetUsers())
						})
					}
				}
			}
			result = temp
		}
		if len(result) > 0 {
			return true, sfvm.NewValues(result), nil
		}
		return false, nil, nil
	}), nc_desc("获取值的Users"))

	// NativeCall_GetPredecessors is used to get the predecessors of a value
	// <getPredecessors()> =  <getPredecessors(1)>
	registerNativeCall(NativeCall_GetPredecessors, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var result []*Value
		v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Value:
				result = append(result, ret)
			}
			return nil
		})
		if len(result) == 0 {
			return false, nil, utils.Errorf("no value found")
		}
		// default param is depth, depth default = 1
		depth := params.GetInt(0, "depth")
		if depth == -1 {
			depth = 1
		}

		// get v.GetPredecessors() through depth
		// this is tree
		visited := make(map[*Value]struct{})
		var allFoundPredecessors []sfvm.ValueOperator
		currentLevel := make([]*Value, 0, len(result))

		// Initialize visited and currentLevel with start nodes
		for _, node := range result {
			if _, exists := visited[node]; !exists {
				visited[node] = struct{}{}
				currentLevel = append(currentLevel, node)
			}
		}

		// Traverse predecessors level by level up to the specified depth
		for d := 0; d < depth; d++ {
			if len(currentLevel) == 0 {
				break // No more nodes to explore
			}
			nextLevel := make([]*Value, 0)
			for _, node := range currentLevel {
				predecessors := node.GetPredecessors() // Assuming GetPredecessors returns []*Value
				for _, p := range predecessors {
					pred := p.Node
					if _, exists := visited[pred]; !exists {
						visited[pred] = struct{}{}
						allFoundPredecessors = append(allFoundPredecessors, pred)
						nextLevel = append(nextLevel, pred)
					}
				}
			}
			currentLevel = nextLevel
		}

		if len(allFoundPredecessors) == 0 {
			return false, nil, utils.Errorf("no predecessors found within depth %d", depth)
		}

		return true, sfvm.NewValues(allFoundPredecessors), nil

	}), nc_desc("获取值的前驱节点"))

	// NativeCall_GetFullFileName is used to get the full file name, the input is a file name. eg.
	// <getFullFileName(filename="xxx")>
	registerNativeCall(NativeCall_GetFullFileName, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var rs []sfvm.ValueOperator
		targetName := params.GetString("filename")
		if targetName == "" {
			return false, nil, utils.Errorf("filename is empty")
		}
		program, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		p := program.Program
		if p == nil {
			return false, nil, utils.Errorf("program is nil")
		}
		matchFilename := func(f func(filename string) bool) {
			program.ForEachAllFile(func(s string, me *memedit.MemEditor) bool {
				if !f(s) {
					return true
				}
				rs = append(rs, program.NewConstValue(s, me.GetFullRange()))
				return true
			})
		}
		if compile, err := glob.Compile(targetName); err == nil {
			matchFilename(func(filename string) bool {
				return compile.Match(filename)
			})
		}
		if r, err := regexp.Compile(targetName); err == nil {
			matchFilename(func(filename string) bool {
				return r.MatchString(filename)
			})
		}
		matchFilename(func(filename string) bool {
			return strings.EqualFold(filename, targetName)
		})
		return true, sfvm.NewValues(rs), nil
	}))
	registerNativeCall(NativeCall_GetFilenameByContent, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var rs []sfvm.ValueOperator

		program, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		prog := program.Program
		v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Program:
				return nil
			case *Value:
				vr := ret.innerValue.GetRange()
				if vr == nil {
					log.Errorf("node range is nil")
					return nil
				}
				editor := vr.GetEditor()
				if editor == nil {
					log.Errorf("node editor is nil")
				}
				_, exist := prog.FileList[editor.GetUrl()]
				if exist {
					rs = append(rs, program.NewConstValue(editor.GetFilename(), editor.GetFullRange()))
				} else {
					log.Errorf("program filelist not found this file")
				}
			}
			return nil
		})
		return true, sfvm.NewValues(rs), nil
	}))
	//<foreach_function_inst(hook=`xxx` as $result)> as $result
	registerNativeCall(NativeCall_Foeach_Func_Inst, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var result []sfvm.ValueOperator
		prog, err := fetchProgram(v)
		if err != nil {
			return false, nil, err
		}
		v.Recursive(func(operator sfvm.ValueOperator) error {
			value, ok := operator.(*Value)
			if !ok {
				return nil
			}
			function, flag := ssa.ToFunction(value.innerValue)
			if !flag {
				return nil
			}
			enter, ok := function.GetBasicBlockByID(function.EnterBlock)
			if !ok || enter == nil {
				return nil
			}
			result1 := searchAlongBasicBlock(enter.GetBlock(), prog, frame, params, Next)
			result = append(result, result1...)
			return nil
		})
		return true, sfvm.NewValues(result), nil
	}))
	registerNativeCall(NativeCall_Java_UnEscape_Output, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var res []sfvm.ValueOperator

		// 模板语言输出的标志位
		flag := template2java.JAVA_REQUEST_PATH
		unEscapeKey := template2java.JAVA_UNESCAPE_OUTPUT_PRINT

		checkUnEscape := func(value *Value) bool {
			t := value.GetType()
			if t == nil || t.t == nil {
				return false
			}
			name := t.t.GetFullTypeNames()
			if len(name) == 0 {
				return false
			}
			for _, n := range name {
				if strings.Contains(n, flag) {
					return true
				}
			}
			return false
		}

		getCalledAndCheck := func(value *Value) []sfvm.ValueOperator {
			var vals []sfvm.ValueOperator
			if !checkUnEscape(value) {
				return vals
			}
			callInst := value.GetCalledBy()
			callInst.ForEach(func(call *Value) {
				vals = append(vals, call.GetCallArgs())
			})

			return vals
		}

		match, outValue, err := v.GlobMatch(frame.GetContext(), ssadb.NameMatch, `out`)
		if !match || err != nil {
			return false, nil, utils.Errorf("no value found")
		}
		match, printValue, err := outValue.GlobMatch(frame.GetContext(), ssadb.KeyMatch, unEscapeKey)
		if !match || err != nil {
			return false, nil, utils.Errorf("no value found")
		}

		switch i := printValue.(type) {
		case *Value:
			vals := getCalledAndCheck(i)
			res = append(res, vals...)
		case Values:
			i.Recursive(func(operator sfvm.ValueOperator) error {
				value, ok := operator.(*Value)
				if !ok {
					return nil
				}
				vals := getCalledAndCheck(value)
				res = append(res, vals...)
				return nil
			})
		case *sfvm.ValueList:
			values, err := SFValueListToValues(i)
			if err == nil {
				values.ForEach(func(value *Value) {
					vals := getCalledAndCheck(value)
					res = append(res, vals...)
				})
			}
		default:
			return false, nil, utils.Errorf("invalid value type %T", i)
		}

		if len(res) > 0 {
			return true, sfvm.NewValues(res), nil
		}
		return false, nil, nil
	}), nc_desc("获取Java模板语言中未转义的输出"))

	registerNativeCall(NativeCall_IsSanitizeName, nc_func(nativeCallSanitizeNames), nc_desc("检查是否为潜在的过滤函数名称"))

	registerNativeCall(NativeCall_VersionIn, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		gt := params.GetString("greaterThan")  // <
		ge := params.GetString("greaterEqual") // <=
		if gt != "" && ge != "" {
			return false, nil, utils.Errorf("lt and le cannot be used at the same time")
		}
		vstart := "0.0.0"
		geFlag := false
		if gt != "" {
			vstart = gt
		} else if ge != "" {
			vstart = ge
			geFlag = true
		}

		lt := params.GetString("lessThan")  // >
		le := params.GetString("lessEqual") // >=
		if lt != "" && le != "" {
			return false, nil, utils.Errorf("gt and ge cannot be used at the same time")
		}
		vend := "99999999.999.999"
		leFlag := false
		if lt != "" {
			vend = lt
		} else if le != "" {
			vend = le
			leFlag = true
		}
		vstart = yakunquote.TryUnquote(vstart)
		vend = yakunquote.TryUnquote(vend)
		compareIn := func(version string) bool {
			c1, err := utils.VersionCompare(version, vstart)
			if err != nil {
				return false
			}
			c2, err := utils.VersionCompare(vend, version)
			if c1 == 0 && !geFlag {
				return false
			}
			if c2 == 0 && !leFlag {
				return false
			}
			return c1 != -1 && c2 != -1
		}

		var results []sfvm.ValueOperator
		switch i := v.(type) {
		case *Values:
			i.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				ssaValue := val.GetSSAInst()
				if ssaValue.GetOpcode() != ssa.SSAOpcodeConstInst {
					return nil
				}
				ver := fmt.Sprint(ssaValue)
				if compareIn(ver) {
					results = append(results, val)
				}
				return nil
			})
		case *Value:
			ssaValue := i.GetSSAInst()
			if ssaValue.GetOpcode() != ssa.SSAOpcodeConstInst {
				return false, nil, utils.Error("not value in version range")
			}
			ver := fmt.Sprint(ssaValue)
			if compareIn(ver) {
				results = append(results, i)
			}
		}
		if len(results) > 0 {
			return true, sfvm.NewValues(results), nil
		}
		return false, nil, utils.Error("not value in version range")
	}), nc_desc("获取版本信息"))
	registerNativeCall(NativeCall_Const, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		var (
			results    []sfvm.ValueOperator
			mode, rule string
		)

		constHandler := func(operator sfvm.ValueOperator) {
			switch mode {
			case "e":
				if match, valueOperator, err := operator.ExactMatch(frame.GetContext(), ssadb.ConstType, rule); match && err == nil {
					results = append(results, valueOperator)
				}
			case "g":
				if match, valueOperator, err := operator.GlobMatch(frame.GetContext(), ssadb.ConstType, rule); match && err == nil {
					results = append(results, valueOperator)
				}
			case "r":
				if match, valueOperator, err := operator.RegexpMatch(frame.GetContext(), ssadb.ConstType, rule); match && err == nil {
					results = append(results, valueOperator)
				}
			}
		}
		getRule := func(preMode ...string) string {
			for _, s := range preMode {
				if params.GetString(s) != "" {
					mode = s
					return params.GetString(s)
				}
			}
			mode = ""
			return yakunquote.TryUnquote(params.GetString(0))
		}
		autoMode := func(rule string) {
			if _, err := glob.Compile(rule); err == nil {
				mode = "g"
				return
			}
			if _, err := regexp.Compile(rule); err == nil {
				mode = "r"
				return
			}
			mode = "e"
		}
		rule = getRule("g", "r", "e")
		if mode == "" {
			autoMode(rule)
		}

		v.Recursive(func(operator sfvm.ValueOperator) error {
			switch ret := operator.(type) {
			case *Program:
				constHandler(ret)
			case *Value:
				if ret.IsConstInst() {
					constHandler(ret)
				}
			}
			return nil
		})
		return true, sfvm.NewValues(results), nil
	}))
	registerNativeCall(NativeCall_DataFlow, nc_func(nativeCallDataFlow))
	registerNativeCall(NativeCall_Self, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		return true, v, nil
	}))
	registerNativeCall(NativeCall_Forbid, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		name := params.GetString(0, "var")
		if name != "" {
			result, _ := frame.GetSFResult()
			if result != nil {
				vars, ok := result.SymbolTable.Get(name)
				if ok && haveResult(vars) {
					return false, nil, utils.Wrapf(sfvm.CriticalError, "forbid sf-var: %v", name)
				}
				if vars, ok := result.SymbolTable.Get(name); ok && haveResult(vars) {
					return false, nil, utils.Wrapf(sfvm.CriticalError, "forbid sf-var: %v", name)
				}
			}
			if vars, ok := frame.GetSymbolTable().Get(name); ok && haveResult(vars) {
				return false, nil, utils.Wrapf(sfvm.CriticalError, "forbid sf-var: %v", name)
			}
			return true, v, nil
		}

		if haveResult(v) {
			return false, nil, utils.Wrapf(sfvm.CriticalError, "forbid")
		}
		return true, v, nil
	}))
	registerNativeCall(NativeCall_DeleteVariable, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		name := params.GetString("name", 0)
		if name != "" {
			frame.GetSymbolTable().Delete(name)
			result, _ := frame.GetSFResult()
			if result != nil {
				result.SymbolTable.Delete(name)
				result.AlertSymbolTable.Delete(name)
				delete(result.GetRule().AlertDesc, name)
			}
		}
		return true, v, nil
	}))
	registerNativeCall(NativeCall_ScanNext, nc_func(nativeCallScan(Next)))
	registerNativeCall(NativeCall_ScanPrevious, nc_func(nativeCallScan(Previous)))
	registerNativeCall(NativeCall_ScanInstruction, nc_func(nativeCallScan(Current)))
	registerNativeCall(NativeCall_SourceCode, nc_func(nativeCallSourceCode))
	registerNativeCall(NativeCall_OpCodes, nc_func(nativeCallOpCodes))

	//nativeCall-slice
	registerNativeCall(NativeCall_Slice, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		start := params.GetInt(0, "start")
		index := params.GetInt(0, "index")

		if index == -1 && start == -1 {
			return false, nil, utils.Errorf("start or index is required")
		}
		idx := 0
		var vals []sfvm.ValueOperator
		_ = v.Recursive(func(operator sfvm.ValueOperator) error {
			if idx >= start && start != -1 {
				vals = append(vals, operator)
			}
			if idx == index && index != -1 {
				vals = append(vals, operator)
				return utils.Error("abort")
			}
			idx++
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value found")
	}))
	registerNativeCall(NativeCall_MyBatisSink, nc_func(nativeCallMybatisXML), nc_desc("Fins MyBatis Sink for default searching"))
	registerNativeCall(NativeCall_FreeMarkerSink, nc_func(nativeCallFreeMarker))
	registerNativeCall(NativeCall_Var, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		varName := params.GetString(0)
		//log.Info("syntax flow native call 'as' to", varName)

		var vals []sfvm.ValueOperator
		result, ok := frame.GetSymbolTable().Get(varName)
		if ok && haveResult(result) {
			_ = result.Recursive(func(operator sfvm.ValueOperator) error {
				_, ok := operator.(*Value)
				if ok {
					vals = append(vals, operator)
				}
				return nil
			})
		}
		_ = v.Recursive(func(operator sfvm.ValueOperator) error {
			_, ok := operator.(*Value)
			if ok {
				vals = append(vals, operator)
			}
			return nil
		})
		frame.GetSymbolTable().Set(varName, sfvm.NewValues(vals))
		return true, v, nil
	}), nc_desc(`put vars to variables`))
	registerNativeCall(NativeCall_StrLower, nc_func(nativeCallStrLower), nc_desc(`convert a string to lower case`))
	registerNativeCall(NativeCall_StrUpper, nc_func(nativeCallStrUpper), nc_desc(`convert a string to upper case`))
	registerNativeCall(NativeCall_Regexp, nc_func(nativeCallRegexp), nc_desc(`regexp a string, group is available`))

	registerNativeCall(NativeCall_Show, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		idx := 0
		_ = v.Recursive(func(operator sfvm.ValueOperator) error {
			if ret, ok := operator.(*Value); ok {
				fmt.Printf("-%3d: %v\n", idx, ret.String())
				idx++
			}
			return nil
		})
		return true, v, nil
	}), nc_desc(`show the value, do nothing`))

	registerNativeCall(
		NativeCall_Eval,
		nc_func(nativeCallEval),
		nc_desc(`eval a new syntaxflow rule, you can use this to eval dynamic rule`),
	)

	registerNativeCall(
		NativeCall_Fuzztag,
		nc_func(nativeCallFuzztag),
		nc_desc(`eval a new yaklang fuzztag template, the variables is in SFFrameResult`),
	)

	registerNativeCall(
		NativeCall_Include,
		nc_func(nativeCallInclude),
		nc_desc(`include a syntaxflow-rule`),
	)

	registerNativeCall(
		NativeCall_String,
		nc_func(nativeCallString),
		nc_desc(`获取输入指令的字符串表示`),
	)

	registerNativeCall(
		NativeCall_Name,
		nc_func(nativeCallName),
		nc_desc(`获取输入指令的名称表示，例如函数名，变量名，或者字段名等`),
	)

	registerNativeCall(
		NativeCall_TypeName,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			/*
				java.io.File #-> File
			*/
			var vals []sfvm.ValueOperator
			var tmpMap = make(map[string]struct{})
			addVals := func(val *Value, typ string) {
				_, ok := tmpMap[typ]
				if ok {
					return
				}
				tmpMap[typ] = struct{}{}
				vx := val.NewConstValue(typ, val.GetRange())
				vx.AppendPredecessor(val, frame.WithPredecessorContext("typeName"))
				vals = append(vals, vx)
			}
			v.Recursive(func(operator sfvm.ValueOperator) error {
				switch val := operator.(type) {
				case *Value:
					typ := val.GetType()
					if typ == nil || typ.t == nil {
						return utils.Errorf("native call type name failed: the value have %s no type", val.String())
					}
					fts := typ.t.GetFullTypeNames()
					if len(fts) == 0 {
						addVals(val, typ.String())
					} else {
						for _, ft := range fts {
							ft = yakunquote.TryUnquote(ft)
							index := strings.Index(ft, ":")
							if index != -1 {
								ft = ft[:index]
								addVals(val, ft)
							}
							lastIndex := strings.LastIndex(ft, ".")
							if lastIndex != -1 && len(ft) > lastIndex+1 {
								addVals(val, ft[lastIndex+1:])
							}
							addVals(val, ft)
						}
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value found")
		}),
		nc_desc(`获取输入指令的类型名称表示，例如int，string，或者自定义类型等：

在 Java 中，会尽可能关联到类名或导入名称，可以根据这个确定使用的类行为。
`),
	)

	registerNativeCall(
		NativeCall_FullTypeName,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			var tmpMap = make(map[string]struct{})
			addVals := func(val *Value, typ string, rangeIf *memedit.Range) {
				if typ == "" {
					return
				}
				_, exist := tmpMap[typ]
				if exist {
					return
				}
				tmpMap[typ] = struct{}{}
				results := val.NewConstValue(typ, rangeIf)
				results.AppendPredecessor(val, frame.WithPredecessorContext("fullTypeName"))
				vals = append(vals, results)
			}
			err := v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				typ := val.GetType()
				if typ == nil || typ.t == nil {
					return utils.Errorf("native call type name failed: the value have %s no type", val.String())
				}
				fts := typ.t.GetFullTypeNames()
				if len(fts) == 0 {
					addVals(val, typ.String(), val.GetRange())
				} else {
					for _, ft := range fts {
						ft = yakunquote.TryUnquote(ft)
						addVals(val, ft, val.GetRange())
					}
				}

				return nil
			})
			if err != nil {
				return false, nil, err
			}
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value found")
		}),
		nc_desc(`获取输入指令的完整类型名称表示，例如int，string，或者自定义类型等

特殊地，在 Java 中，会尽可能使用全限定类名，例如 com.alibaba.fastjson.JSON, 也会尽可能包含 sca 版本`),
	)

	registerNativeCall(
		NativeCall_GetFormalParams,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				if val.getOpcode() == ssa.SSAOpcodeFunction {
					rets, ok := ssa.ToFunction(val.innerValue)
					if !ok {
						return nil
					}
					for _, param := range rets.Params {
						param, ok := rets.GetValueById(param)
						if !ok || param == nil {
							continue
						}
						newVal := val.NewValue(param)
						if newVal != nil {
							newVal.AppendPredecessor(val, frame.WithPredecessorContext("getFormalParams"))
							vals = append(vals, newVal)
						}
					}
				}
				return nil
			})
			if len(vals) > 0 {
				// fmt.Println("getFormalParams: ", vals)
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(formal params) found")
		}),
		nc_desc(`获取输入指令的形参，输入必须是一个函数指令`),
	)

	registerNativeCall(
		NativeCall_GetReturns,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				originIns := val.innerValue
				funcIns, ok := ssa.ToFunction(originIns)
				if !ok {
					return nil
				}
				for _, ret := range funcIns.Return {
					ret, ok := funcIns.GetValueById(ret)
					if !ok || ret == nil {
						continue
					}
					retVal, ok := ssa.ToReturn(ret)
					if !ok {
						continue
					}
					for _, retIns := range retVal.Results {
						retIns, ok := funcIns.GetValueById(retIns)
						if !ok || retIns == nil {
							continue
						}
						new := val.NewValue(retIns)
						if new != nil {
							new.AppendPredecessor(val, frame.WithPredecessorContext("getReturns"))
							vals = append(vals, new)
						}
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(returns) found")
		}),
		nc_desc(`获取输入指令的返回值，输入必须是一个函数指令`),
	)

	registerNativeCall(
		NativeCall_GetCallee,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}

				if val.IsCall() {
					call := val.GetCallee()
					if call != nil {
						call.AppendPredecessor(val, frame.WithPredecessorContext("getCallee"))
						vals = append(vals, call)
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(callers) found")
		}),
		nc_desc(`获取输入指令的调用者，输入必须是一个调用指令(call)`),
	)

	registerNativeCall(
		NativeCall_GetFunc,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				f := val.GetFunction()
				if f != nil {
					f.AppendPredecessor(val, frame.WithPredecessorContext("getFunc"))
					vals = append(vals, f)
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(func) found")
		}),
		nc_desc("获取输入指令的所在的函数，输入可以是任何指令"),
	)

	registerNativeCall(
		NativeCall_GetSiblings,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				obj := val.GetObject()
				if obj == nil {
					return nil
				}
				for _, elements := range obj.GetMembers() {
					for _, newVal := range elements {
						if newVal == nil {
							continue
						}
						newVal.AppendPredecessor(val, frame.WithPredecessorContext("getSiblings"))
						vals = append(vals, newVal)
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(siblings) found")
		}),
		nc_desc("获取输入指令的兄弟指令，一般说的是如果这个指令是一个对象的成员，可以通过这个指令获取这个对象的其他成员。"),
	)

	registerNativeCall(
		NativeCall_GetMembers,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var rets []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				for _, members := range val.GetMembers() {
					for _, member := range members {
						if member == nil {
							continue
						}
						member.AppendPredecessor(val, frame.WithPredecessorContext("getMembers"))
						rets = append(rets, member)
					}
				}
				return nil
			})
			if len(rets) > 0 {
				return true, sfvm.NewValues(rets), nil
			}
			return false, nil, utils.Error("no value(members) found")
		}),
		nc_desc("获取输入指令的成员指令，一般说的是如果这个指令是一个对象，可以通过这个指令获取这个对象的成员。"),
	)

	registerNativeCall(
		NativeCall_GetMemberByKey,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var rets []sfvm.ValueOperator
			key := actualParams.GetString(0, "key")

			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}

				if ret, ok := val.GetMembersByString(key); ok {
					ret.AppendPredecessor(val, frame.WithPredecessorContext("getMemberByKey"))
					rets = append(rets, ret)
				}

				return nil
			})
			if len(rets) > 0 {
				return true, sfvm.NewValues(rets), nil
			}
			return false, nil, utils.Error("no value(members) found")
		}),
		nc_desc("获取输入指令的成员指令，一般说的是如果这个指令是一个对象，可以通过这个指令获取这个对象的某个特定的成员。"),
	)

	registerNativeCall(
		NativeCall_GetObject,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var ret []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				obj := val.GetObject()
				if obj != nil {
					obj.AppendPredecessor(val, frame.WithPredecessorContext("getObject"))
					ret = append(ret, obj)
					return nil
				}
				return nil
			})
			if len(ret) > 0 {
				return true, sfvm.NewValues(ret), nil
			}
			return false, nil, utils.Error("no value(parent object) found")
		}),
		nc_desc(`获取输入指令的父对象，一般说的是如果这个指令是一个成员，可以通过这个指令获取这个成员的父对象。`),
	)
	registerNativeCall(
		NativeCall_GetCall,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				for _, u := range val.GetUsers() {
					if u.getOpcode() == ssa.SSAOpcodeCall {
						u.AppendPredecessor(val, frame.WithPredecessorContext("getCall"))
						vals = append(vals, u)
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(call) found")
		}),
		nc_desc(`获取输入指令的调用指令，输入必须是一个函数指令`),
	)
	registerNativeCall(
		NativeCall_SearchFunc,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				if val, ok := operator.(*Value); ok {
					switch ins := val.getOpcode(); ins {
					case ssa.SSAOpcodeParameterMember:
						param, ok := ssa.ToParameterMember(val.innerValue)
						if ok {
							funcName := param.GetFunc().GetName()
							if val.ParentProgram == nil {
								return utils.Error("ParentProgram is nil")
							}
							ok, next, _ := val.ParentProgram.ExactMatch(frame.GetContext(), sfvm.BothMatch, funcName)
							if ok {
								vals = append(vals, next)
							}
						}
					case ssa.SSAOpcodeParameter:
						param, ok := ssa.ToParameter(val.innerValue)
						if ok {
							funcIns := param.GetFunc()
							funcName := funcIns.GetName()
							if m := funcIns.GetMethodName(); m != "" {
								funcName = m
							}
							if val.ParentProgram == nil {
								return utils.Error("ParentProgram is nil")
							}
							ok, next, _ := val.ParentProgram.ExactMatch(frame.GetContext(), sfvm.BothMatch, funcName)
							if ok {
								next.AppendPredecessor(val, frame.WithPredecessorContext("searchCall: "+funcName))
								vals = append(vals, next)
							}
						}
					case ssa.SSAOpcodeCall:
						callee := val.GetCallee()
						if callee == nil {
							return nil
						}

						log.Warn("callee: ", callee.GetName(), callee.GetVerboseName(), callee.String())

						methodName := callee.GetName()
						if obj := callee.GetObject(); obj != nil {
							methodName, _ = strings.CutPrefix(methodName, fmt.Sprintf("#%d.", obj.GetId()))
						}

						prog := val.ParentProgram
						if prog == nil {
							return utils.Error("ParentProgram is nil")
						}
						haveNext, next, _ := prog.ExactMatch(frame.GetContext(), sfvm.BothMatch, methodName)
						if haveNext && next != nil {
							next.Recursive(func(operator sfvm.ValueOperator) error {
								callee, ok := operator.(*Value)
								if !ok {
									return nil
								}
								vals = append(vals, callee)
								return nil
							})
						}
					case ssa.SSAOpcodeConstInst:
						// name := val.GetName()
						funcName := val.String()
						if str, err := strconv.Unquote(funcName); err == nil {
							funcName = str
						}
						ok, next, _ := val.ParentProgram.ExactMatch(frame.GetContext(), sfvm.BothMatch, funcName)
						if ok {
							next.AppendPredecessor(val, frame.WithPredecessorContext("searchCall: "+funcName))
							vals = append(vals, next)
						}
					default:
						//for _, call := range val.GetCalledBy() {
						//	call.AppendPredecessor(val, frame.WithPredecessorContext("searchCall"))
						//	funcIns := call.GetCallee()
						//	name := funcIns.GetName()
						//	log.Info(name)
						//	vals = append(vals, call)
						//}
					}
				}
				return nil
			})

			if len(vals) == 0 {
				return false, new(Values), utils.Errorf("no value found")
			}
			return true, sfvm.NewValues(vals), nil
		}),
		nc_desc(`搜索输入指令的调用指令，输入可以是任何指令，但是会尽可能搜索到调用这个指令的调用指令`),
	)
}

func fetchProgram(v sfvm.ValueOperator) (*Program, error) {
	var parent *Program
	v.Recursive(func(operator sfvm.ValueOperator) error {
		switch ret := operator.(type) {
		case *Value:
			parent = ret.ParentProgram
			return utils.Error("normal abort")
		case *Program:
			parent = ret
			return utils.Error("normal abort")
		}
		return nil
	})
	if parent == nil {
		return nil, utils.Error("no parent program found")
	}
	return parent, nil
}

func isProgram(v sfvm.ValueOperator) bool {
	_, ok := v.(*Program)
	if !ok {
		_ = v.Recursive(func(operator sfvm.ValueOperator) error {
			_, ok = operator.(*Program)
			return utils.Error("normal abort")
		})
	}
	return ok
}

func registerNativeCall(name string, options ...func(*NativeCallDocument)) {
	if name == "" {
		return
	}
	n := &NativeCallDocument{
		Name: name,
	}
	for _, o := range options {
		o(n)
	}
	NativeCallDocuments[name] = n
	sfvm.RegisterNativeCall(n.Name, n.Function)
}

func haveResult(operator sfvm.ValueOperator) bool {
	if utils.IsNil(operator) {
		return false
	}
	haveResultFlag := false
	_ = operator.Recursive(func(operator sfvm.ValueOperator) error {
		if _, ok := operator.(*Value); ok {
			haveResultFlag = true
			return utils.Error("abort")
		}
		return nil
	})
	return haveResultFlag
}
