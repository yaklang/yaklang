package yakurl

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type riskTreeAction struct {
	register map[string]int
}

/*
	Get SSA Risk
	Request :
		url : {
			schema: "ssarisk"
			path:  path
			query: {
				search: "${search}"
				type: "${type}"
			}
		}

		type="file" (default):
			path:="${program}/${source}/${function}/${risk}"
		type="rule" :
			path:="${program}/${rule}/${source}/${risk}"

	Response:
		// get program, level=program
		// get rule, level=rule
		// get file, level=source
		// get function, level=function
		// get risk, level=risk

		resource: []Resource{
			// VerboseName:  ${name}
			// VerboseType:  ${level}

			// frontend use this to render
			ResourceName: ${name}
			ResourceType: ${level}

			Extra: []{
					{
						Key: "count"
						Value: "${risk_count}"
					},
					{
						Key: "filter"
						Value: "${risk_filter}"
					},
			}
		}
*/

type SSARiskResponseLevel string

const (
	SSARiskLevelProgram  SSARiskResponseLevel = "program"
	SSARiskLevelSource   SSARiskResponseLevel = "source"
	SSARiskLevelFunction SSARiskResponseLevel = "function"
	SSARiskLevelRisk     SSARiskResponseLevel = "risk"
	SSARiskLevelRules    SSARiskResponseLevel = "rule"
)

type SSARiskCountFilter struct {
	// filter with level
	Level SSARiskResponseLevel
	// ProgramName  string
	// SourceUrl    string
	// FunctionName string
	// RuleName     string
	filter *ypb.SSARisksFilter
}

type SSARiskCountInfo struct {
	Name  string
	Count int64
}

func (t riskTreeAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	var res []*ypb.YakURLResource

	filter, err := GetSSARiskCountFilter(params)
	if err != nil {
		return nil, err
	}

	rcs, err := GetSSARiskCountInfo(filter)
	if err != nil {
		return nil, err
	}

	for _, rc := range rcs {
		r, err := ConvertSSARiskCountInfoToResource(params.GetUrl(), filter, rc)
		if err != nil {
			return nil, err
		}
		res = append(res, r)
	}

	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  100,
		Total:     int64(len(res)),
		Resources: res,
	}, nil
}

func GetSSARiskCountFilter(params *ypb.RequestYakURLParams) (*SSARiskCountFilter, error) {
	ret := &SSARiskCountFilter{}

	u := params.GetUrl()

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	// type="file" (default):
	// 	path:="${program}/${path}/${function}/${risk}"
	// type="rule" :
	// 	path:="${program}/${rule}/${path}/${risk}"

	rawpath := strings.TrimPrefix(u.GetPath(), "/")

	var programName, sourceUrl, funcName string

	if firstIndex := strings.Index(rawpath, "/"); firstIndex != -1 {
		programName = rawpath[:firstIndex]
		sourceUrl = rawpath[firstIndex:]
	} else {
		programName = rawpath
	}
	if dotIndex := strings.LastIndex(sourceUrl, "."); dotIndex != -1 {
		sourceUrl = strings.TrimPrefix(sourceUrl, "/")
		if lastIndex := strings.LastIndex(sourceUrl, "/"); lastIndex > dotIndex {
			funcName = sourceUrl[lastIndex+1:]
			sourceUrl = sourceUrl[:lastIndex]
		}
		sourceUrl = "/" + sourceUrl
	}

	var opts []yakit.SSARiskFilterOption
	switch {
	case programName == "":
		ret.Level = SSARiskLevelProgram // not found program name, return all program
	case programName != "" && sourceUrl == "":
		// ret.filter.ProgramName = append(ret.filter.ProgramName, programName)
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))
		ret.Level = SSARiskLevelSource // not found source url, return all source in program
	case programName != "" && sourceUrl != "":
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))
		opts = append(opts, yakit.WithSSARiskFilterSourceUrl(sourceUrl))
		ret.Level = SSARiskLevelFunction // not found function name, return all function in source & program
	default:
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))
		opts = append(opts, yakit.WithSSARiskFilterSourceUrl(sourceUrl))
		opts = append(opts, yakit.WithSSARiskFilterFunction(funcName))
		ret.Level = SSARiskLevelRisk // return all risk in {program, source, function}
	}

	if search := query.Get("search"); search != "" {
		opts = append(opts, yakit.WithSSARiskFilterSearch(search))
	}

	ret.filter = yakit.NewSSARiskFilter(opts...)
	return ret, nil
}

func GetSSARiskCountInfo(filter *SSARiskCountFilter) ([]*SSARiskCountInfo, error) {
	db := ssadb.GetDB()
	db = db.Model(&schema.SSARisk{})
	// db = db.Debug()

	db = yakit.FilterSSARisk(db, filter.filter)

	switch filter.Level {
	case SSARiskLevelProgram:
		db = db.Select("program_name as name, COUNT(*) as count").Group("program_name")
	case SSARiskLevelSource:
		db = db.Select("code_source_url as name, COUNT(*) as count").Group("code_source_url")
	case SSARiskLevelFunction:
		db = db.Select("function_name as name, COUNT(*) as count").Group("function_name")
	case SSARiskLevelRisk:
		db = db.Select("hash as name, COUNT(*) as count").Group("hash")
	case SSARiskLevelRules:
		db = db.Select("from_rule as name, COUNT(*) as count").Group("from_rule")
	default:
		return nil, utils.Errorf("unknown level: %s", filter.Level)
	}

	var v []*SSARiskCountInfo
	if err := db.Scan(&v).Error; err != nil {
		return nil, utils.Errorf("scan failed: %v", err)
	}
	return v, nil
}

func ConvertSSARiskCountInfoToResource(originParam *ypb.YakURL, countFilter *SSARiskCountFilter, rc *SSARiskCountInfo) (*ypb.YakURLResource, error) {
	var filter ypb.SSARisksFilter = *countFilter.filter // copy assign
	switch countFilter.Level {
	case SSARiskLevelProgram:
		filter.ProgramName = []string{rc.Name}
	case SSARiskLevelSource:
		filter.CodeSourceUrl = []string{rc.Name}
	case SSARiskLevelFunction:
		filter.FunctionName = []string{rc.Name}
	case SSARiskLevelRisk:
		filter.Hash = []string{rc.Name}
	case SSARiskLevelRules:
		filter.FromRule = []string{rc.Name}
	}

	filterData, err := json.Marshal(&filter)
	if err != nil {
		return nil, err
	}

	extraData := []extra{
		{"count", rc.Count},
		{"filter", filterData},
	}
	res := createNewRes(originParam, 0, extraData)

	res.Path = path.Join(originParam.Path, rc.Name)
	res.ResourceName = rc.Name
	res.ResourceType = string(countFilter.Level)

	return res, nil
}

func (t riskTreeAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (t riskTreeAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (t riskTreeAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (t riskTreeAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (t riskTreeAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (t *riskTreeAction) FormatPath(params *ypb.RequestYakURLParams) (string, string, string, error) {
	return "", "", "", utils.Error("not implemented")
}
