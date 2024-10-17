package yakurl

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dot"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/utils/orderedmap"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowAction struct {
	ProgramCache  *utils.CacheWithKey[string, *ssaapi.Program]          // name - program
	QueryCache    *utils.CacheWithKey[string, *ssaapi.SyntaxFlowResult] // hash - result
	ResultIDCache *utils.CacheWithKey[uint, *ssaapi.SyntaxFlowResult]   // result_id - result
}

func NewSyntaxFlowAction() *SyntaxFlowAction {
	ttl := 5 * time.Minute
	ret := &SyntaxFlowAction{
		ProgramCache:  utils.NewTTLCacheWithKey[string, *ssaapi.Program](ttl),
		QueryCache:    utils.NewTTLCacheWithKey[string, *ssaapi.SyntaxFlowResult](ttl),
		ResultIDCache: utils.NewTTLCacheWithKey[uint, *ssaapi.SyntaxFlowResult](ttl),
	}
	return ret
}

func (a *SyntaxFlowAction) getProgram(name string) (*ssaapi.Program, error) {
	if prog, ok := a.ProgramCache.Get(name); ok {
		return prog, nil
	}

	ssadb.CheckAndSwitchDB(name)
	prog, err := ssaapi.FromDatabase(name)
	if err != nil {
		return nil, utils.Wrapf(err, "get program %s", name)
	}
	a.ProgramCache.Set(name, prog)
	return prog, nil
}

func (a *SyntaxFlowAction) querySF(programName, code string) (*ssaapi.SyntaxFlowResult, error) {
	hash := codec.Md5(programName + code)
	if res, ok := a.QueryCache.Get(hash); ok {
		return res, nil
	}

	prog, err := a.getProgram(programName)
	if err != nil {
		return nil, err
	}
	res, err := prog.SyntaxFlowWithError(code)
	if err != nil {
		return nil, err
	}
	a.QueryCache.Set(hash, res)
	return res, nil
}

func (a *SyntaxFlowAction) getResult(programName, code string, resultID uint) (*ssaapi.SyntaxFlowResult, uint, error) {
	// get result
	if resultID != 0 {
		if res, ok := a.ResultIDCache.Get(resultID); ok {
			// get result from cache
			return res, resultID, nil
		}

		// get db result by ResultID
		result, err := ssaapi.CreateResultByID(resultID)
		if err != nil {
			return nil, 0, utils.Errorf("get result by id %d failed: %v", resultID, err)
		}
		a.ResultIDCache.Set(resultID, result)
		return result, resultID, nil
	}

	// query sf get memory result
	syntaxFlowCode := string(code)
	result, err := a.querySF(programName, syntaxFlowCode)
	if err != nil {
		return nil, 0, utils.Errorf("query syntaxflow failed: %v", err)
	}
	// save result to db
	resultID, err = result.Save()
	if err != nil {
		return nil, 0, utils.Errorf("save result failed: %v", err)
	}
	// save result to cache
	a.ResultIDCache.Set(resultID, result)

	return result, resultID, nil
}

type QuerySyntaxFlow struct {
	// query
	variable string
	index    int64

	// extra info
	programName string
	ResultID    uint

	// result
	Result *ssaapi.SyntaxFlowResult
}

func (a *SyntaxFlowAction) GetResult(params *ypb.RequestYakURLParams) (*QuerySyntaxFlow, error) {
	u := params.GetUrl()

	// query
	query := make(map[string]string)
	for _, v := range u.GetQuery() {
		query[v.GetKey()] = v.GetValue()
	}

	// get resultID from query
	var resultID uint = 0
	resultIDRaw, useResultID := query["result_id"]
	if useResultID {
		// parse result_id
		if res, err := strconv.ParseUint(resultIDRaw, 10, 64); err == nil {
			resultID = uint(res)
		} else {
			return nil, utils.Errorf("parse result_id %s failed: %v", resultIDRaw, err)
		}
		// check result_id
		if resultID == 0 {
			return nil, utils.Errorf("result_id can not be 0")
		}
	}

	// Parse variable and index from path
	path := u.Path
	variable := ""
	var index int64 = -1
	if path != "" {
		parts := strings.Split(path, "/")
		if len(parts) > 1 {
			variable = parts[1]
		}
		if len(parts) > 2 {
			if i, err := strconv.ParseInt(parts[2], 10, 64); err == nil {
				index = i
			} else {
				return nil, utils.Errorf("parse index %s failed: %v", parts[2], err)
			}
		}
	}

	// get program
	programName := u.GetLocation()
	result, resultID, err := a.getResult(programName, string(params.GetBody()), resultID)
	if err != nil {
		return nil, err
	}

	return &QuerySyntaxFlow{
		variable: variable,
		index:    index,

		ResultID:    resultID,
		programName: programName,

		Result: result,
	}, nil
}

