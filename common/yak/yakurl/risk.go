package yakurl

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
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
				path: "${program}/${path}/${function}"
			}
		Response:
			1. path : "${program}" :
				return {
					resource: []Resource{
						name: "${file}"
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
				}

				// SELECT program AS programName, COUNT(*) AS Count FROM db GROUP BY program;
			2. path: "${program}/${file}"
				return {
					resource: []Resource{
						name: "${function}"
						Extra: []{
							Key: "count"
							Value: "${risk_count}"
						}
						{
							Key: "filter"
							Value: "${risk_filter}"
						},
					}
				}
*/

type riskLevel string

const (
	rroot     riskLevel = "root"
	rprogram  riskLevel = "program"
	rsource   riskLevel = "source"
	rfunction riskLevel = "function"
)

func (t riskTreeAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	var res []*ypb.YakURLResource
	u := params.GetUrl()
	db := ssadb.GetDB()

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	rawpath := u.GetPath()
	programName, sourceUrl, funcName := t.splittingRowPath(rawpath)

	var rcs []*yakit.SsaRiskCount
	var err error

	lowerLevel := rroot
	search := ""
	if ret := query.Get("search"); ret != "" {
		search = strings.TrimPrefix(ret, "/")
		db = bizhelper.FuzzSearchWithStringArrayOrEx(db, []string{"code_source_url", "function_name"}, []string{ret, ret}, false)
		//db = bizhelper.FuzzQueryLike(db, "function_name", search)
	}

	if funcName != "" {
		rcs, err = yakit.GetSSARiskByFuncName(db, programName, sourceUrl, funcName)
		if err != nil {
			return nil, err
		}
		lowerLevel = rfunction
	} else if sourceUrl != "" {
		rcs, err = yakit.GetSSARiskBySourceUrl(db, programName, sourceUrl)
		if err != nil {
			return nil, err
		}
		lowerLevel = rfunction
	} else if programName != "" {
		rcs, err = yakit.GetSSARiskByProgram(db, programName)
		if err != nil {
			return nil, err
		}
		lowerLevel = rsource
	} else {
		rcs, err = yakit.GetSSARiskByRoot(db)
		if err != nil {
			return nil, err
		}
		lowerLevel = rprogram
	}

	for _, rc := range rcs {
		r, err := t.riskToResource(params.GetUrl(), lowerLevel, search, rc)
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

func (t riskTreeAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, nil
}

func (t riskTreeAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, nil
}

func (t riskTreeAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, nil
}

func (t riskTreeAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (t riskTreeAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	return nil, utils.Error("not implemented")
}

func (t *riskTreeAction) FormatPath(params *ypb.RequestYakURLParams) (string, string, string, error) {
	return "", "", "", nil
}

func (t *riskTreeAction) splittingRowPath(rawpath string) (string, string, string) {
	rawpath = strings.TrimPrefix(rawpath, "/")

	programName := ""
	sourceUrl := ""
	funcName := ""

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

	return programName, sourceUrl, funcName
}

func (t *riskTreeAction) riskToResource(originParam *ypb.YakURL, level riskLevel, search string, rc *yakit.SsaRiskCount) (*ypb.YakURLResource, error) {
	programName := rc.Prog
	sourceUrl := strings.TrimPrefix(rc.Source, "/")
	if index := strings.Index(sourceUrl, "/"); index != -1 {
		sourceUrl = sourceUrl[index:]
	}
	funcName := rc.Func
	count := rc.Count

	part := ""
	currentPath := ""
	filter := &ypb.SSARisksFilter{}
	switch level {
	case rprogram:
		part = programName
		currentPath = path.Join("/", programName)
		filter.ProgramName = []string{programName}
		if search != "" && strings.Index(sourceUrl, search) != -1 {
			filter.CodeSourceUrl = yakit.GetSSARiskByFuzzy(ssadb.GetDB(), programName, "", search, string(rsource))
		}
		if search != "" && strings.Index(funcName, search) != -1 {
			filter.FunctionName = yakit.GetSSARiskByFuzzy(ssadb.GetDB(), programName, "", search, string(rfunction))
		}
	case rsource:
		part = sourceUrl
		currentPath = path.Join("/", programName, sourceUrl)
		filter.ProgramName = []string{programName}
		filter.CodeSourceUrl = []string{path.Join("/", programName, sourceUrl)}
		if search != "" && strings.Index(funcName, search) != -1 {
			filter.FunctionName = yakit.GetSSARiskByFuzzy(ssadb.GetDB(), programName, path.Join("/", programName, sourceUrl), search, string(rfunction))
		}
	case rfunction:
		part = funcName
		currentPath = path.Join("/", programName, sourceUrl, funcName)
		filter.ProgramName = []string{programName}
		filter.CodeSourceUrl = []string{path.Join("/", programName, sourceUrl)}
		filter.FunctionName = []string{funcName}
	}

	filterdata, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}

	yakURL := &ypb.YakURL{
		Schema:   originParam.Schema,
		User:     originParam.GetUser(),
		Pass:     originParam.GetPass(),
		Location: originParam.GetLocation(),
		Path:     currentPath,
		Query:    originParam.GetQuery(),
	}

	extraData := []extra{
		{"count", count},
		{"filter", filterdata},
	}

	res := createNewRes(yakURL, 0, extraData)
	res.VerboseName = part
	res.ResourceName = part
	res.VerboseType = string(level)
	res.ResourceType = string(level)

	return res, nil
}
