package sfvm

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"

	"github.com/yaklang/yaklang/common/log"
)

type OpCodes struct {
	Version string `json:"version"`
	Opcode  []*SFI `json:"opcode"`
}

func ToOpCodes(code string) (*OpCodes, bool) {
	var opcodes *OpCodes
	if err := json.Unmarshal([]byte(code), &opcodes); err != nil {
		log.Errorf("to opcode fail: %s", err)
		return nil, false
	}
	// check version
	if consts.GetYakVersion() != "dev" && opcodes.Version != "dev" && opcodes.Version != consts.GetYakVersion() {
		return nil, false
	}

	existHash := make(map[string]*IterIndex)
	for _, opcode := range opcodes.Opcode {
		switch opcode.OpCode {
		case OpCreateIter:
			iter := opcode.Iter
			if iter == nil {
				return nil, false
			}
			hash := iter.CalcHash()
			if _, exist := existHash[hash]; exist {
				log.Errorf("duplicate iter hash: %s", hash)
				return nil, false
			}
			existHash[hash] = iter
		case OpIterNext, OpIterEnd, OpIterLatch, OpCheckEmpty:
			if opcode.Iter == nil {
				return nil, false
			}
			calcHash := opcode.Iter.CalcHash()
			index, ok := existHash[calcHash]
			if !ok {
				log.Errorf("iter not exist: %s", calcHash)
				return nil, false
			}
			opcode.Iter = index
		}
	}
	return opcodes, true
}
func (y *SyntaxFlowVisitor) ToString() string {
	p := &OpCodes{
		Version: consts.GetYakVersion(),
		Opcode:  y.codes,
	}

	var result string
	if jsonBytes, err := json.Marshal(p); err == nil {
		result = string(jsonBytes)
	} else {
		log.Errorf("opcode to string fail: %s", err)
		result = fmt.Sprintf("%v", p)
	}
	return result
}

type SFVMOpCode int

const (
	OpPass SFVMOpCode = iota

	// enter/exit statement
	OpEnterStatement
	OpExitStatement

	// duplicate the top of stack
	OpDuplicate

	// OpPushSearchExact can push data from origin
	OpPushSearchExact
	OpPushSearchGlob
	OpPushSearchRegexp

	// OpRecursive... can fetch origin value (not program) push data from origin
	OpRecursiveSearchExact
	OpRecursiveSearchGlob
	OpRecursiveSearchRegexp

	// handle function call
	OpGetCall
	OpGetCallArgs

	// use def chain
	OpGetUsers
	OpGetBottomUsers
	OpGetDefs
	OpGetTopDefs

	// ListOperation
	OpListIndex

	// => variable
	OpNewRef
	OpUpdateRef

	// OpPushNumber and OpPushString and OpPushBool can push literal into stack
	OpPushNumber
	OpPushString
	OpPushBool
	OpPop

	// Condition
	// use the []bool  && []Value of stack top, push result into stack

	OpCondition
	OpCompareOpcode
	OpCompareString
	OpEmptyCompare

	OpVersionIn
	//OpPopDuplicate is copy popStack to stack
	OpPopDuplicate
	//OpCheckEmpty check the stack top, if empty, push false
	OpCheckEmpty

	OpEq
	OpNotEq
	OpGt
	OpGtEq
	OpLt
	OpLtEq
	OpLogicAnd
	OpLogicOr
	OpLogicBang

	/*
		Unary Operator: Fetch ONE in STACK, calc result, push result into stack
	*/
	OpReMatch
	OpGlobMatch
	OpNot

	OpAlert // echo variable

	// OpCheckParams check the params in vm context
	// if not match, record error
	// matched, use 'then expr' (if exists)
	OpCheckParams

	// OpAddDescription add description to current context
	OpAddDescription

	// OpCreateIter will create iterator for current context
	// the context contains origin values(list) and channel for elements
	OpCreateIter
	// OpIterNext will get next value from iterator
	// if the channel from iter context has a next element, push into stack and execute filter
	// if not, exit
	OpIterNext  // check next to end or end
	OpIterLatch // jump to next
	OpIterEnd   // end

	OpCheckStackTop // check the top of stack, if empty, push input into stack

	OpMergeRef
	OpRemoveRef
	OpIntersectionRef

	OpNativeCall

	//fileFilter
	OpFileFilterReg
	OpFileFilterXpath
	OpFileFilterJsonPath

	OpConditionStart
)

