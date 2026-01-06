//go:build !yakit_exclude

package yakgrpc

import (
	"context"
	"fmt"
	"strings"

	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryYakScriptGroup(ctx context.Context, req *ypb.QueryYakScriptGroupRequest) (*ypb.QueryYakScriptGroupResponse, error) {
	var groupCount ypb.QueryYakScriptGroupResponse

	groups, _ := yakit.QueryGroupCount(s.GetProfileDatabase(), req.ExcludeType, req.IsMITMParamPlugins)
	filterGroup := filter.NewFilter()
	for _, group := range groups {
		if filterGroup.Exist(group.Value) {
			continue
		}
		if req.GetAll() {
			if req.GetIsPocBuiltIn() != group.IsPocBuiltIn {
				continue
			}
			if req.GetPageId() != group.TemporaryId {
				continue
			}
			groupCount.Group = append(groupCount.Group, &ypb.GroupCount{
				Value:        group.Value,
				Total:        int32(group.Count),
				Default:      false,
				TemporaryId:  group.TemporaryId,
				IsPocBuiltIn: group.IsPocBuiltIn,
			})
			filterGroup.Insert(group.Value)
		} else if !req.GetIsPocBuiltIn() && req.GetPageId() == "" && !req.GetAll() {
			if req.GetIsPocBuiltIn() != group.IsPocBuiltIn {
				continue
			}
			if req.GetPageId() != group.TemporaryId {
				continue
			}
			groupCount.Group = append(groupCount.Group, &ypb.GroupCount{
				Value:        group.Value,
				Total:        int32(group.Count),
				Default:      false,
				TemporaryId:  group.TemporaryId,
				IsPocBuiltIn: group.IsPocBuiltIn,
			})
			filterGroup.Insert(group.Value)
		} else {
			// 检查记录是否满足IsPocBuiltIn为true
			isPocBuiltInMatch := req.GetIsPocBuiltIn() && group.IsPocBuiltIn

			// 检查记录的TemporaryId是否与PageId相等
			pageIdMatch := req.GetPageId() != "" && group.TemporaryId == req.GetPageId()

			// 如果记录满足IsPocBuiltIn为true或PageId相等的任一条件
			if isPocBuiltInMatch || pageIdMatch {
				groupCount.Group = append(groupCount.Group, &ypb.GroupCount{
					Value:        group.Value,
					Total:        int32(group.Count),
					Default:      false,
					TemporaryId:  group.TemporaryId,
					IsPocBuiltIn: group.IsPocBuiltIn,
				})
				filterGroup.Insert(group.Value)
			}
		}
	}
	filterGroup.Close()
	return &groupCount, nil
}

func (s *Server) SaveYakScriptGroup(ctx context.Context, req *ypb.SaveYakScriptGroupRequest) (*ypb.Empty, error) {
	if req.SaveGroup == nil && req.RemoveGroup == nil {
		return nil, utils.Errorf("params is empty")
	}
	var errGroup []string
	db := s.GetProfileDatabase().Model(&schema.YakScript{})
	db = yakit.FilterYakScript(db, req.Filter)
	yakScripts := yakit.YieldYakScripts(db, context.Background())
	for yakScript := range yakScripts {
		if len(req.SaveGroup) > 0 {
			for _, group := range req.SaveGroup {
				saveData := &schema.PluginGroup{
					YakScriptName: yakScript.ScriptName,
					Group:         group,
					TemporaryId:   req.GetPageId(),
				}
				saveData.Hash = saveData.CalcHash()
				err := yakit.CreateOrUpdatePluginGroup(s.GetProfileDatabase(), saveData.Hash, saveData)
				if err != nil {
					errGroup = append(errGroup, fmt.Sprintf("%v", saveData))
					log.Errorf("[%v] Save YakScriptGroup [%v] err %s", yakScript.ScriptName, group, err.Error())
				}
			}
		}
		if len(req.RemoveGroup) > 0 {
			for _, group := range req.RemoveGroup {
				saveData := &schema.PluginGroup{
					YakScriptName: yakScript.ScriptName,
					Group:         group,
					TemporaryId:   req.GetPageId(),
				}
				saveData.Hash = saveData.CalcHash()
				err := yakit.DeletePluginGroupByHash(s.GetProfileDatabase(), saveData.Hash)
				if err != nil {
					errGroup = append(errGroup, fmt.Sprintf("%v", saveData))
					log.Errorf("[%v] Remove YakScriptGroup [%v] err %s", yakScript.ScriptName, group, err.Error())
				}
			}
		}
	}
	if len(errGroup) > 0 {
		return nil, utils.Error("设置分组失败")
	}
	return &ypb.Empty{}, nil
}

func (s *Server) RenameYakScriptGroup(ctx context.Context, req *ypb.RenameYakScriptGroupRequest) (*ypb.Empty, error) {
	if req.NewGroup == "" || req.Group == "" {
		return nil, utils.Errorf("params is empty")
	}
	rets, err := yakit.GetPluginByGroup(s.GetProfileDatabase(), req.Group)
	if err != nil {
		return nil, utils.Errorf("yakScript is empty")
	}
	utils.GormTransaction(s.GetProfileDatabase(), func(tx *gorm.DB) error {
		for _, ret := range rets {
			saveData := &schema.PluginGroup{
				YakScriptName: ret.YakScriptName,
				Group:         req.NewGroup,
			}
			saveData.Hash = saveData.CalcHash()
			err = yakit.CreateOrUpdatePluginGroup(tx, saveData.Hash, saveData)
			if err != nil {
				return utils.Errorf("rename YakScriptGroup err %s", err.Error())
			}
			err = yakit.DeletePluginGroupByHash(tx, ret.Hash)
			if err != nil {
				return utils.Errorf("rename YakScriptGroup err %s", err.Error())
			}
		}
		return nil
	})

	return &ypb.Empty{}, nil
}

func (s *Server) DeleteYakScriptGroup(ctx context.Context, req *ypb.DeleteYakScriptGroupRequest) (*ypb.Empty, error) {
	if req.GetGroup() == "" {
		return nil, utils.Errorf("params is empty")
	}
	groups := strings.Split(req.GetGroup(), ",")
	for _, group := range groups {
		err := yakit.DeletePluginGroup(s.GetProfileDatabase(), group)
		if err != nil {
			return nil, err
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) GetYakScriptGroup(ctx context.Context, req *ypb.QueryYakScriptRequest) (*ypb.GetYakScriptGroupResponse, error) {
	var data ypb.GetYakScriptGroupResponse
	allGroup, _ := yakit.QueryGroupCount(s.GetProfileDatabase(), []string{}, req.IsMITMParamPlugins)

	db := s.GetProfileDatabase().Model(&schema.YakScript{})
	db = yakit.FilterYakScript(db, req)
	yakScripts := yakit.YieldYakScripts(db, context.Background())

	var yakScriptName []string
	for yakScript := range yakScripts {
		yakScriptName = append(yakScriptName, yakScript.ScriptName)
	}
	for k, v := range funk.ChunkStrings(yakScriptName, 100) {
		setGroup, err := yakit.GetGroup(s.GetProfileDatabase(), v)
		if err != nil {
			log.Errorf("getGroup error: %v", err)
		}
		var setGroups []string
		for _, group := range setGroup {
			setGroups = append(setGroups, group.Group)
		}
		if k == 0 {
			data.SetGroup = setGroups
		} else {
			data.SetGroup = funk.IntersectString(data.SetGroup, setGroups)
		}
	}

	filterGroup := filter.NewFilter()
	for _, v := range allGroup {
		if filterGroup.Exist(v.Value) {
			continue
		}
		found := false
		for _, setGroup := range data.SetGroup {
			if v.Value == setGroup {
				found = true
				continue
			}
		}
		if v.IsPocBuiltIn {
			continue
		}
		if len(v.TemporaryId) > 0 {
			continue
		}
		if !found {
			data.AllGroup = append(data.AllGroup, v.Value)
		}
		filterGroup.Insert(v.Value)
	}

	return &data, nil
}

func (s *Server) ResetYakScriptGroup(ctx context.Context, req *ypb.ResetYakScriptGroupRequest) (*ypb.Empty, error) {
	// 置空本地组
	err := yakit.DeletePluginGroup(s.GetProfileDatabase(), "")
	if err != nil {
		return nil, utils.Errorf("rename Group err %s", err.Error())
	}

	YakPlugin := yakit.YieldYakScripts(s.GetProfileDatabase(), context.Background())
	var scriptNames []string
	for v := range YakPlugin {
		scriptNames = append(scriptNames, v.ScriptName)
	}
	if len(scriptNames) > 0 {
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
		plugins := client.DownloadOnlinePluginByPluginName(ctx, req.Token, scriptNames)
		for pluginIns := range plugins.Chan {
			err := client.Save(s.GetProfileDatabase(), pluginIns.Plugin)
			if err != nil {
				log.Errorf("save err failed: %s", err)
			}

		}
		return &ypb.Empty{}, nil
	}
	return &ypb.Empty{}, nil
}

func (s *Server) SetGroup(ctx context.Context, req *ypb.SetGroupRequest) (*ypb.Empty, error) {
	if req.GroupName == "" {
		return nil, utils.Errorf("params is empty")
	}
	saveData := &schema.PluginGroup{
		Group: req.GetGroupName(),
	}
	saveData.Hash = saveData.CalcHash()
	err := yakit.CreateOrUpdatePluginGroup(s.GetProfileDatabase(), saveData.Hash, saveData)
	if err != nil {
		return nil, utils.Errorf("Save YakScriptGroup [%v] err %s", req.GroupName, err.Error())
	}

	return &ypb.Empty{}, nil
}
