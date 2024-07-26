package yakurl

import (
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/fuzzy"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
	"github.com/yaklang/yaklang/common/yak/yakdoc/doc"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

type documentAction struct{}

func (d *documentAction) Post(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

func (d *documentAction) Put(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

func (d *documentAction) Delete(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

func (d *documentAction) Head(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

func (d *documentAction) Do(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// TODO implement me
	return nil, utils.Error("not implemented")
}

var (
	docMapOnce      = utils.NewCoolDown(10 * time.Second)
	docMap          = make(map[string][]*ypb.YakURLResource)
	fuzzySearchMap  = make(map[string]*ypb.YakURLResource)
	fuzzySearchKeys = make([]string, 0)
	maxFuzzyLength  = 64
)

func (d *documentAction) Get(params *ypb.RequestYakURLParams) (*ypb.RequestYakURLResponse, error) {
	// available query:
	// u.Path=xxx # search keyword
	u := params.GetUrl()
	if u == nil {
		return nil, utils.Error("url is nil")
	}

	query := make(url.Values)
	for _, v := range u.GetQuery() {
		query.Add(v.GetKey(), v.GetValue())
	}

	if len(docMap) <= 0 {
		docMapOnce.Do(func() {
			// all lib
			resources := make([]*ypb.YakURLResource, 0, len(doc.GetDefaultDocumentHelper().Libs))
			keys := lo.Keys(doc.GetDefaultDocumentHelper().Libs)
			sort.Strings(keys)
			for _, key := range keys {
				lib := doc.GetDefaultDocumentHelper().Libs[key]
				resources = append(resources, &ypb.YakURLResource{
					ResourceType:      "document",
					VerboseType:       "Document",
					ResourceName:      lib.Name,
					VerboseName:       lib.Name,
					Path:              "/",
					Url:               &ypb.YakURL{Schema: "yakdocument", Location: lib.Name},
					HaveChildrenNodes: true,
				})
			}
			docMap[""] = resources

			// each lib
			for _, lib := range doc.GetDefaultDocumentHelper().Libs {
				resources = generateResourceFromLib(lib)
				sort.SliceStable(resources, func(i, j int) bool {
					if resources[i].ResourceType != resources[j].ResourceType {
						return resources[i].ResourceType < resources[j].ResourceType
					}
					return resources[i].ResourceName < resources[j].ResourceName
				})

				for _, rsc := range resources {
					extrasStr := strings.Join(lo.Map(rsc.Extra, func(v *ypb.KVPair, i int) string {
						return v.Value
					}), "\n")
					totalName := fmt.Sprintf("%s.%s", lib.Name, rsc.ResourceName)
					docMap[totalName] = []*ypb.YakURLResource{rsc}

					fuzzyKey := strings.ToLower(fmt.Sprintf("%s.%s|%s", lib.Name, rsc.ResourceName, extrasStr))
					cloned := cloneResource(rsc)
					cloned.ResourceName = totalName
					fuzzySearchMap[fuzzyKey] = cloned
					fuzzySearchKeys = append(fuzzySearchKeys, fuzzyKey)
				}
				docMap[lib.Name] = resources
			}
		})
	}

	// exact word
	word := u.GetLocation()
	exactWord := strings.TrimSpace(word)
	resources, ok := docMap[exactWord]
	if !ok {
		// fuzzy search
		fuzzyResults := fuzzy.RankFindEx(
			strings.ToLower(word),
			fuzzySearchKeys,
			func(s1, s2 string) float64 {
				var i, counter float64
				splited := strings.Split(s1, " ")
				for _, word := range splited {
					if strings.Contains(s2, word) {
						counter++
						i += fuzzy.LevenshteinDistance(word, s2)
					}
				}
				if i > 0 {
					return i / counter
				}
				return math.MaxFloat64
			})
		sort.Sort(fuzzyResults)
		maxLen := len(fuzzyResults)
		if maxLen > maxFuzzyLength {
			maxLen = maxFuzzyLength
		}
		for i := 0; i < maxLen; i++ {
			if fuzzyResults[i].Distance == math.MaxFloat64 {
				continue
			}
			resources = append(resources, fuzzySearchMap[fuzzyResults[i].Target])
		}
	}
	return &ypb.RequestYakURLResponse{Resources: resources}, nil
}

func cloneResource(rsc *ypb.YakURLResource) *ypb.YakURLResource {
	if rsc == nil {
		return nil
	}
	// copy extras
	extras := lo.Map(rsc.Extra, func(v *ypb.KVPair, i int) *ypb.KVPair {
		return &ypb.KVPair{Key: v.Key, Value: v.Value}
	})

	res := &ypb.YakURLResource{
		ResourceType:      rsc.ResourceType,
		VerboseType:       rsc.VerboseType,
		ResourceName:      rsc.ResourceName,
		VerboseName:       rsc.VerboseName,
		Path:              rsc.Path,
		Url:               rsc.Url,
		HaveChildrenNodes: rsc.HaveChildrenNodes,
		Extra:             extras,
	}
	return res
}

func generateResourceFromLib(lib *yakdoc.ScriptLib) []*ypb.YakURLResource {
	resources := make([]*ypb.YakURLResource, 0, len(lib.Functions)+len(lib.Instances))
	for _, i := range lib.Functions {
		name := i.MethodName
		decl, doc := i.Decl, i.Document
		if doc != "" {
			doc = "\n\n" + strings.Replace(doc, "```\n", "```yak\n", 1)
		}
		if decl != "" {
			decl = "func " + decl
		}
		desc := fmt.Sprintf("```go\n%s\n```%s", decl, doc)
		rsc := &ypb.YakURLResource{
			ResourceType: "function",
			VerboseType:  "Function",
			ResourceName: name,
			VerboseName:  fmt.Sprintf("%s.%s", lib.Name, name),
			Path:         "/" + name,
			Url:          &ypb.YakURL{Schema: "yakdocument", Location: lib.Name, Path: name},
			Extra: []*ypb.KVPair{
				{Key: "Content", Value: desc},
			},
		}
		resources = append(resources, rsc)
	}

	for _, i := range lib.Instances {
		name := i.InstanceName
		resources = append(resources, &ypb.YakURLResource{
			ResourceType: "variable",
			VerboseType:  "variable",
			ResourceName: name,
			VerboseName:  fmt.Sprintf("variable %s.%s", lib.Name, name),
			Path:         "/" + name,
			Url:          &ypb.YakURL{Schema: "yakdocument", Location: lib.Name, Path: name},
		})
	}
	sort.SliceStable(resources, func(i, j int) bool {
		return resources[i].VerboseName < resources[j].VerboseName
	})
	return resources
}