var Opcode2String = map[SFVMOpCode]string{
	OpPass:                  "OpPass",
	OpEnterStatement:        "OpEnterStatement",
	OpExitStatement:         "OpExitStatement",
	OpDuplicate:             "OpDuplicate",
	OpPushSearchExact:       "OpPushSearchExact",
	OpPushSearchGlob:        "OpPushSearchGlob",
	OpPushSearchRegexp:      "OpPushSearchRegexp",
	OpRecursiveSearchExact:  "OpRecursiveSearchExact",
	OpRecursiveSearchGlob:   "OpRecursiveSearchGlob",
	OpRecursiveSearchRegexp: "OpRecursiveSearchRegexp",
	OpGetCall:               "OpGetCall",
	OpGetCallArgs:           "OpGetCallArgs",
	OpGetUsers:              "OpGetUsers",
	OpGetBottomUsers:        "OpGetBottomUsers",
	OpGetDefs:               "OpGetDefs",
	OpGetTopDefs:            "OpGetTopDefs",
	OpListIndex:             "OpListIndex",
	OpNewRef:                "OpNewRef",
	OpUpdateRef:             "OpUpdateRef",
	OpPushNumber:            "OpPushNumber",
	OpPushString:            "OpPushString",
	OpPushBool:              "OpPushBool",
	OpPop:                   "OpPop",
	OpCondition:             "OpCondition",
	OpCompareOpcode:         "OpCompareOpcode",
	OpCompareString:         "OpCompareString",
	OpVersionIn:             "OpVersionIn",
	OpEq:                    "OpEq",
	OpNotEq:                 "OpNotEq",
	OpGt:                    "OpGt",
	OpGtEq:                  "OpGtEq",
	OpLt:                    "OpLt",
	OpLtEq:                  "OpLtEq",
	OpLogicAnd:              "OpLogicAnd",
	OpLogicOr:               "OpLogicOr",
	OpLogicBang:             "OpLogicBang",
	OpReMatch:               "OpReMatch",
	OpGlobMatch:             "OpGlobMatch",
	OpNot:                   "OpNot",
	OpAlert:                 "OpAlert",
	OpCheckParams:           "OpCheckParams",
	OpAddDescription:        "OpAddDescription",
	OpCreateIter:            "OpCreateIter",
	OpIterNext:              "OpIterNext",
	OpIterLatch:             "OpIterLatch",
	OpIterEnd:               "OpIterEnd",
	OpCheckStackTop:         "OpCheckStackTop",
	OpMergeRef:              "OpMergeRef",
	OpRemoveRef:             "OpRemoveRef",
	OpIntersectionRef:       "OpIntersectionRef",
	OpNativeCall:            "OpNativeCall",
	OpFileFilterReg:         "OpFileFilterReg",
	OpFileFilterXpath:       "OpFileFilterXpath",
	OpFileFilterJsonPath:    "OpFileFilterJsonPath",
	OpCheckEmpty:            "OpCheckEmpty",
	OpPopDuplicate:          "OpPopDuplicate",
	OpEmptyCompare:          "OpEmptyCompare",
}

type SFI struct {
	OpCode               SFVMOpCode             `json:"op_code"`
	UnaryInt             int                    `json:"unary_int"`
	UnaryStr             string                 `json:"unary_str"`
	UnaryBool            bool                   `json:"unary_bool"`
	Values               []string               `json:"values"`
	MultiOperator        []int                  `json:"multi_operator"`
	SyntaxFlowConfig     []*RecursiveConfigItem `json:"syntax_flow_config"`
	FileFilterMethodItem map[string]string      `json:"file_filter_method_item"`
	Iter                 *IterIndex             `json:"iter"`
}

func (s *SFI) IsIterOpcode() bool {
	switch s.OpCode {
	case OpCreateIter, OpIterLatch, OpIterNext, OpIterEnd:
		return true
	default:
		return false
	}
}

type IterIndex struct {
	// static
	Start int `json:"start"`
	Next  int `json:"next"`
	Latch int `json:"latch"`
	End   int `json:"end"`

	//记录当前的索引位置
	currentIndex int
}

