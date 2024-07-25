package yakurl

import (
	"strconv"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type SyntaxFlowAction struct {
	ProgramCache *utils.CacheWithKey[string, *ssaapi.Program]          // name - program
	QueryCache   *utils.CacheWithKey[string, *ssaapi.SyntaxFlowResult] // hash - result
}

func NewSyntaxFlowAction() *SyntaxFlowAction {
	ttl := 5 * time.Minute
	ret := &SyntaxFlowAction{
		ProgramCache: utils.NewTTLCacheWithKey[string, *ssaapi.Program](ttl),
		QueryCache:   utils.NewTTLCacheWithKey[string, *ssaapi.SyntaxFlowResult](ttl),
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
	res := prog.SyntaxFlow(code)
	a.QueryCache.Set(hash, res)
	return res, nil
}

var _ Action = (*SyntaxFlowAction)(nil)

func (a *SyntaxFlowAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	url := params.GetUrl()
	// "syntaxflow://program_id/variable/index" body:syntaxflow code
	programID := url.GetLocation()
	syntaxFlowCode := string(params.GetBody())
	path := url.Path
	// Parse variable and index from path
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

	result, err := a.querySF(programID, syntaxFlowCode)
	if err != nil {
		return nil, err
	}
	_ = result
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
			res := createNewRes(url, 0, map[string]string{
				"index": strconv.Itoa(index),
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
		if msg := result.AlertMsgTable[variable]; msg != "" {
		}
		_ = value
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
	for _, msg := range result.Errors {
		res := createNewRes(url, 0)
		res.ResourceType = "message"
		res.VerboseType = "error"
		res.VerboseName = msg
		resources = append(resources, res)
	}
	for _, name := range result.CheckParams {
		msg, ok := result.Description.Get("$" + name)
		if !ok {
			continue
		}
		res := createNewRes(url, 0)
		res.ResourceType = "message"
		res.VerboseType = "info"
		res.VerboseName = msg
		resources = append(resources, res)
	}

	normalRes := make([]*ypb.YakURLResource, 0)
	for _, name := range result.SymbolTable.Keys() {
		if name == "_" {
			continue
		}
		vs := result.GetValues(name)
		res := createNewRes(url, len(vs))
		res.ResourceType = "variable"
		res.ResourceName = name
		if msg, ok := result.AlertMsgTable[name]; ok {
			res.VerboseType = "alert"
			res.VerboseName = msg
			resources = append(resources, res)
		} else {
			res.VerboseType = "normal"
			normalRes = append(normalRes, res)
		}
	}
	resources = append(resources, normalRes...)

	// last add "_"
	{
		res := createNewRes(url, len(result.GetValues("_")))
		res.ResourceType = "variable"
		res.VerboseType = "unknown"
		res.ResourceName = "_"
		resources = append(resources, res)
	}
	return resources
}

func createNewRes(originParam *ypb.YakURL, size int, extra ...map[string]string) *ypb.YakURLResource {
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
		Path:              originParam.GetPass(),
		YakURLVerbose:     "",
		Url:               yakURL,
	}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			res.Extra = append(res.Extra, &ypb.KVPair{
				Key:   k,
				Value: v,
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
