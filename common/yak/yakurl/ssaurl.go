package yakurl

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/log"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
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
		result, err := ssaapi.LoadResultByID(resultID)
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
	resultID, err = result.Save(schema.SFResultKindQuery)
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

	// option
	haveRange      bool
	useVerboseName bool
	// extra info
	programName string
	ResultID    uint

	// result
	Result *ssaapi.SyntaxFlowResult

	riskHash string
}

func (a *SyntaxFlowAction) GetQuerySyntaxFlow(params *ypb.RequestYakURLParams) (*QuerySyntaxFlow, error) {
	u := params.GetUrl()

	// query
	query := make(map[string]string)
	for _, v := range u.GetQuery() {
		query[v.GetKey()] = v.GetValue()
	}

	useVerboseName := false
	if ret, ok := query["use_verbose_name"]; ok {
		useVerboseName = ret == "true"
	}
	haveRange := false
	if have_range, ok := query["have_range"]; ok {
		haveRange = have_range == "true"
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
	}

	riskHash := query["risk_hash"]

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
	result, resultID, _ := a.getResult(programName, string(params.GetBody()), resultID)

	q := &QuerySyntaxFlow{
		variable: variable,
		index:    index,

		haveRange:      haveRange,
		useVerboseName: useVerboseName,

		ResultID:    resultID,
		programName: programName,
		Result:      result,
		riskHash:    riskHash,
	}
	return q, nil
}

var _ Action = (*SyntaxFlowAction)(nil)

/*
Get SyntaxFlowAction

	Request :
		url : "syntaxflow://program_id/variable/index"
		body: syntaxflow code // if set this will query result with this syntaxflow code
		query:
			result_id	string  // if set this, will get result from database
			have_range  bool    // if set this, will just return value contain code range
			use_verbose_name bool // if set this, will use verbose name in response
		page:
			start from
	Response:
		1. "syntaxflow://program_id/" :`
			* ResourceType: (message / variable) +  result_id
			all variable names
		2. "syntaxflow://program_id/variable_name" :
			* ResourceType: value +  result_id
			all values in this variable
		3. "syntaxflow://program_id/variable_name/index" :
			* ResourceType: information + result_id
			this value information, contain message && graph && node-info
*/
func (a *SyntaxFlowAction) Get(params *ypb.RequestYakURLParams) (resp *ypb.RequestYakURLResponse, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = utils.Errorf("recover: %v", r)
			utils.PrintCurrentGoroutineRuntimeStack()
		}
	}()

	query, err := a.GetQuerySyntaxFlow(params)
	if err != nil {
		return nil, err
	}
	if query.Result != nil {
		return a.GetResultBySFResult(query, params)
	} else if query.riskHash != "" {
		return a.GetResultByRiskHash(query, params)
	} else {
		return nil, utils.Errorf("query syntaxflow failed, sf result not found  or riskHash %s not found", query.riskHash)
	}
}

func (a *SyntaxFlowAction) GetResultBySFResult(
	query *QuerySyntaxFlow,
	params *ypb.RequestYakURLParams,
) (*ypb.RequestYakURLResponse, error) {
	variable := query.variable
	index := query.index
	result := query.Result
	programName := query.programName
	url := params.GetUrl()

	// page start from 1
	if params.Page <= 1 {
		params.Page = 1
	}

	var resources []*ypb.YakURLResource

	finish := true
	switch {
	case variable == "":
		// "syntaxflow://program_id/"
		// response:  all variable names
		resources = Variable2Response(result, url)
	case index == -1:
		// "syntaxflow://program_id/variable_name"
		// response: variable values
		valueLen := result.GetValueCount(variable)
		finish = false
		if params.PageSize <= 0 {
			params.PageSize = int64(valueLen)
		}
		for i := 0; i < int(params.PageSize); i++ {
			index := int(params.Page-1)*int(params.PageSize) + i
			if valueLen <= int(index) {
				finish = true
				break
			}
			v, err := result.GetValue(variable, index)
			if v == nil || err != nil {
				continue
			}
			codeRange, source := ssaapi.CoverCodeRange(v.GetRange())
			if query.haveRange && codeRange.URL == "" {
				continue
			}
			extraData := []extra{
				{"index", index},
				{"code_range", codeRange},
				{"source", source}}
			if hash := result.GetRiskHash(variable, int(index)); hash != "" {
				extraData = append(extraData, extra{"risk_hash", hash})
			}

			res := createNewRes(url, 0, extraData)
			res.ResourceType = "value"
			if query.useVerboseName {
				res.ResourceName = v.GetInnerValueVerboseName()
			} else {
				res.ResourceName = v.String()
			}
			resources = append(resources, res)
		}

		if params.Page*params.PageSize >= int64(valueLen) {
			finish = true
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
	if finish && query.ResultID != 0 {
		// when have resultId, this item mark the end.
		res := createNewRes(url, 0, []extra{})
		res.ResourceType = "result_id"
		res.VerboseType = "result_id"
		res.ResourceName = strconv.FormatUint(uint64(query.ResultID), 10)
		resources = append(resources, res)
	}
	// res.CheckParams
	// for _, msg := range resources {
	// 	if len(msg.Extra) > 3 {
	// 		fmt.Println(msg.Extra[1].Value)
	// 	}
	// }
	return &ypb.RequestYakURLResponse{
		Page:      params.Page,
		PageSize:  params.PageSize,
		Total:     int64(len(resources)),
		Resources: resources,
	}, nil
}

func (a *SyntaxFlowAction) GetResultByRiskHash(
	query *QuerySyntaxFlow,
	params *ypb.RequestYakURLParams,
) (*ypb.RequestYakURLResponse, error) {
	riskHash := query.riskHash
	programName := query.programName

	risk, err := yakit.QuerySSARiskByRiskHash(ssadb.GetDB(), riskHash)
	if err != nil {
		return nil, err
	}
	value, err := GetTmpValueByRiskHash(programName, risk)
	if err != nil {
		return nil, err
	}
	resources := []*ypb.YakURLResource{
		Value2Response(query.programName, value, risk.GetAlertMsg(), params.GetUrl()),
	}
	return &ypb.RequestYakURLResponse{
		Resources: resources,
	}, nil
}

func GetTmpValueByRiskHash(programName string, risk *schema.SSARisk) (*ssaapi.Value, error) {
	var auditNodeID string

	auditNodeIDs, err := ssadb.GetResultNodeByRiskHash(ssadb.GetDB(), risk.Hash)
	if err != nil {
		return nil, err
	}
	if len(auditNodeIDs) == 0 {
		return nil, utils.Error("auditNodeIDs is empty")
	} else if len(auditNodeIDs) == 1 {
		auditNodeID = auditNodeIDs[0]
	} else if len(auditNodeIDs) > 1 {
		log.Errorf("multiple auditNodeIDs found: %v", auditNodeIDs)
		auditNodeID = auditNodeIDs[0]
	}

	prog := ssaapi.NewTmpProgram(programName)
	value := prog.NewValueFromAuditNode(auditNodeID)
	return value, nil
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

func Value2Response(programName string, value *ssaapi.Value, msg string, url *ypb.YakURL) *ypb.YakURLResource {
	graphInfo := value.GetGraphInfo()
	res := createNewRes(url, 0, []extra{
		{"node_id", graphInfo.NodeID},
		{"graph", graphInfo.Graph},
		{"graph_info", graphInfo.GraphInfo},
		{"message", msg},
		{"graph_line", graphInfo.GraphPath},
	})

	res.ResourceType = "information"
	res.ResourceName = value.String()
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