func (i *IterIndex) CalcHash() string {
	return utils.CalcMd5(i.Start, i.Next, i.Latch, i.End)
}

func (s *SFI) ValueByIndex(i int) string {
	if i < 0 || i >= len(s.Values) {
		return ""
	}
	return s.Values[i]
}

const verboseLen = "%-12s"

func (s *SFI) String() string {
	switch s.OpCode {
	case OpEnterStatement:
		return "- enter -"
	case OpExitStatement:
		return "- exit -"
	case OpPass:
		return "- pass -"
	case OpPushBool:
		v := "false"
		if s.UnaryInt > 0 {
			v = "true"
		}
		return fmt.Sprintf(verboseLen+" %v", "push", v)
	case OpPushString:
		return fmt.Sprintf(verboseLen+" (len:%v) %v", "push", len(s.UnaryStr), strconv.Quote(s.UnaryStr))
	case OpPushNumber:
		return fmt.Sprintf(verboseLen+" %v", "push", s.UnaryInt)
	case OpDuplicate:
		return fmt.Sprintf(verboseLen+" %v", "duplicate", s.UnaryStr)
	case OpPushSearchGlob:
		return fmt.Sprintf(verboseLen+" %v isMember[%v]", "push$glob", s.UnaryStr, MatchModeString(s.UnaryInt))
	case OpPushSearchExact:
		return fmt.Sprintf(verboseLen+" %v isMember[%v]", "push$exact", s.UnaryStr, MatchModeString(s.UnaryInt))
	case OpPushSearchRegexp:
		return fmt.Sprintf(verboseLen+" %v isMember[%v]", "push$regexp", s.UnaryStr, MatchModeString(s.UnaryInt))
	case OpGetCall:
		return fmt.Sprintf(verboseLen+" %v", "getCall", s.UnaryStr)
	case OpGetCallArgs:
		return fmt.Sprintf(verboseLen+" %v withOther(%v)", "getCallArgs", s.UnaryInt, s.UnaryBool)
	case OpGetUsers:
		return fmt.Sprintf(verboseLen+" %v", "users", s.UnaryStr)
	case OpGetDefs:
		return fmt.Sprintf(verboseLen+" %v", "defs", s.UnaryStr)
	case OpGetTopDefs:
		return fmt.Sprintf(verboseLen+" %v", "topDefs", s.UnaryStr)
	case OpGetBottomUsers:
		return fmt.Sprintf(verboseLen+" %v", "bottomUse", s.UnaryStr)
	case OpListIndex:
		return fmt.Sprintf(verboseLen+" %v", "listIndex", s.UnaryStr)
	case OpNewRef:
		return fmt.Sprintf(verboseLen+" %v", "new$ref", s.UnaryStr)
	case OpUpdateRef:
		return fmt.Sprintf(verboseLen+" %v", "update$ref", s.UnaryStr)
	case OpCompareOpcode:
		return fmt.Sprintf(verboseLen+" %v", "compare opcode", s.Values)
	case OpCompareString:
		return fmt.Sprintf(verboseLen+" %v [%d] mul:%v", "compare string", s.Values, s.UnaryInt, s.MultiOperator)
	case OpCondition:
		return fmt.Sprintf(verboseLen+" %v", "condition", s.UnaryStr)
	case OpConditionStart:
		return fmt.Sprintf(verboseLen+" %v", "condition start", s.UnaryStr)
	case OpEq:
		return fmt.Sprintf(verboseLen+" %v", "(operator) ==", s.UnaryStr)
	case OpNotEq:
		return fmt.Sprintf(verboseLen+" %v", "(operator) !=", s.UnaryStr)
	case OpGt:
		return fmt.Sprintf(verboseLen+" %v", "(operator) >", s.UnaryStr)
	case OpGtEq:
		return fmt.Sprintf(verboseLen+" %v", "(operator) >=", s.UnaryStr)
	case OpLt:
		return fmt.Sprintf(verboseLen+" %v", "(operator) <", s.UnaryStr)
	case OpLtEq:
		return fmt.Sprintf(verboseLen+" %v", "(operator) <=", s.UnaryStr)
	case OpReMatch:
		return fmt.Sprintf(verboseLen+" %v", "(operator) ~=", s.UnaryStr)
	case OpGlobMatch:
		return fmt.Sprintf(verboseLen+" %v", "(operator) *~", s.UnaryStr)
	case OpNot:
		return fmt.Sprintf(verboseLen+" %v", "(operator) !", s.UnaryStr)
	case OpLogicAnd:
		return fmt.Sprintf(verboseLen+" %v", "(operator) &&", s.UnaryStr)
	case OpLogicOr:
		return fmt.Sprintf(verboseLen+" %v", "(operator) ||", s.UnaryStr)
	case OpLogicBang:
		return fmt.Sprintf(verboseLen+" %v", "(operator) !", s.UnaryStr)
	case OpPop:
		return fmt.Sprintf(verboseLen+" %v", "pop", s.UnaryStr)
	case OpCheckParams:
		var suffix string
		if ret := s.ValueByIndex(0); ret != "" {
			suffix += " then: " + ret
		}
		if ret := s.ValueByIndex(1); ret != "" {
			suffix += ", else: " + ret
		}
		return fmt.Sprintf(verboseLen+" $%v"+suffix, "check", s.UnaryStr)
	case OpAddDescription:
		var suffix string
		if ret := s.ValueByIndex(1); ret != "" {
			suffix += " value: " + ret
		} else if ret := s.ValueByIndex(0); ret != "" {
			suffix += " value: true"
		}
		return fmt.Sprintf(verboseLen+" %v"+suffix, "desc", s.UnaryStr)
	case OpAlert:
		return fmt.Sprintf(verboseLen+" %v", "alert", s.UnaryStr)
	case OpCreateIter:
		return fmt.Sprintf(verboseLen+" %v", "iter-start", s.UnaryStr)
	case OpIterEnd:
		return fmt.Sprintf(verboseLen+" %v", "iter-end", s.UnaryStr)
	case OpIterLatch:
		return fmt.Sprintf(verboseLen+" iter-latch to: %d", s.UnaryStr, s.Iter.Next)
	case OpIterNext:
		return fmt.Sprintf(verboseLen+" start: %v  latch:%v end: %v", "iter-next", s.Iter.Start, s.Iter.Latch, s.Iter.End)
	case OpCheckStackTop:
		return fmt.Sprintf(verboseLen+" %v", "check top", s.UnaryStr)
	case OpMergeRef:
		return fmt.Sprintf(verboseLen+" %v", "merge$ref", s.UnaryStr)
	case OpRemoveRef:
		return fmt.Sprintf(verboseLen+" %v", "remove$ref", s.UnaryStr)
	case OpIntersectionRef:
		return fmt.Sprintf(verboseLen+" %v", "intersection$ref", s.UnaryStr)
	case OpRecursiveSearchRegexp:
		return fmt.Sprintf(verboseLen+" %v", "recursive$regexp", s.UnaryStr)
	case OpRecursiveSearchGlob:
		return fmt.Sprintf(verboseLen+" %v", "recursive$glob", s.UnaryStr)
	case OpRecursiveSearchExact:
		return fmt.Sprintf(verboseLen+" %v", "recursive$exact", s.UnaryStr)
	case OpNativeCall:
		if s.UnaryStr == "include" {
			return fmt.Sprintf(verboseLen+" %v", "native$call", fmt.Sprintf("include %+v", codec.AnyToString(s.SyntaxFlowConfig)))
		}
		return fmt.Sprintf(verboseLen+" %v", "native$call", s.UnaryStr)
	case OpFileFilterReg:
		return fmt.Sprintf(verboseLen+" %v", "fileFilter$regexp", s.UnaryStr)
	case OpFileFilterXpath:
		return fmt.Sprintf(verboseLen+" %v", "fileFilter$xpath", s.UnaryStr)
	case OpFileFilterJsonPath:
		return fmt.Sprintf(verboseLen+" %v", "fileFilter$jsonpath", s.UnaryStr)
	case OpVersionIn:
		return fmt.Sprintf(verboseLen+" ", "version$in")
	case OpCheckEmpty:
		return fmt.Sprintf(verboseLen+" ", "check empty")
	case OpPopDuplicate:
		return fmt.Sprintf(verboseLen+" ", "pop-duplicate")
	case OpEmptyCompare:
		return fmt.Sprintf(verboseLen+" ", "empty compare")
	default:
		panic("unhandled default case")
	}
	return ""
}
