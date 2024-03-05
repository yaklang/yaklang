package yakurl

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/yaklang"
	"github.com/yaklang/yaklang/common/yakdocument"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sort"
	"strings"
	"time"
)

type documentAction struct{}

func (d *documentAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (d *documentAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (d *documentAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (d *documentAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (d *documentAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

var (
	yakDocumentExistedCoolDown = utils.NewCoolDown(10 * time.Second)
	yakDocumentExisted         = omap.NewOrderedMap(make(map[string]yakdocument.LibDoc))
)

func (d *documentAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}

	if yakDocumentExisted.Len() <= 0 {
		yakDocumentExistedCoolDown.Do(func() {
			results := yak.EngineToLibDocuments(yaklang.New())
			sort.SliceStable(results, func(i, j int) bool {
				return results[i].Name < results[j].Name
			})
			for _, i := range results {
				yakDocumentExisted.Set(i.Name, i)
			}
		})
	}

	doc := yakDocumentExisted
	libName := strings.TrimSpace(u.GetLocation())
	if libName == "" {
		var rsc []*ypb.YakURLResource
		doc.ForEach(func(i string, v yakdocument.LibDoc) bool {
			rsc = append(rsc, &ypb.YakURLResource{
				ResourceType:      "document",
				VerboseType:       "Document",
				ResourceName:      v.Name,
				VerboseName:       v.Name,
				Path:              "/",
				Url:               &ypb.YakURL{Schema: "yakdocument", Location: v.Name},
				HaveChildrenNodes: true,
			})
			return true
		})
		sort.SliceStable(rsc, func(i, j int) bool {
			return rsc[i].VerboseName < rsc[j].VerboseName
		})
		return &ypb.RequestYakURLResponse{Resources: rsc}, nil
	} else {
		result, ok := doc.Get(libName)
		if !ok {
			return nil, utils.Errorf("lib[%v] is not existed", libName)
		}
		return getResponseFromPath(result, u)
	}
	return &ypb.RequestYakURLResponse{}, nil
}

func getResponseFromPath(document yakdocument.LibDoc, u *ypb.YakURL) (*ypb.RequestYakURLResponse, error) {
	ret := strings.ToLower(strings.Trim(u.GetPath(), "/"))
	var rsc []*ypb.YakURLResource
	for _, i := range document.Functions {
		searchName, _ := strings.CutPrefix(i.Name, i.LibName+".")
		if ret != "" && !utils.IContains(searchName, ret) {
			continue
		}
		rsc = append(rsc, &ypb.YakURLResource{
			ResourceType: "function",
			VerboseType:  "Function",
			ResourceName: i.Name,
			VerboseName:  i.Name,
			Path:         "/" + i.Name,
			Url:          &ypb.YakURL{Schema: "yakdocument", Location: document.Name, Path: i.Name},
		})
	}
	for _, i := range document.Variables {
		searchName, _ := strings.CutPrefix(i.Name, document.Name+".")
		if ret != "" && !utils.IContains(searchName, ret) {
			continue
		}

		rsc = append(rsc, &ypb.YakURLResource{
			ResourceType: "variable",
			VerboseType:  "variable",
			ResourceName: i.Name,
			VerboseName:  "func " + i.Name,
			Path:         "/" + i.Name,
			Url:          &ypb.YakURL{Schema: "yakdocument", Location: document.Name, Path: i.Name},
		})
	}
	sort.SliceStable(rsc, func(i, j int) bool {
		return rsc[i].VerboseName < rsc[j].VerboseName
	})
	return &ypb.RequestYakURLResponse{Resources: rsc}, nil
}