var _ Action = (*SyntaxFlowAction)(nil)

/*
Get SyntaxFlowAction

	Request :
		url : "syntaxflow://program_id/variable/index"
		body: syntaxflow code // if set this will query result with this syntaxflow code
		query:
			result_id	string  // if set this, will get result from database
	Response:
		1. "syntaxflow://program_id/" :
			* ResourceType: (message / variable) +  result_id
			all variable names
		2. "syntaxflow://program_id/variable_name" :
			* ResourceType: value +  result_id
			all values in this variable
		3. "syntaxflow://program_id/variable_name/index" :
			* ResourceType: information + result_id
			this value information, contain message && graph && node-info
*/
func (a *SyntaxFlowAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {

	query, err := a.GetResult(params)
	if err != nil {
		return nil, err
	}

	variable := query.variable
	index := query.index
	result := query.Result
	programName := query.programName
	url := params.GetUrl()

	var resources []*ypb.YakURLResource

	switch {
	case variable == "":
		// "syntaxflow://program_id/"
		// response:  all variable names
		resources = Variable2Response(result, url)
	case index == -1:
		// "syntaxflow://program_id/variable_name"
		// response: variable values
		values := result.GetValues(variable)
		for index, v := range values {
			_ = v
			_ = index
			codeRange, source := coverCodeRange(programName, v.GetRange())
			res := createNewRes(url, 0, []extra{
				{"index", index},
				{"code_range", codeRange},
				{"source", source},
			})
			res.ResourceType = "value"
			res.ResourceName = v.String()
			resources = append(resources, res)
		}
	default:
		// "syntaxflow://program_id/variable_name/index"
		// response: variable value
		vs := result.GetValues(variable)
		if int(index) >= len(vs) {
			return nil, utils.Errorf("index out of range: %d", index)
		}
		value := vs[index]
		msg, _ := result.GetAlertMsg(variable)
		res := Value2Response(programName, value, msg, url)
		resources = append(resources, res)
	}

	// result_id
	if query.ResultID != 0 {
		res := createNewRes(url, 0, []extra{})
		res.ResourceType = "message"
		res.VerboseType = "result_id"
		res.ResourceName = strconv.FormatUint(uint64(query.ResultID), 10)
		resources = append(resources, res)
	}
	// res.CheckParams
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(resources)),
		Resources: resources,
	}, nil
}

func Variable2Response(result *ssaapi.SyntaxFlowResult, url *ypb.YakURL) []*ypb.YakURLResource {
	var resources []*ypb.YakURLResource

	// if contain check params, add check params
	for _, msg := range result.GetErrors() {
		res := createNewRes(url, 0, nil)
		res.ResourceType = "message"
		res.VerboseType = "error"
		res.VerboseName = msg
		resources = append(resources, res)
	}
	for _, msg := range result.GetCheckMsg() {
		res := createNewRes(url, 0, nil)
		res.ResourceType = "message"
		res.VerboseType = "info"
		res.VerboseName = msg
		resources = append(resources, res)
	}

	normalRes := make([]*ypb.YakURLResource, 0)
	if variables := result.GetAllVariable(); variables != nil {
		variables.ForEach(func(variable string, num any) {
			valueNum := num.(int)
			if variable == "_" {
				return
			}
			// vs := result.GetValues(name)
			res := createNewRes(url, valueNum, nil)
			res.ResourceType = "variable"
			res.ResourceName = variable
			if msg, ok := result.GetAlertMsg(variable); ok {
				res.VerboseType = "alert"
				res.VerboseName = codec.AnyToString(msg)
				if risk := result.GetRisk(variable); risk != nil {
					res.Extra = append(res.Extra, &ypb.KVPair{
						Key:   "risk_hash",
						Value: risk.Hash,
					})
				}
				resources = append(resources, res)
			} else {
				res.VerboseType = "normal"
				normalRes = append(normalRes, res)
			}
		})
	}
	sort.Slice(normalRes, func(i, j int) bool {
		return normalRes[i].ResourceName < normalRes[j].ResourceName
	})
	resources = append(resources, normalRes...)

	// last add unName values
	if vs := result.GetUnNameValues(); len(vs) > 0 {
		res := createNewRes(url, len(vs), nil)
		res.ResourceType = "variable"
		res.VerboseType = "unknown"
		res.ResourceName = "_"
		resources = append(resources, res)
	}
	return resources
}

type CodeRange struct {
	URL         string `json:"url"`
	StartLine   int64  `json:"start_line"`
	StartColumn int64  `json:"start_column"`
	EndLine     int64  `json:"end_line"`
	EndColumn   int64  `json:"end_column"`
}

