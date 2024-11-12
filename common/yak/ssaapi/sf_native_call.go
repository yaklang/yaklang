package ssaapi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/gobwas/glob"
	"github.com/yaklang/yaklang/common/utils/yakunquote"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"

	"github.com/yaklang/yaklang/common/log"
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

	// NativeCall_GetCaller is used to get the caller of a value
	// find the caller instruction which contains the value
	NativeCall_GetCaller = "getCaller"

	// NativeCall_SearchFunc is used to search the call of a value, generally used to search the call of a function
	// if the input is a call already, check the 'call' 's method(function) 's other call(search mode)
	//
	// searchCall is not like getCall, search call will search all function name(from call) in the program
	NativeCall_SearchFunc = "searchFunc"

	// NativeCall_GetObject is used to get the object of a value
	NativeCall_GetObject = "getObject"

	// NativeCall_GetMembers is used to get the members of a value
	NativeCall_GetMembers = "getMembers"

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

	//NativeCall_DeleteVariable is used to delete a variable
	NativeCall_DeleteVariable = "delete"

	// NativeCall_Forbid is used to forbid a value, if values existed, report critical error.
	NativeCall_Forbid = "forbid"

	// NativeCall_Self is used to get self value
	NativeCall_Self = "self"

	// NativeCall_DataFlow is used to get data flow
	// if u want to fetch dataflow, call <dataflow...> after --> or #->
	// use it like: $data<dataflow(<<<CODE
	// *?{opcode: call && <getCaller><name>?{name} }
	// CODE)>
	NativeCall_DataFlow = "dataflow"

	// NativeCall_Const is used to search const value
	NativeCall_Const = "const"

	// NativeCall_VersionIn is used to get the version in
	NativeCall_VersionIn = "versionIn"

	// NativeCall_IsSanitizeName checks for potential sanitization function names
	NativeCall_IsSanitizeName = "isSanitizeName"
)

func init() {
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
				ssaValue := val.GetSSAValue()
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
			ssaValue := i.GetSSAValue()
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
				delete(result.AlertSymbolTable, name)
				delete(result.GetRule().AlertDesc, name)
			}
		}
		return true, v, nil
	}))
	registerNativeCall(NativeCall_ScanNext, nc_func(nativeCallScanNext))
	registerNativeCall(NativeCall_ScanPrevious, nc_func(nativeCallScanPrevious))
	registerNativeCall(NativeCall_SourceCode, nc_func(nativeCallSourceCode))
	registerNativeCall(NativeCall_OpCodes, nc_func(nativeCallOpCodes))
	registerNativeCall(NativeCall_Slice, nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, params *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
		start := params.GetInt(0, "start")
		idx := 0
		var vals []sfvm.ValueOperator
		_ = v.Recursive(func(operator sfvm.ValueOperator) error {
			if idx >= start {
				vals = append(vals, operator)
			}
			idx++
			return nil
		})
		if len(vals) > 0 {
			return true, sfvm.NewValues(vals), nil
		}
		return false, nil, utils.Error("no value found")
	}))
	registerNativeCall(NativeCall_MyBatisSink, nc_func(nativeCallMybatixXML), nc_desc("Fins MyBatis Sink for default searching"))
	registerNativeCall(NativeCall_FreeMarkerSink, nc_func(nativeCallFreeMarker))
	registerNativeCall(NativeCall_MyBatisSink, nc_func(nativeCallMybatixXML))
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
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				t := val.GetType()
				fts := t.t.GetFullTypeNames()
				var results []string
				if len(fts) == 0 {
					results = append(results, t.String())
				} else {
					for _, ft := range fts {
						//remove versioin name
						ft = yakunquote.TryUnquote(ft)
						index := strings.Index(ft, ":")
						if index != -1 {
							ft = ft[:index]
							results = append(results, ft)
						}

						// get type name
						lastIndex := strings.LastIndex(ft, ".")
						if lastIndex != -1 && len(ft) > lastIndex+1 {
							results = append(results, ft[lastIndex+1:])
						}
						results = append(results, ft)
					}
				}
				results = utils.RemoveRepeatStringSlice(results)
				for _, result := range results {
					v := val.NewValue(ssa.NewConst(result))
					v.AppendPredecessor(val, frame.WithPredecessorContext("typeName"))
					vals = append(vals, v)
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
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				t := val.GetType()
				fts := t.t.GetFullTypeNames()
				if len(fts) == 0 {
					results := val.NewValue(ssa.NewConst(t.String()))
					vals = append(vals, results)
				} else {
					for _, ft := range fts {
						ft = yakunquote.TryUnquote(ft)
						results := val.NewValue(ssa.NewConst(ft))
						results.AppendPredecessor(val, frame.WithPredecessorContext("fullTypeName"))
						vals = append(vals, results)
					}
				}

				return nil
			})
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
					rets, ok := ssa.ToFunction(val.node)
					if !ok {
						return nil
					}
					for _, param := range rets.Params {
						newVal := val.NewValue(param)
						newVal.AppendPredecessor(v, frame.WithPredecessorContext("getFormalParams"))
						vals = append(vals, newVal)
					}
				}
				return nil
			})
			if len(vals) > 0 {
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
				originIns := val.node
				funcIns, ok := ssa.ToFunction(originIns)
				if !ok {
					return nil
				}
				for _, ret := range funcIns.Return {
					retVal, ok := ssa.ToReturn(ret)
					if !ok {
						continue
					}
					for _, retIns := range retVal.Results {
						val := val.NewValue(retIns)
						val.AppendPredecessor(v, frame.WithPredecessorContext("getReturns"))
						vals = append(vals, val)
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
		NativeCall_GetCaller,
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
						call.AppendPredecessor(v, frame.WithPredecessorContext("getCaller"))
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
					for _, val := range elements {
						if val == nil {
							continue
						}
						val.AppendPredecessor(v, frame.WithPredecessorContext("getSiblings"))
						vals = append(vals, val)
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
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				for _, i := range val.GetMembers() {
					for _, val := range i {
						if val == nil {
							continue
						}
						val.AppendPredecessor(v, frame.WithPredecessorContext("getMembers"))
						vals = append(vals, val)
					}
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
			}
			return false, nil, utils.Error("no value(members) found")
		}),
		nc_desc("获取输入指令的成员指令，一般说的是如果这个指令是一个对象，可以通过这个指令获取这个对象的成员。"),
	)
	registerNativeCall(
		NativeCall_GetObject,
		nc_func(func(v sfvm.ValueOperator, frame *sfvm.SFFrame, actualParams *sfvm.NativeCallActualParams) (bool, sfvm.ValueOperator, error) {
			var vals []sfvm.ValueOperator
			v.Recursive(func(operator sfvm.ValueOperator) error {
				val, ok := operator.(*Value)
				if !ok {
					return nil
				}
				val = val.GetObject()
				if val != nil {
					val.AppendPredecessor(v, frame.WithPredecessorContext("getObject"))
					vals = append(vals, val)
					return nil
				}
				return nil
			})
			if len(vals) > 0 {
				return true, sfvm.NewValues(vals), nil
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
						u.AppendPredecessor(v, frame.WithPredecessorContext("getCall"))
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
						param, ok := ssa.ToParameterMember(val.node)
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
						param, ok := ssa.ToParameter(val.node)
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
