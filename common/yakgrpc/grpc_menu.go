package yakgrpc

import (
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"strconv"
	"strings"
)

func (s *Server) AddToMenu(ctx context.Context, req *ypb.AddToMenuRequest) (*ypb.Empty, error) {
	r, err := yakit.GetYakScript(s.GetProfileDatabase(), req.GetYakScriptId())
	if err != nil {
		return nil, err
	}
	if r.ScriptName == "" {
		return nil, utils.Errorf("no script name...")
	}

	item := &yakit.MenuItem{
		Group:         req.Group,
		Verbose:       req.Verbose,
		YakScriptName: r.ScriptName,
		Mode:          req.Mode,
		MenuSort:      req.MenuSort,
		GroupSort:     req.GroupSort,
	}

	if req.Group == "" {
		req.Group = "UserDefined"
	}
	_ = yakit.CreateOrUpdateMenuItem(s.GetProfileDatabase(), item.CalcHash(), item)
	return &ypb.Empty{}, nil
}

func (s *Server) RemoveFromMenu(ctx context.Context, req *ypb.RemoveFromMenuRequest) (*ypb.Empty, error) {
	r, err := yakit.GetYakScript(s.GetProfileDatabase(), req.GetYakScriptId())
	if err != nil {
		return nil, err
	}
	if r.ScriptName == "" {
		return nil, utils.Errorf("no script name...")
	}

	err = yakit.DeleteMenuItem(s.GetProfileDatabase(), req.Group, r.ScriptName, req.Mode)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) YakScriptIsInMenu(ctx context.Context, req *ypb.YakScriptIsInMenuRequest) (*ypb.Empty, error) {
	r, err := yakit.GetYakScript(s.GetProfileDatabase(), req.GetYakScriptId())
	if err != nil {
		return nil, err
	}
	if r.ScriptName == "" {
		return nil, utils.Errorf("no script name...")
	}

	f, err := yakit.GetMenuItem(s.GetProfileDatabase(), req.GetGroup(), r.ScriptName)
	if err != nil {
		return nil, err
	}
	_ = f
	return &ypb.Empty{}, nil
}

func (s *Server) _yakitMenuItemToGRPCMenuItem(i *yakit.MenuItem) (*ypb.MenuItem, error) {
	var YakScriptId int64
	if i.YakScriptName != "" {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), i.YakScriptName)
		if err != nil {
			return nil, utils.Errorf("loading menuItem failed: %s", err)
		}
		YakScriptId = int64(script.ID)
	}

	rawJson, _ := strconv.Unquote(i.BatchPluginFilterJson)
	var query ypb.BatchExecutionPluginFilter
	_ = json.Unmarshal([]byte(rawJson), &query)

	item := &ypb.MenuItem{
		Group:       i.Group,
		Verbose:     i.Verbose,
		YakScriptId: YakScriptId,
		Query:       &query,
		MenuItemId:  uint64(i.ID),
	}
	if item.Verbose == "" {
		item.Verbose = i.YakScriptName
	}
	return item, nil
}

func (s *Server) GetAllMenuItem(ctx context.Context, req *ypb.Empty) (*ypb.MenuByGroup, error) {
	var groups = map[string]*ypb.MenuItemGroup{}
	for _, i := range yakit.GetAllMenuItem(s.GetProfileDatabase()) {
		group, ok := groups[i.Group]
		if !ok {
			groups[i.Group] = &ypb.MenuItemGroup{
				Group: i.Group,
			}
			group = groups[i.Group]
		}

		item, err := s._yakitMenuItemToGRPCMenuItem(i)
		if err != nil {
			log.Error(err)
			continue
		}
		group.Items = append(group.Items, item)
	}

	var groupItems []*ypb.MenuItemGroup
	for _, i := range groups {
		if i.Items == nil {
			continue
		}
		groupItems = append(groupItems, i)
	}
	return &ypb.MenuByGroup{Groups: groupItems}, nil
}

func (s *Server) QueryGroupsByYakScriptId(ctx context.Context, req *ypb.QueryGroupsByYakScriptIdRequest) (*ypb.GroupNames, error) {
	r, err := yakit.GetYakScript(s.GetProfileDatabase(), req.GetYakScriptId())
	if err != nil {
		return nil, err
	}
	if r.ScriptName == "" {
		return nil, utils.Errorf("no script name...")
	}

	var items []*yakit.MenuItem
	db := s.GetProfileDatabase().Where("yak_script_name = ?", r.ScriptName)
	if req.GetMode() != "" {
		db = db.Where("mode = ?", req.GetMode())
	}
	db = db.Find(&items)
	if db.Error != nil {
		return nil, err
	}

	var groups []string
	for _, i := range items {
		groups = append(groups, i.Group)
	}
	return &ypb.GroupNames{Groups: utils.RemoveRepeatedWithStringSlice(groups)}, nil
}

func (s *Server) GetMenuItemById(ctx context.Context, req *ypb.GetMenuItemByIdRequest) (*ypb.MenuItem, error) {
	menuItem, err := yakit.GetMenuItemById(s.GetProfileDatabase(), int64(req.GetID()))
	if err != nil {
		return nil, err
	}
	return s._yakitMenuItemToGRPCMenuItem(menuItem)
}

