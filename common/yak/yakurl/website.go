package yakurl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"path"
)

type websiteFromHttpFlow struct {
}

func (f *websiteFromHttpFlow) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()

	db := consts.GetGormProjectDatabase()
	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	var websiteRoot string = u.GetLocation()
	if ret := query.Get("schema"); ret != "" {
		db = yakit.FilterHTTPFlowBySchema(db, ret)
		websiteRoot = ret + "://" + websiteRoot
	}

	if u.GetLocation() == "" {
		var res []*ypb.YakURLResource
		for _, result := range yakit.GetHTTPFlowDomainsByDomainSuffix(db, u.GetLocation()) {
			newParam := &ypb.YakURL{
				Schema:   u.GetSchema(),
				Location: result.NextPart,
				Path:     "/",
				Query:    []*ypb.KVPair{{Key: "schema", Value: result.Schema}},
			}
			urlstr := fmt.Sprintf("%v://%v", result.Schema, result.NextPart)
			res = append(res, &ypb.YakURLResource{
				ResourceType:      "dir",
				VerboseType:       "filesystem-directory",
				ResourceName:      "website",
				VerboseName:       urlstr,
				Path:              "/",
				YakURLVerbose:     "website://" + newParam.GetLocation() + "/",
				Url:               newParam,
				HaveChildrenNodes: true,
				Extra: []*ypb.KVPair{
					{Key: "url", Value: urlstr},
				},
			})
		}
		return &ypb.RequestYakURLResponse{
			Page: 1, PageSize: 1000,
			Resources: res,
		}, nil
	}

	db = yakit.FilterHTTPFlowByDomain(db, u.GetLocation()).Debug()

	var res []*ypb.YakURLResource
	switch query.Get("op") {
	case "list":
		fallthrough
	default:
		for _, result := range yakit.GetHTTPFlowNextPartPathByPathPrefix(db, u.GetPath()) {
			newParam := &ypb.YakURL{
				Schema:   u.GetSchema(),
				User:     u.GetUser(),
				Pass:     u.GetPass(),
				Location: u.GetLocation(),
				Path:     path.Join(u.GetPath(), result.NextPart),
				Query:    u.GetQuery(),
			}
			verboseName := result.NextPart
			if result.HaveChildren || result.Count > 1 {
				verboseName += "/"
			}
			srcItem := &ypb.YakURLResource{
				VerboseName: verboseName,
				Size:        int64(result.Count),
				SizeVerbose: fmt.Sprint(result.Count),
				Path:        path.Join(u.GetPath(), result.NextPart),
				Url:         newParam,
			}
			if ret, err := utils.UrlJoin(websiteRoot, result.NextPart); err == nil {
				srcItem.Extra = append(srcItem.Extra, &ypb.KVPair{Key: "url", Value: ret})
			}
			if result.HaveChildren {
				srcItem.ResourceType = "dir"
				srcItem.HaveChildrenNodes = true
				srcItem.VerboseType = "filesystem-directory"
			}
			if result.Count > 1 {
				srcItem.VerboseName = fmt.Sprintf("%v/ [%v]", result.NextPart, srcItem.SizeVerbose)
			} else {
				srcItem.VerboseName = result.NextPart
			}
			res = append(res, srcItem)
		}
		return &ypb.RequestYakURLResponse{
			Page:      1,
			PageSize:  1000,
			Resources: res,
		}, nil
	}
}

func (f *websiteFromHttpFlow) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (f *websiteFromHttpFlow) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (f *websiteFromHttpFlow) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (f *websiteFromHttpFlow) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (f *websiteFromHttpFlow) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	panic("implement me")
}

var defaultWebsiteFromHttpFlow = &websiteFromHttpFlow{}

func GetWebsiteViewerAction() Action {
	return defaultWebsiteFromHttpFlow
}
