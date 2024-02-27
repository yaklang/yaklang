package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryYakScriptGroup(ctx context.Context, req *ypb.QueryYakScriptGroupRequest) (*ypb.QueryYakScriptGroupResponse, error) {
	var groupCount ypb.QueryYakScriptGroupResponse
	if req.All {
		countAll, err := yakit.CountYakScriptByWhere(s.GetProfileDatabase(), false)
		if err == nil {
			groupCount.Group = append(groupCount.Group, &ypb.GroupCount{
				Value:   "全部",
				Total:   int32(countAll),
				Default: true,
			})
		}
		countGroup, err := yakit.CountYakScriptByWhere(s.GetProfileDatabase(), true)
		if err == nil {
			groupCount.Group = append(groupCount.Group, &ypb.GroupCount{
				Value:   "未分组",
				Total:   int32(countGroup),
				Default: true,
			})
		}
	}
	group, _ := yakit.GroupCount(s.GetProfileDatabase())
	for _, v := range group {
		groupCount.Group = append(groupCount.Group, &ypb.GroupCount{
			Value:   v.Value,
			Total:   int32(v.Count),
			Default: false,
		})
	}
	return &groupCount, nil
}

func (s *Server) SaveYakScriptGroup(ctx context.Context, req *ypb.SaveYakScriptGroupRequest) (*ypb.Empty, error) {
	if req.SaveGroup == nil && req.RemoveGroup == nil {
		return nil, utils.Errorf("params is empty")
	}
	var errGroup []string
	db := s.GetProfileDatabase().Model(&yakit.YakScript{})
	db = yakit.FilterYakScript(db, req.Filter)
	yakScripts := yakit.YieldYakScripts(db, context.Background())
	for yakScript := range yakScripts {
		res, _ := yakit.GetYakScriptByName(s.GetProfileDatabase(), yakScript.ScriptName)
		if res == nil || res.Type == "yak" || res.Type == "codec" {
			continue
		}
		if len(req.SaveGroup) > 0 {
			for _, group := range req.SaveGroup {
				saveData := &yakit.PluginGroup{
					YakScriptName: yakScript.ScriptName,
					Group:         group,
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
				saveData := &yakit.PluginGroup{
					YakScriptName: yakScript.ScriptName,
					Group:         group,
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
	sw := s.GetProfileDatabase().Begin()
	for _, ret := range rets {
		saveData := &yakit.PluginGroup{
			YakScriptName: ret.YakScriptName,
			Group:         req.NewGroup,
		}
		saveData.Hash = saveData.CalcHash()
		err = yakit.CreateOrUpdatePluginGroup(sw, saveData.Hash, saveData)
		if err != nil {
			sw.Rollback()
			return nil, utils.Errorf("rename YakScriptGroup err %s", err.Error())
		}
		err = yakit.DeletePluginGroupByHash(sw, ret.Hash)
		if err != nil {
			sw.Rollback()
			return nil, utils.Errorf("rename YakScriptGroup err %s", err.Error())
		}
	}
	sw.Commit()
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteYakScriptGroup(ctx context.Context, req *ypb.DeleteYakScriptGroupRequest) (*ypb.Empty, error) {
	err := yakit.DeletePluginGroup(s.GetProfileDatabase(), req.Group)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetYakScriptGroup(ctx context.Context, req *ypb.QueryYakScriptRequest) (*ypb.GetYakScriptGroupResponse, error) {
	var data ypb.GetYakScriptGroupResponse
	allGroup, _ := yakit.GetGroup(s.GetProfileDatabase(), nil)

	db := s.GetProfileDatabase().Model(&yakit.YakScript{})
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

	for _, group := range allGroup {
		found := false
		for _, setGroup := range data.SetGroup {
			if group.Group == setGroup {
				found = true
				continue
			}
		}
		if !found {
			data.AllGroup = append(data.AllGroup, group.Group)
		}
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