func coverCodeRange(programName string, r memedit.RangeIf) (*CodeRange, string) {
	// url := ""
	source := ""
	ret := &CodeRange{
		URL:         "",
		StartLine:   0,
		StartColumn: 0,
		EndLine:     0,
		EndColumn:   0,
	}
	if r == nil {
		return ret, source
	}
	if editor := r.GetEditor(); editor != nil {
		ret.URL = fmt.Sprintf("/%s/%s", programName, editor.GetFilename())
		source = editor.GetTextContextWithPrompt(r, 1)
	}
	if start := r.GetStart(); start != nil {
		ret.StartLine = int64(start.GetLine())
		ret.StartColumn = int64(start.GetColumn())
	}
	if end := r.GetEnd(); end != nil {
		ret.EndLine = int64(end.GetLine())
		ret.EndColumn = int64(end.GetColumn())
	}
	return ret, source
}

type NodeInfo struct {
	NodeID     string     `json:"node_id"`
	IRCode     string     `json:"ir_code"`
	SourceCode string     `json:"source_code"`
	CodeRange  *CodeRange `json:"code_range"`
}

func coverNodeInfos(graph *ssaapi.ValueGraph, programName string, nodeID int) []*NodeInfo {
	res := make([]*NodeInfo, 0, len(graph.Node2Value))
	for id, node := range graph.Node2Value {
		codeRange, source := coverCodeRange(programName, node.GetRange())
		ret := &NodeInfo{
			NodeID:     dot.NodeName(id),
			IRCode:     node.String(),
			SourceCode: source,
			CodeRange:  codeRange,
		}
		res = append(res, ret)
	}
	return res
}

// deep first search for nodeID and its children to [][]id, id is string,
// if node.Prev have more than one, add a new line
type DeepFirst struct {
	res     [][]string
	current *orderedmap.OrderedMap // map[string]nil
	graph   *ssaapi.ValueGraph
}

func (d *DeepFirst) deepFirst(nodeID int) {
	if _, ok := d.current.Get(dot.NodeName(nodeID)); ok {
		return
	}
	d.current.Set(dot.NodeName(nodeID), nil)
	// d.current = append(d.current, dot.NodeName(nodeID))
	node := d.graph.GetNodeByID(nodeID)
	prevs := node.Prevs()
	if len(prevs) == 0 {
		d.res = append(d.res, d.current.Keys())
		return
	}
	if len(prevs) == 1 {
		prev := prevs[0]
		d.deepFirst(prev)
		return
	}

	// origin
	current := d.current
	for _, prev := range prevs {
		// new line
		d.current = orderedmap.New()
		d.current = current.Copy()
		d.deepFirst(prev)
	}
}

func DeepFirstGraph(graph *ssaapi.ValueGraph, nodeID int) [][]string {
	df := &DeepFirst{
		res:     make([][]string, 0),
		current: orderedmap.New(),
		graph:   graph,
	}
	df.deepFirst(nodeID)
	return df.res
}

func Value2Response(programName string, value *ssaapi.Value, msg string, url *ypb.YakURL) *ypb.YakURLResource {
	valueGraph := ssaapi.NewValueGraph(value)
	var buf bytes.Buffer
	valueGraph.GenerateDOT(&buf)

	nodeID := valueGraph.Value2Node[value.GetId()]
	nodeInfos := coverNodeInfos(valueGraph, programName, nodeID)
	graphLines := DeepFirstGraph(valueGraph, nodeID)

	res := createNewRes(url, 0, []extra{
		{"node_id", dot.NodeName(nodeID)},
		{"graph", buf.String()},
		{"graph_info", nodeInfos},
		{"message", msg},
		{"graph_line", graphLines},
	})

	res.ResourceType = "information"
	res.ResourceName = value.String()
	return res
}

type extra struct {
	name  string
	value any
}

func createNewRes(originParam *ypb.YakURL, size int, extra []extra) *ypb.YakURLResource {
	yakURL := &ypb.YakURL{
		Schema:   originParam.Schema,
		User:     originParam.GetUser(),
		Pass:     originParam.GetPass(),
		Location: originParam.GetLocation(),
		Path:     originParam.GetPath(),
		Query:    originParam.GetQuery(),
	}

	res := &ypb.YakURLResource{
		Size:              int64(size),
		ModifiedTimestamp: time.Now().Unix(),
		Path:              originParam.Path,
		YakURLVerbose:     "",
		Url:               yakURL,
	}
	if len(extra) > 0 {
		for _, v := range extra {
			res.Extra = append(res.Extra, &ypb.KVPair{
				Key:   v.name,
				Value: codec.AnyToString(v.value),
			})
		}
	}
	return res
}

func (a *SyntaxFlowAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (a *SyntaxFlowAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (a *SyntaxFlowAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (a *SyntaxFlowAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (a *SyntaxFlowAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}
