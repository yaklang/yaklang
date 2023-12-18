package yakurl

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type websiteFromHttpFlow struct {
	yakit.HTTPFlow
}

func parseQueryToRequest(db *gorm.DB, query string) *gorm.DB {
	var req ypb.QueryHTTPFlowRequest

	err := json.Unmarshal([]byte(query), &req)
	if err != nil {
		return db
	}
	return yakit.BuildHTTPFlowQuery(db, &req)
}

func (f *websiteFromHttpFlow) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	websiteRoot := u.GetLocation()

	db := consts.GetGormProjectDatabase()
	db = db.Model(&yakit.HTTPFlow{})

	if ret := query.Get("params"); ret != "" {
		db = parseQueryToRequest(db, ret)
	}

	if ret := query.Get("schema"); ret != "" {
		db = yakit.FilterHTTPFlowBySchema(db, ret)
		websiteRoot = ret + "://" + websiteRoot
	}
	if ret := query.Get("runtime_id"); ret != "" {
		db = yakit.FilterHTTPFlowByRuntimeID(db, ret)
	}
	isSearch := false
	if ret := query.Get("search"); ret != "" {
		isSearch = true
		if u.GetLocation() != "" {
			db = yakit.FilterHTTPFlowByDomain(db, u.GetLocation())
		}
	}

	if u.GetLocation() == "" || isSearch {
		var res []*ypb.YakURLResource
		for _, result := range yakit.GetHTTPFlowDomainsByDomainSuffix(db, u.GetLocation()) {

			var urlstr, location string
			if strings.HasPrefix(result.NextPart, result.Schema) {
				urlstr = result.NextPart
				location = strings.TrimPrefix(result.NextPart, result.Schema+"://")
			} else {
				urlstr = fmt.Sprintf("%v://%v", result.Schema, result.NextPart)
				location = result.NextPart
			}
			newParam := &ypb.YakURL{
				Schema:   u.GetSchema(),
				Location: location,
				Path:     "/",
				Query:    []*ypb.KVPair{{Key: "schema", Value: result.Schema}},
			}

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
				p = u.GetPath() + "?" + result.RawQueryKey
			} else {
				if len(result.RawNextPart) > 0 {
					p = strings.TrimSuffix(u.GetPath(), "/") + result.RawNextPart
				} else {
					p = path.Join(u.GetPath(), result.NextPart)
				}
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
			srcItem.Extra = append(srcItem.Extra, &ypb.KVPair{Key: "url", Value: websiteRoot + srcItem.Url.GetPath()})

			if result.Count > 1 {
				srcItem.VerboseName = fmt.Sprintf("%v/ [%v]", result.NextPart, srcItem.SizeVerbose)
			} else {
				srcItem.VerboseName = result.NextPart
			}
			if result.HaveChildren {
				srcItem.ResourceType = "path"
				srcItem.HaveChildrenNodes = true
				srcItem.VerboseType = "website-path"
			}
			if result.IsQuery {
				srcItem.ResourceType = "query"
				srcItem.VerboseType = "website-file-with-query"
				srcItem.VerboseName = fmt.Sprintf("GET 参数: %v", result.NextPart)
			}
			if result.IsFile {
				srcItem.ResourceType = "file"
				srcItem.VerboseType = "website-file"
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
	// TODO implement me
	panic("implement me")
}

func (f *websiteFromHttpFlow) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	panic("implement me")
}

func (f *websiteFromHttpFlow) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	panic("implement me")
}

func (f *websiteFromHttpFlow) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	panic("implement me")
}

func (f *websiteFromHttpFlow) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	panic("implement me")
}

var defaultWebsiteFromHttpFlow = &websiteFromHttpFlow{}

func GetWebsiteViewerAction() Action {
	return defaultWebsiteFromHttpFlow
}
