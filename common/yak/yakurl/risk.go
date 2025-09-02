package yakurl

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
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

				"program": "${program}" // if set this, path will ignore program
				"rule": "${rule}" 		// if set this, path will ignore rule

				// for filter
				"result_id": "${result_id}"
				"task_id": "${task_id}"
			}
		}

		type="risk" "" (default):
			path:="${program?}/${source}/${function}"
		type="file"
			path:="${program?}/${source}/${risk}"
		type="rule" :
			path:="${program?}/${rule?}/${source}/${risk}"

		source must contain "."

	Response:
		// get program, level=program
		// get rule, level=rule
		// get file, level=source
		// get function, level=function
		// get risk, level=risk

		resource: []Resource{
			VerboseName:  ${name}
			VerboseType:  ${level} // leaf or branch

			// frontend use this to require backend
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
					// ResourceType == risk
					{
						Key: "id"
					}
					{
						Key: "hash"
					}
					{
						Key: "code_range"
					}
					{
						Key: "severity"
					}
			}
		}
*/

type SSARiskResponseLevel string

const (
	SSARiskLevelProgram  SSARiskResponseLevel = "program"
	SSARiskLevelRule     SSARiskResponseLevel = "rule"
	SSARiskLevelSource   SSARiskResponseLevel = "source"
	SSARiskLevelFunction SSARiskResponseLevel = "function"
	SSARiskLevelRisk     SSARiskResponseLevel = "risk"
)

type SSARiskResponseType string

const (
	SSARiskTypeRisk SSARiskResponseType = "risk" // /${program}/${source}/${function}  			default
	SSARiskTypeFile SSARiskResponseType = "file" // /${program}/${source}/${function}/${risk}
	SSARiskTypeRule SSARiskResponseType = "rule" // /${program}/${rule}/${source}/${risk}
)

func (t riskTreeAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	var res []*ypb.YakURLResource

	filter, err := GetSSARiskCountFilter(params.GetUrl())
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
			log.Warnf("cover ssa-risk-info to resource: %v", err)
			continue
			// return nil, err
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

type SSARiskCountFilter struct {
	Level    SSARiskResponseLevel
	LeafNode bool
	Filter   *ypb.SSARisksFilter
}

func GetSSARiskCountFilter(u *ypb.YakURL) (*SSARiskCountFilter, error) {
	ret := &SSARiskCountFilter{}

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	riskType := SSARiskTypeRisk
	switch query.Get("type") {
	case "rule":
		riskType = SSARiskTypeRule
	case "file":
		riskType = SSARiskTypeFile
	default:
		riskType = SSARiskTypeRisk
	}

	var programName, rule, sourceUrl, funcName string

	if prog := query.Get("program"); prog != "" {
		programName = prog
	}
	if r := query.Get("rule"); r != "" {
		rule = r
	}

	restPath := ""
	if programName == "" {
		rawpath := strings.TrimPrefix(u.GetPath(), "/")
		// if no set program-name, parse from path
		if firstIndex := strings.Index(rawpath, "/"); firstIndex != -1 {
			programName = rawpath[:firstIndex]
			restPath = rawpath[firstIndex:]
		} else {
			programName = rawpath
		}
	} else {
		// if set program-name in param, ignore program in path
		restPath = u.GetPath()
	}

	switch riskType {
	case SSARiskTypeRisk:
		// 	restPath:="/${source}/${function}"
		if dotIndex := strings.LastIndex(restPath, "."); dotIndex != -1 {
			if lastIndex := strings.LastIndex(restPath, "/"); dotIndex < lastIndex {
				// like : "rule_name.rule_suffix/function_name"
				funcName = restPath[lastIndex+1:]
				sourceUrl = restPath[:lastIndex]
			} else {
				sourceUrl = restPath
			}
		} else {
			log.Warnf("path source not contain `.`, request path: [%s] param: [%v]", u.Path, u)
		}
	case SSARiskTypeFile:
		// 	restPath:="/${source}/"
		if dotIndex := strings.LastIndex(restPath, "."); dotIndex != -1 {
			sourceUrl = restPath
		}
	case SSARiskTypeRule:
		if rule == "" {
			// 	restPath:="/${rule}/${source}"
			restPath = strings.TrimPrefix(restPath, "/")
			if lastIndex := strings.Index(restPath, "/"); lastIndex != -1 {
				sourceUrl = restPath[lastIndex:]
				rule = restPath[:lastIndex]
			} else {
				rule = restPath
			}
		} else {
			// restPath = "/${source}"
			sourceUrl = restPath
		}
	}

	var opts []yakit.SSARiskFilterOption
	switch {
	case programName == "":
		ret.Level = SSARiskLevelProgram // not found program name, return all program
		opts = append(opts, yakit.WithSSARiskFilterRuleName(rule))

	// risk type=risk  /${program}/${source}/${function}
	case riskType == SSARiskTypeRisk && sourceUrl == "":
		ret.Level = SSARiskLevelSource // not found source url, return all source in program
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))
	case riskType == SSARiskTypeRisk && funcName == "":
		ret.Level = SSARiskLevelFunction // not found function name, return all function in source & program
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))
		opts = append(opts, yakit.WithSSARiskFilterSourceUrl(sourceUrl))

	// rule type=file /${program}/${source}/${risk}
	case riskType == SSARiskTypeFile && sourceUrl == "":
		ret.Level = SSARiskLevelSource // not found source url, return all source in program
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))

	// rule type=rule
	case riskType == SSARiskTypeRule && rule == "":
		ret.Level = SSARiskLevelRule
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))
	case riskType == SSARiskTypeRule && sourceUrl == "":
		ret.Level = SSARiskLevelSource
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))
		opts = append(opts, yakit.WithSSARiskFilterRuleName(rule))

	// get risk
	default:
		opts = append(opts, yakit.WithSSARiskFilterProgramName(programName))
		opts = append(opts, yakit.WithSSARiskFilterRuleName(rule))
		opts = append(opts, yakit.WithSSARiskFilterSourceUrl(sourceUrl))
		opts = append(opts, yakit.WithSSARiskFilterFunction(funcName))
		ret.Level = SSARiskLevelRisk // return all risk in {program, source, function}
	}

	if search := query.Get("search"); search != "" {
		opts = append(opts, yakit.WithSSARiskFilterSearch(search))
	}
	if taskID := query.Get("task_id"); taskID != "" {
		if compare := query.Get("compare"); compare != "" {
			opts = append(opts, yakit.WithSSARiskFilterCompare(taskID, compare))
		}
		if incremental := query.Get("incremental"); incremental == "true" {
			opts = append(opts, yakit.WithSSARiskFilterIncremental(taskID))
		}
		opts = append(opts, yakit.WithSSARiskFilterTaskID(taskID))
	}
	if resultID := query.Get("result_id"); resultID != "" {
		opts = append(opts, yakit.WithSSARiskResultID(uint64(utils.ParseStringToInts(resultID)[0])))
	}

	switch riskType {
	case SSARiskTypeRisk: // end in function
		ret.LeafNode = (ret.Level == SSARiskLevelFunction || ret.Level == SSARiskLevelRisk)
	case SSARiskTypeFile, SSARiskTypeRule: // end in risk
		ret.LeafNode = (ret.Level == SSARiskLevelRisk)
	}

	ret.Filter = yakit.NewSSARiskFilter(opts...)
	log.Debugf("path [%s] param [%v] filter [%v]", u.Path, query, ret)
	return ret, nil
}

