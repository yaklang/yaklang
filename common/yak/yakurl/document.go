package yakurl

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
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
	yakDocumentExisted         = omap.NewOrderedMap(make(map[string]*yakdoc.ScriptLib))
)

func (d *documentAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}

	if yakDocumentExisted.Len() <= 0 {
		yakDocumentExistedCoolDown.Do(func() {
			result := doc.DefaultDocumentHelper
			for _, i := range result.Libs {
				yakDocumentExisted.Set(i.Name, i)
			}
		})
	}

	docMap := yakDocumentExisted
	libName := strings.TrimSpace(u.GetLocation())
	if libName == "" {
		var rsc []*ypb.YakURLResource
		docMap.ForEach(func(i string, v *yakdoc.ScriptLib) bool {
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
		result, ok := docMap.Get(libName)
		if !ok {
			return nil, utils.Errorf("lib[%v] is not existed", libName)
		}
		return getResponseFromPath(result, u)
	}
	return &ypb.RequestYakURLResponse{}, nil
}

func getResponseFromPath(document *yakdoc.ScriptLib, u *ypb.YakURL) (*ypb.RequestYakURLResponse, error) {
	ret := strings.ToLower(strings.Trim(u.GetPath(), "/"))
	var rsc []*ypb.YakURLResource
	for _, i := range document.Functions {
		name := fmt.Sprintf("%v.%v", i.LibName, i.MethodName)
		if ret != "" && !utils.IContains(i.MethodName, ret) {
			continue
		}
		rsc = append(rsc, &ypb.YakURLResource{
			ResourceType: "function",
			VerboseType:  "Function",
			ResourceName: name,
			VerboseName:  name,
			Path:         "/" + name,
			Url:          &ypb.YakURL{Schema: "yakdocument", Location: document.Name, Path: name},
			Extra: []*ypb.KVPair{
				{Key: "Content", Value: i.String()},
			},
		})
	}
	for _, i := range document.Instances {
		name := fmt.Sprintf("%v.%v", i.LibName, i.InstanceName)
		if ret != "" && !utils.IContains(i.InstanceName, ret) {
			continue
		}

		rsc = append(rsc, &ypb.YakURLResource{
			ResourceType: "variable",
			VerboseType:  "variable",
			ResourceName: name,
			VerboseName:  "func " + name,
			Path:         "/" + name,
			Url:          &ypb.YakURL{Schema: "yakdocument", Location: document.Name, Path: name},
		})
	}
	sort.SliceStable(rsc, func(i, j int) bool {
		return rsc[i].VerboseName < rsc[j].VerboseName
	})
	return &ypb.RequestYakURLResponse{Resources: rsc}, nil
}
