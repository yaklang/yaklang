package yakurl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"net/url"
	"path"
)

type websiteFromHttpFlow struct {
}

func (f *websiteFromHttpFlow) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()

	var query = make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}
	var res []*ypb.YakURLResource
	switch query.Get("op") {
	case "list":
		for _, result := range yakit.GetHTTPFlowNextPartPathByPathPrefix(consts.GetGormProjectDatabase(), u.GetPath()) {
			newParam := &ypb.YakURL{
				Schema:   u.GetSchema(),
				User:     u.GetUser(),
				Pass:     u.GetPass(),
				Location: u.GetLocation(),
				Path:     path.Join(u.GetPath(), result.NextPart),
			}
			srcItem := &ypb.YakURLResource{
				VerboseName: result.NextPart,
				Size:        int64(result.Count),
				SizeVerbose: fmt.Sprint(result.Count),
				Path:        path.Join(u.GetPath(), result.NextPart),
				Url:         newParam,
			}
			if result.HaveChildren {
				srcItem.ResourceType = "dir"
				srcItem.HaveChildrenNodes = true
				srcItem.VerboseType = "filesystem-directory"
			}
			if result.Count > 0 {
				srcItem.VerboseName = fmt.Sprintf("%v [%v]", result.NextPart, srcItem.SizeVerbose)
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
	return &ypb.RequestYakURLResponse{
		Page:      1,
		PageSize:  1000,
		Resources: []*ypb.YakURLResource{},
	}, nil
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
