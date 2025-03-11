package yakurl

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type riskTreeAction struct {
	register map[string]int
}

func (t riskTreeAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	var res []*ypb.YakURLResource
	var tmpMap = make(map[string]struct{})
	u := params.GetUrl()

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	// _, risks, _ := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{}, nil)
	path := u.GetPath()
	programName := u.GetLocation()
	funcName := ""
	// funcName := query.Get("function_name")

	if strings.Contains(path, ".go/") {
		lastIndex := strings.LastIndex(path, "/")
		if lastIndex == -1 {

		} else {
			funcName = path[lastIndex+1:]
			path = path[:lastIndex]
		}
	}

	risks, err := yakit.GetSSARisk(ssadb.GetDB(), programName, path, funcName)
	if err != nil {
		return nil, err
	}

	for _, r := range risks {
		if r.CodeSourceUrl == "" {
			continue
		}
		path := fmt.Sprintf("%s/%s", r.CodeSourceUrl, r.FunctionName)
		if _, ok := tmpMap[path]; ok {
			continue
		}
		tmpMap[path] = struct{}{}

		count, err := yakit.GetCount(ssadb.GetDB(), r.CodeSourceUrl, r.FunctionName)
		if err != nil {
			return nil, err
		}

		res = append(res, t.riskToResource(params.GetUrl(), path, count))
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

func (t *riskTreeAction) riskToResource(originParam *ypb.YakURL, currentPath string, count int) *ypb.YakURLResource {
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
	}
	res := createNewRes(yakURL, 0, extraData)

	return res
}
