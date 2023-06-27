package yakgrpc

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"strings"
)

func (s *Server) AddToNavigation (ctx context.Context, req *ypb.AddToNavigationRequest)  (*ypb.Empty, error) {
	if req.Data != nil {
		var errVerbose []string
		for _, v := range req.Data {
			for _, k := range v.Items {
				item := &yakit.NavigationBar{
					Group:         v.Group,
					YakScriptName: k.YakScriptName,
					Mode:          v.Mode,
					VerboseSort:   k.VerboseSort,
					GroupSort:     v.GroupSort,
					Route:         k.Route,
					Verbose:       k.Verbose,
					VerboseLabel:  k.VerboseLabel,
					GroupLabel:    v.GroupLabel,
				}
				if v.Group == "" {
					v.Group = "UserDefined"
				}
				item.Hash = item.CalcHash()
				err := yakit.CreateOrUpdateNavigation(s.GetProfileDatabase(), item.CalcHash(), item)
				if err != nil {
					errVerbose = append(errVerbose, k.Verbose)
				}
			}
		}
		if len(errVerbose) > 0 {
			return nil, utils.Errorf(strings.Join(errVerbose, ",") + "加载失败")
		}
	}
	return &ypb.Empty{}, nil

}

func IsContainNavigation(items []*ypb.NavigationList, item string) bool {
	for _, eachItem := range items {
		if eachItem.Group == item {
			return true
		}
	}
	return false
}


func (s *Server) GetAllNavigationItem (ctx context.Context, req *ypb.GetAllNavigationRequest) (*ypb.GetAllNavigationItemResponse, error) {
	var groups = map[string]*ypb.NavigationList{}
	allNavigationItem := yakit.GetAllNavigation(s.GetProfileDatabase(), req)
	var groupItems []*ypb.NavigationList
	for _, i := range allNavigationItem {
		groups[i.Group] = &ypb.NavigationList{
			Group: i.Group,
			GroupLabel: i.GroupLabel,
			GroupSort: i.GroupSort,
			Mode:      i.Mode,
		}
		if !IsContainNavigation(groupItems, groups[i.Group].Group) {
			groupItems = append(groupItems, groups[i.Group])
		}
	}

	for _, v := range groupItems {
		for _, i := range allNavigationItem {
			if i.Group == v.Group {
				item, err := s.ToGRPCNavigation(i)
				if err != nil {
					log.Error(err)
					continue
				}
				v.Items = append(v.Items, item)
			}
		}
	}
	return &ypb.GetAllNavigationItemResponse{Data: groupItems}, nil
}

func (s *Server) ToGRPCNavigation(i *yakit.NavigationBar) (*ypb.NavigationItem, error) {
	var (
		yakScriptId int64
		headImg string
	)
	if i.YakScriptName != "" {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), i.YakScriptName)
		if err != nil {
			return nil, utils.Errorf("loading NavigationBar failed: %s", err)
		}
		headImg = script.HeadImg
		yakScriptId = int64(script.ID)
	}
	item := &ypb.NavigationItem{
		Group:       	i.Group,
		YakScriptId: 	yakScriptId,
		//MenuItemId:  uint64(i.ID),
		Mode:          	i.Mode,
		VerboseSort:      i.VerboseSort,
		GroupSort:     	i.GroupSort,
		Route:         	i.Route,
		YakScriptName: 	i.YakScriptName,
		Verbose: 		i.Verbose,
		GroupLabel: 	i.GroupLabel,
		VerboseLabel: 	i.VerboseLabel,
		HeadImg:        headImg,
	}
	if item.Verbose == "" {
		item.Verbose = i.YakScriptName
	}
	return item, nil
}

func (s *Server) DeleteAllNavigation (ctx context.Context, req *ypb.GetAllNavigationRequest) (*ypb.Empty, error) {
	err := yakit.DeleteNavigationByWhere(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) AddOneNavigation (ctx context.Context, req *ypb.AddOneNavigationRequest) (*ypb.Empty, error)  {
	if req.YakScriptName == "" {
		return nil, utils.Errorf("no script name...")
	}

	item := &yakit.NavigationBar{
		Group:         req.Group,
		Verbose:       req.Verbose,
		YakScriptName: req.YakScriptName,
		Mode:          req.Mode,
		GroupSort:     req.GroupSort,
		VerboseSort:   req.VerboseSort,
		Route:         req.Route,
		GroupLabel:    req.GroupLabel,
		VerboseLabel:  req.VerboseLabel,
	}
	item.Hash = item.CalcHash()
	if req.Group == "" {
		req.Group = "UserDefined"
	}
	_ = yakit.CreateOrUpdateNavigation(s.GetProfileDatabase(), item.CalcHash(), item)
	return &ypb.Empty{}, nil
}

func (s *Server) QueryNavigationGroups (ctx context.Context, req *ypb.QueryNavigationGroupsRequest) (*ypb.GroupNames, error)  {
	var items []*yakit.NavigationBar
	db := s.GetProfileDatabase().Where("yak_script_name = ?", req.YakScriptName)
	if req.GetMode() != "" {
		db = db.Where("mode = ?", req.GetMode())
	}
	db = db.Find(&items)
	if db.Error != nil {
		return nil, db.Error
	}
	var groups []string
	for _, i := range items {
		groups = append(groups, i.Group)
	}
	return &ypb.GroupNames{Groups: utils.RemoveRepeatedWithStringSlice(groups)}, nil
}