type SSARiskCountInfo struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`

	Title     string `json:"title"`
	Id        int64  `json:"id"`
	Hash      string `json:"hash"`
	CodeRange string `json:"code_range"`
	Severity  string `json:"severity"`

	ResultId string `json:"result_id"`
	Variable string `json:"variable"`
	Index    int64  `json:"index"`
}

func GetSSARiskCountInfo(filter *SSARiskCountFilter) ([]*SSARiskCountInfo, error) {
	db := ssadb.GetDB()
	db = db.Model(&schema.SSARisk{})
	// db = db.Debug()

	db = yakit.FilterSSARisk(db, filter.Filter)

	switch filter.Level {
	case SSARiskLevelProgram:
		db = db.Select("program_name as name, COUNT(*) as count").Group("program_name")
	case SSARiskLevelRule:
		db = db.Select("from_rule as name, COUNT(*) as count").Group("from_rule")
	case SSARiskLevelSource:
		db = db.Select("code_source_url as name, COUNT(*) as count").Group("code_source_url")
	case SSARiskLevelFunction:
		db = db.Select("function_name as name, COUNT(*) as count").Group("function_name")
	case SSARiskLevelRisk:
		db = db.Select(`title_verbose as name, 1 as count, title , id , hash , code_range, severity, result_id, variable, "index"`)
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

	var filter ypb.SSARisksFilter = *countFilter.Filter // copy assign

	switch countFilter.Level {
	case SSARiskLevelProgram:
		filter.ProgramName = append(filter.ProgramName, rc.Name)
	case SSARiskLevelRule:
		filter.FromRule = append(filter.FromRule, rc.Name)
	case SSARiskLevelSource:
		filter.CodeSourceUrl = append(filter.CodeSourceUrl, rc.Name)
	case SSARiskLevelFunction:
		filter.FunctionName = append(filter.FunctionName, rc.Name)
	case SSARiskLevelRisk:
		filter.Hash = append(filter.Hash, rc.Hash)
		filter.ID = append(filter.ID, rc.Id)
		if rc.Name == "" {
			rc.Name = rc.Title
		}
	}

	if rc.Name == "" {
		return nil, utils.Error("name is empty")
	}

	filterData, err := json.Marshal(&filter)
	if err != nil {
		return nil, err
	}

	extraData := make([]extra, 0)
	if countFilter.Level == SSARiskLevelRisk {
		extraData = append(extraData,
			extra{"id", rc.Id},
			extra{"hash", rc.Hash},
			extra{"code_range", rc.CodeRange},
			extra{"severity", rc.Severity},
			extra{"result_id", rc.ResultId},
			extra{"variable", rc.Variable},
			extra{"index", rc.Index},
		)
	} else {
		extraData = append(extraData,
			extra{"count", rc.Count},
			extra{"filter", filterData},
		)
	}

	res := createNewRes(originParam, 0, extraData)

	if countFilter.Level == SSARiskLevelRisk {
		res.Path = path.Join(originParam.Path, rc.Hash)
	} else {
		res.Path = path.Join(originParam.Path, rc.Name)
	}
	res.ResourceName = rc.Name
	res.ResourceType = string(countFilter.Level)
	res.HaveChildrenNodes = !countFilter.LeafNode

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