func (s *Server) DeleteAllMenuItem(ctx context.Context, req *ypb.Empty) (*ypb.Empty, error) {
	err := yakit.DeleteMenuItemAll(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) ExportMenuItem(ctx context.Context, req *ypb.Empty) (*ypb.ExportMenuItemResult, error) {
	items := yakit.GetAllMenuItem(s.GetProfileDatabase())
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	return &ypb.ExportMenuItemResult{RawJson: string(raw)}, nil
}

func (s *Server) ImportMenuItem(ctx context.Context, req *ypb.ImportMenuItemRequest) (*ypb.Empty, error) {
	if req.GetJsonFileName() != "" && req.GetRawJson() == "" {
		content, err := ioutil.ReadFile(req.GetJsonFileName())
		if err != nil {
			return nil, err
		}
		req.RawJson = string(content)
	}

	var items []*yakit.MenuItem
	err := json.Unmarshal([]byte(req.RawJson), &items)
	if err != nil {
		return nil, err
	}

	items = funk.Map(items, func(i *yakit.MenuItem) *yakit.MenuItem {
		i.ID = 0
		return i
	}).([]*yakit.MenuItem)
	for _, i := range items {
		i.Hash = i.CalcHash()
		yakit.CreateOrUpdateMenuItem(s.GetProfileDatabase(), i.CalcHash(), i)
	}
	return &ypb.Empty{}, nil
}

func (s *Server) QueryAllMenuItem(ctx context.Context, req *ypb.QueryAllMenuItemRequest) (*ypb.MenuByGroup, error) {
	var groups = map[string]*ypb.MenuItemGroup{}

	AllMenuItem := yakit.QueryAllMenuItemByWhere(s.GetProfileDatabase(), req)

	var groupItems []*ypb.MenuItemGroup
	for _, i := range AllMenuItem {
		groups[i.Group] = &ypb.MenuItemGroup{
			Group:    i.Group,
			MenuSort: i.MenuSort,
			Mode:     i.Mode,
		}
		if !IsContain(groupItems, groups[i.Group].Group) {
			groupItems = append(groupItems, groups[i.Group])
		}

	}
	for _, v := range groupItems {
		for _, i := range AllMenuItem {
			if i.Group == v.Group {
				item, err := s._queryAllYakitMenuItemToGRPCMenuItem(i)
				if err != nil {
					log.Error(err)
					continue
				}
				v.Items = append(v.Items, item)
			}
		}
	}
	return &ypb.MenuByGroup{Groups: groupItems}, nil
}

func IsContain(items []*ypb.MenuItemGroup, item string) bool {
	for _, eachItem := range items {
		if eachItem.Group == item {
			return true
		}
	}
	return false
}

func (s *Server) _queryAllYakitMenuItemToGRPCMenuItem(i *yakit.MenuItem) (*ypb.MenuItem, error) {
	var YakScriptId int64
	if i.YakScriptName != "" {
		script, err := yakit.GetYakScriptByName(s.GetProfileDatabase(), i.YakScriptName)
		if err == nil {
			YakScriptId = int64(script.ID)
		}
	}

	rawJson, _ := strconv.Unquote(i.BatchPluginFilterJson)
	var query ypb.BatchExecutionPluginFilter
	_ = json.Unmarshal([]byte(rawJson), &query)

	item := &ypb.MenuItem{
		Group:         i.Group,
		Verbose:       i.Verbose,
		YakScriptId:   YakScriptId,
		Query:         &query,
		MenuItemId:    uint64(i.ID),
		GroupSort:     i.GroupSort,
		YakScriptName: i.YakScriptName,
	}
	if item.Verbose == "" {
		item.Verbose = i.YakScriptName
	}
	return item, nil
}

func (s *Server) AddMenus(ctx context.Context, req *ypb.AddMenuRequest) (*ypb.Empty, error) {
	if req.Data == nil {
		return nil, utils.Errorf("no Data...")
	}
	var errVerbose []string
	for _, v := range req.Data {
		for _, k := range v.Items {
			r, err := yakit.GetYakScript(s.GetProfileDatabase(), k.YakScriptId)
			ScriptName := k.YakScriptName
			if r != nil {
				ScriptName = r.ScriptName
			}
			item := &yakit.MenuItem{
				Group:         v.Group,
				Verbose:       k.Verbose,
				YakScriptName: ScriptName,
				Mode:          v.Mode,
				MenuSort:      v.MenuSort,
				GroupSort:     k.GroupSort,
			}

			if v.Group == "" {
				v.Group = "UserDefined"
			}
			err = yakit.CreateOrUpdateMenuItem(s.GetProfileDatabase(), item.CalcHash(), item)
			if err != nil {
				errVerbose = append(errVerbose, k.Verbose)
			}
		}
	}
	if len(errVerbose) > 0 {
		return nil, utils.Errorf(strings.Join(errVerbose, ",") + "加载失败")
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteAllMenu(ctx context.Context, req *ypb.QueryAllMenuItemRequest) (*ypb.Empty, error) {
	db := s.GetProfileDatabase().Model(&yakit.MenuItem{})
	if req.GetVerbose() != "" {
		db = db.Where("verbose = ?", req.Verbose)
	} else {
		if req.GetMode() != "" {
			db = db.Where("true and mode = ? ", req.Mode)
		} else {
			db = db.Where("mode IS NULL OR mode = '' ")
		}
	}
	db = db.Unscoped().Delete(&yakit.MenuItem{})
	if db.Error != nil {
		return nil, db.Error
	}
	return &ypb.Empty{}, nil
}
