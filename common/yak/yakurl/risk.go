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
	db := ssadb.GetDB()

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	rawpath := u.GetPath()
	programName, sourceUrl, funcName := t.splittingRowPath(rawpath)

	var rcs []*yakit.SsaRiskCount
	var err error

	lowerLevel := ""
	if ret := query.Get("search"); ret != "" {
		var rcfs []*yakit.SsaRiskFullCount
		rcfs, err = yakit.GetSSARiskByFuzzy(db, ret)
		if err != nil {
			return nil, err
		}

		for _, rcf := range rcfs {
			if rcf.Funcdata != "" {
				lowerLevel = "function"
			} else if rcf.Pathdata != "" {
				lowerLevel = "source"
			} else if rcf.Progdata != "" {
				lowerLevel = "program"
			} else {
				continue
			}

			rawpath := path.Join(rcf.Pathdata, rcf.Funcdata)
			if rawpath == "" || rawpath == "/" {
				continue
			}
			r, err := t.riskToResource(params.GetUrl(), rawpath, "", lowerLevel, rcf.Count)
			if err != nil {
				return nil, err
			}
			res = append(res, r)
		}

		return &ypb.RequestYakURLResponse{
			Page:      1,
			PageSize:  1000,
			Total:     int64(len(res)),
			Resources: res,
		}, nil
	}

	if funcName != "" {
		rcs, err = yakit.GetSSARiskByFuncName(db, programName, sourceUrl, funcName)
		if err != nil {
			return nil, err
		}
		lowerLevel = "function"
	} else if sourceUrl != "" {
		rcs, err = yakit.GetSSARiskBySourceUrl(db, programName, sourceUrl)
		if err != nil {
			return nil, err
		}
		lowerLevel = "function"
	} else if programName != "" {
		rcs, err = yakit.GetSSARiskByProgram(db, programName)
		if err != nil {
			return nil, err
		}
		lowerLevel = "source"
	} else {
		rcs, err = yakit.GetSSARiskByRoot(db)
		if err != nil {
			return nil, err
		}
		lowerLevel = "program"
	}

	for _, rc := range rcs {
		r, err := t.riskToResource(params.GetUrl(), rawpath, rc.Data, lowerLevel, rc.Count)
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

func (t *riskTreeAction) riskToResource(originParam *ypb.YakURL, currentPath, data, level string, count int64) (*ypb.YakURLResource, error) {
	rawpath := path.Join("/", currentPath, data)
	programName, sourceUrl, funcName := t.splittingRowPath(rawpath)
	part := ""

	filter := &ypb.SSARisksFilter{}
	if programName != "" {
		filter.ProgramName = []string{programName}
		part = programName
	}
	if sourceUrl != "" {
		filter.CodeSourceUrl = []string{path.Join("/", programName, sourceUrl)}
		part = sourceUrl
	}
	if funcName != "" {
		filter.FunctionName = []string{funcName}
		part = funcName
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
		Path:     rawpath,
		Query:    originParam.GetQuery(),
	}

	extraData := []extra{
		{"count", count},
		{"filter", filterdata},
	}

	res := createNewRes(yakURL, 0, extraData)
	res.VerboseName = part
	res.ResourceName = part
	res.VerboseType = level
	res.ResourceType = level

	return res, nil
}
