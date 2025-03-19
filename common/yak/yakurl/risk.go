package yakurl

import (
	"encoding/json"
	"net/url"
	"path"
	"strings"

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

func (t riskTreeAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	var res []*ypb.YakURLResource
	u := params.GetUrl()

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	rawpath := path.Join(u.GetLocation(), u.GetPath())
	programName, sourceUrl, funcName := t.splittingRowPath(rawpath)

	db := ssadb.GetDB()
	isend := false
	var rcs []*yakit.SsaRiskCount
	var err error

	if funcName != "" {
		rcs, err = yakit.GetSSARiskByFuncName(db, programName, sourceUrl, funcName)
		isend = true
		if err != nil {
			return nil, err
		}
	} else if sourceUrl != "" {
		rcs, err = yakit.GetSSARiskBySourceUrl(db, programName, sourceUrl)
		if err != nil {
			return nil, err
		}
	} else if programName != "" {
		rcs, err = yakit.GetSSARiskByProgram(db, programName)
		if err != nil {
			return nil, err
		}
	} else {
		rcs, err = yakit.GetSSARiskByRoot(db)
		if err != nil {
			return nil, err
		}
	}

	for _, rc := range rcs {
		r, err := t.riskToResource(params.GetUrl(), rc.Data, rc.Count, isend)
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
	if len(rawpath) > 0 && rawpath[0] == '/' {
		rawpath = rawpath[1:]
	}

	programName := ""
	sourceUrl := ""
	funcName := ""

	if firstIndex := strings.Index(rawpath, "/"); firstIndex != -1 {
		programName = rawpath[:firstIndex]
		sourceUrl = "/" + rawpath
	} else {
		programName = rawpath
	}

	dotIndex := strings.LastIndex(sourceUrl, ".")
	if lastIndex := strings.LastIndex(sourceUrl, "/"); dotIndex != -1 && lastIndex > dotIndex {
		funcName = sourceUrl[lastIndex+1:]
		sourceUrl = sourceUrl[:lastIndex]
	}

	return programName, sourceUrl, funcName
}

func (t *riskTreeAction) riskToResource(originParam *ypb.YakURL, currentPath string, count int64, isend bool) (*ypb.YakURLResource, error) {
	rawpath := currentPath
	programName, sourceUrl, funcName := t.splittingRowPath(rawpath)

	filter := &ypb.SSARisksFilter{}
	if funcName != "" {
		filter.FunctionName = []string{funcName}
	}
	if sourceUrl != "" {
		filter.CodeSourceUrl = []string{sourceUrl}
	}
	if programName != "" {
		filter.ProgramName = []string{programName}
	}

	filterdata, err := json.Marshal(filter)
	if err != nil {
		return nil, err
	}

	if isend { // 最后一层展开时不需要返回路径
		currentPath = ""
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

	return res, nil
}
