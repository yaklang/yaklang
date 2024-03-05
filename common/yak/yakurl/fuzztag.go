package yakurl

import (
	"fmt"
	"github.com/yaklang/yaklang/common/fuzztagx/parser"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sort"
	"strings"
)

type fuzzTagDocAction struct{}

func parserTagMethodToExtra(t *parser.TagMethod) []*ypb.KVPair {
	return []*ypb.KVPair{
		{Key: "is_dyn", Value: fmt.Sprint(t.IsDyn)},
		{Key: "alias", Value: strings.Join(t.Alias, ",")},
		{Key: "description_ch", Value: t.Description},
	}
}

func (f fuzzTagDocAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}
	u.Schema = "fuzztag"
	tagName := strings.Trim(u.GetLocation(), "/")
	if tagName == "" {
		var rsc []*ypb.YakURLResource
		for name, tag := range mutate.GetExistedFuzzTagMap() {
			if utils.StringArrayContains(tag.Alias, name) {
				continue
			}
			newUrl, err := CreateUrlFromString("fuzztag://" + name)
			if err != nil {
				continue
			}
			rsc = append(rsc, &ypb.YakURLResource{
				ResourceType:      "fuzztag",
				VerboseType:       "FuzzTag",
				ResourceName:      name,
				VerboseName:       name,
				Path:              "/",
				Url:               newUrl,
				HaveChildrenNodes: len(tag.Alias) > 0,
				Extra:             parserTagMethodToExtra(tag),
			})
		}
		sort.SliceStable(rsc, func(i, j int) bool {
			return rsc[i].VerboseName < rsc[j].VerboseName
		})
		return &ypb.RequestYakURLResponse{
			Resources: rsc,
		}, nil
	} else {
		var alias []*ypb.YakURLResource
		m := mutate.GetExistedFuzzTagMap()
		tag, ok := m[tagName]
		if ok && len(tag.Alias) > 0 {
			for _, name := range tag.Alias {
				nu, err := CreateUrlFromString("fuzztag://" + name)
				if err != nil {
					continue
				}
				alias = append(alias, &ypb.YakURLResource{
					ResourceType: "fuzztag",
					VerboseType:  "FuzzTag",
					ResourceName: name,
					VerboseName:  name,
					Url:          nu,
					Path:         "/",
					Extra:        parserTagMethodToExtra(tag),
				})
			}
		}
		sort.SliceStable(alias, func(i, j int) bool {
			return alias[i].VerboseName < alias[j].VerboseName
		})
		return &ypb.RequestYakURLResponse{
			Resources: alias,
		}, nil
	}
}

func (f fuzzTagDocAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fuzzTagDocAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fuzzTagDocAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fuzzTagDocAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}

func (f fuzzTagDocAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	//TODO implement me
	return nil, utils.Error("not implemented")
}
