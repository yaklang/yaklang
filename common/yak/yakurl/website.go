package yakurl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"path"
	"strings"
)

type websiteFromHttpFlow struct {
}

func (f *websiteFromHttpFlow) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()

	db := consts.GetGormProjectDatabase()
	db = db.Model(&yakit.HTTPFlow{})
	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	var websiteRoot = u.GetLocation()
	if ret := query.Get("schema"); ret != "" {
		db = yakit.FilterHTTPFlowBySchema(db, ret)
		websiteRoot = ret + "://" + websiteRoot
	}
	// 临时过滤一下 404、502
	db = bizhelper.ExactQueryExcludeStringArrayOr(db, "status_code", utils.PrettifyListFromStringSplited("404,502", ","))

	if filterType := query.Get("filter"); filterType != "" {
		db = bizhelper.ExactQueryStringArrayOr(db, "source_type", utils.PrettifyListFromStringSplited(filterType, ","))
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
				ResourceName:      "/",
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

	db = yakit.FilterHTTPFlowByDomain(db, u.GetLocation()) //.Debug()

	var res []*ypb.YakURLResource
	switch query.Get("op") {
	case "list":
		fallthrough
	default:
		for _, result := range yakit.GetHTTPFlowNextPartPathByPathPrefix(db, u.GetPath()) {

			var p string
			if result.IsQuery {
				p = u.GetPath()
			} else {
				p = path.Join(u.GetPath(), result.NextPart)
			}
			if !strings.HasPrefix(p, "/") {
				p = "/" + p
			}
			newParam := &ypb.YakURL{
				Schema:   u.GetSchema(),
				User:     u.GetUser(),
				Pass:     u.GetPass(),
				Location: u.GetLocation(),
				Path:     p,
				Query:    []*ypb.KVPair{{Key: "schema", Value: result.Schema}},
			}
			verboseName := result.NextPart
			if result.HaveChildren || result.Count > 1 {
				verboseName += "/"
			}
			srcItem := &ypb.YakURLResource{
				ResourceName: result.NextPart,
				VerboseName:  verboseName,
				Size:         int64(result.Count),
				SizeVerbose:  fmt.Sprint(result.Count),
				Path:         p,
				Url:          newParam,
			}
			if !strings.Contains(websiteRoot, "://") {
				websiteRoot = result.Schema + "://" + websiteRoot
			}
			suff := strings.TrimPrefix(p, "/")
			if ret, err := utils.UrlJoin(websiteRoot, suff); err == nil {
				if !strings.Contains(ret, "://") {
					srcItem.Extra = append(srcItem.Extra, &ypb.KVPair{Key: "url", Value: result.Schema + "://" + ret})
				} else {
					srcItem.Extra = append(srcItem.Extra, &ypb.KVPair{Key: "url", Value: ret})
				}
			}
			if result.HaveChildren {
				srcItem.ResourceType = "path"
				srcItem.HaveChildrenNodes = true
				srcItem.VerboseType = "website-path"
			}
			if result.IsQuery {
				srcItem.ResourceType = "query"
				srcItem.VerboseType = "website-file-with-query"
			}
			if result.IsFile {
				srcItem.ResourceType = "file"
				srcItem.VerboseType = "website-file"
			}
			if result.Count > 1 {
				srcItem.VerboseName = fmt.Sprintf("%v/ [%v]", result.NextPart, srcItem.SizeVerbose)
			} else {
				srcItem.VerboseName = result.NextPart
			}

			srcItem.YakURLVerbose = srcItem.Url.GetSchema() + "://" + srcItem.Url.GetLocation() + srcItem.Url.GetPath()
			res = append(res, srcItem)
		}
		return &ypb.RequestYakURLResponse{
			Page:      1,
			PageSize:  1000,
			Resources: res,
			Total:     int64(len(res)),
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
