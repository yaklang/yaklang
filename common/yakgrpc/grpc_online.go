package yakgrpc

import (
	"context"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"time"
)

func (s *Server) GetOnlineProfile(ctx context.Context, req *ypb.Empty) (*ypb.OnlineProfile, error) {
	return &ypb.OnlineProfile{BaseUrl: consts.GetOnlineBaseUrl(), Password: ""}, nil
}

func (s *Server) SetOnlineProfile(ctx context.Context, req *ypb.OnlineProfile) (*ypb.Empty, error) {
	proxy := req.GetProxy()
	host, port, err := utils.ParseStringToHostPort(req.GetBaseUrl())
	if err != nil {
		return nil, utils.Errorf("parse url[%s] failed: %s", req.GetBaseUrl(), err)
	}

	if proxy != "" {
		conn, err := netx.DialTCPTimeoutForceProxy(10*time.Second, utils.HostPort(host, port), proxy)
		if err != nil {
			if req.IsCompany {
				return &ypb.Empty{}, nil
			}
			return nil, utils.Errorf("connect to [%s] via proxy[%v] failed: %s", utils.HostPort(host, port), proxy, err)
		}
		conn.Close()
	}
	consts.SetOnlineBaseUrlProxy(req.GetProxy())
	consts.SetOnlineBaseUrl(req.GetBaseUrl())
	return &ypb.Empty{}, nil
}

func (s *Server) DownloadOnlinePluginById(ctx context.Context, req *ypb.DownloadOnlinePluginByIdRequest) (*ypb.Empty, error) {
	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	plugin, err := client.DownloadYakitPluginById(req.GetUUID(), req.Token)
	if err != nil {
		return nil, utils.Errorf("download from [%v] failed: %s", req.GetOnlineID(), err)
	}
	err = client.Save(s.GetProfileDatabase(), plugin)
	if err != nil {
		return nil, utils.Errorf("save plugin[%s] to database failed: %v", plugin.ScriptName, err)
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DownloadOnlinePluginByIds(ctx context.Context, req *ypb.DownloadOnlinePluginByIdsRequest) (*ypb.Empty, error) {
	err := yaklib.DownloadOnlineAuthProxy(consts.GetOnlineBaseUrl())
	if err != nil {
		return nil, utils.Errorf("download failed: %s", err.Error())
	}
	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	plugins := client.DownloadYakitPluginIDWithToken(ctx, req.Token, req.GetUUID()...)
	for pluginIns := range plugins.Chan {
		err := client.Save(s.GetProfileDatabase(), pluginIns.Plugin)
		if err != nil {
			log.Errorf("save err failed: %s", err)
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) DownloadOnlinePluginAll(req *ypb.DownloadOnlinePluginByTokenRequest, stream ypb.Yak_DownloadOnlinePluginAllServer) error {
	err := yaklib.DownloadOnlineAuthProxy(consts.GetOnlineBaseUrl())
	if err != nil {
		return utils.Errorf("download failed: %s", err.Error())
	}
	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	var ch *yaklib.OnlineDownloadStream
	if req.GetBindMe() && req.GetToken() != "" {
		ch = client.DownloadYakitPluginWithTokenBindMe(stream.Context(), req.GetToken(), req.GetKeywords(), req.GetPluginType(), req.GetStatus(), req.GetIsPrivate(), req.GetTags(), req.GetUserId(), req.GetUserName(), req.GetTimeSearch(), req.GetGroup())
	} else {
		ch = client.DownloadYakitPluginAllWithToken(stream.Context(), req.GetToken(), req.GetKeywords(), req.GetPluginType(), req.GetStatus(), req.GetIsPrivate(), req.GetTags(), req.GetUserId(), req.GetUserName(), req.GetTimeSearch(), req.GetGroup())
	}
	if ch == nil {
		return utils.Error("BUG: download stream error: empty")
	}

	var progress float64
	var count float64 = 0
	stream.Send(&ypb.DownloadOnlinePluginProgress{
		Progress: 0,
		Log:      "initializing",
	})
	defer func() {
		stream.Send(&ypb.DownloadOnlinePluginProgress{
			Progress: 1,
			Log:      "finished",
		})
	}()
	for resultIns := range ch.Chan {
		result := resultIns.Plugin
		total := resultIns.Total
		if total > 0 {
			progress = count / float64(total)
		}
		count++
		err := client.Save(s.GetProfileDatabase(), result)
		if err != nil {
			stream.Send(&ypb.DownloadOnlinePluginProgress{
				Progress: progress,
				Log:      fmt.Sprintf("save [%s] to local db failed: %s", result.ScriptName, err),
			})
		} else {
			stream.Send(&ypb.DownloadOnlinePluginProgress{
				Progress: progress,
				Log:      fmt.Sprintf("save [%s] to local db finished", result.ScriptName),
			})
		}
	}
	return nil
}

func (s *Server) DeletePluginByUserID(ctx context.Context, req *ypb.DeletePluginByUserIDRequest) (*ypb.Empty, error) {
	err := yakit.DeleteYakScriptByUserID(s.GetProfileDatabase(), req.GetUserID(), req.GetOnlineBaseUrl())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteAllLocalPlugins(ctx context.Context, req *ypb.Empty) (*ypb.Empty, error) {
	if db := s.GetProfileDatabase().DropTableIfExists(&yakit.YakScript{}); db.Error == nil {
		if db := s.GetProfileDatabase().AutoMigrate(&yakit.YakScript{}); db.Error == nil {
			return &ypb.Empty{}, nil
		}
	}

	err := yakit.DeleteYakScriptAll(s.GetProfileDatabase())
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DeleteLocalPluginsByWhere(ctx context.Context, req *ypb.DeleteLocalPluginsByWhereRequest) (*ypb.Empty, error) {

	err := yakit.DeleteYakScriptByWhere(s.GetProfileDatabase(), req)
	if err != nil {
		return nil, err
	}
	return &ypb.Empty{}, nil
}

func (s *Server) DownloadOnlinePluginByScriptNames(ctx context.Context, req *ypb.DownloadOnlinePluginByScriptNamesRequest) (*ypb.DownloadOnlinePluginByScriptNamesResponse, error) {
	err := yaklib.DownloadOnlineAuthProxy(consts.GetOnlineBaseUrl())
	if err != nil {
		return nil, utils.Errorf("download failed: %s", err.Error())
	}
	YakPlugin := QueryYakScriptByNames(s.GetProfileDatabase(), req.GetScriptNames())
	var scriptNames []string
	for _, v := range req.GetScriptNames() {
		if !inContain(YakPlugin, v) {
			scriptNames = append(scriptNames, v)
		}
	}
	if scriptNames != nil {
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
		plugins := client.DownloadYakitScriptName(ctx, req.Token, scriptNames...)

		for pluginIns := range plugins.Chan {
			err := client.Save(s.GetProfileDatabase(), pluginIns.Plugin)
			if err != nil {
				log.Errorf("save err failed: %s", err)
			}
		}
		YakPlugin = append(YakPlugin, QueryYakScriptByNames(s.GetProfileDatabase(), scriptNames)...)
	}

	return &ypb.DownloadOnlinePluginByScriptNamesResponse{Data: YakPlugin}, nil
}

func inContain(YakPlugin []*ypb.DownloadOnlinePluginByScriptName, ScriptName string) bool {
	for _, v := range YakPlugin {
		if v.ScriptName == ScriptName {
			return true
		}
	}
	return false
}

func QueryYakScriptByNames(db *gorm.DB, req []string) []*ypb.DownloadOnlinePluginByScriptName {
	var YakPlugin []*ypb.DownloadOnlinePluginByScriptName
	for _, y := range yakit.QueryYakScriptByNames(db, req...) {
		YakPlugin = append(YakPlugin, &ypb.DownloadOnlinePluginByScriptName{
			ScriptName: y.ScriptName,
			Id:         int64(y.ID),
			HeadImg:    y.HeadImg,
		})
	}
	return YakPlugin
}

/*
 * new online
 * 插件下载
 */

func (s *Server) DownloadOnlinePluginBatch(ctx context.Context, req *ypb.DownloadOnlinePluginsRequest) (*ypb.Empty, error) {
	err := yaklib.DownloadOnlineAuthProxy(consts.GetOnlineBaseUrl())
	if err != nil {
		return nil, utils.Errorf("download failed: %s", err.Error())
	}
	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	plugins := client.DownloadOnlinePluginsBatch(ctx, req.Token, req.IsPrivate, req.Keywords, req.PluginType, req.Tags, req.UserName, req.UserId,
		req.TimeSearch, req.Group, req.ListType, req.Status, req.UUID, req.ScriptName)
	for pluginIns := range plugins.Chan {
		err := client.Save(s.GetProfileDatabase(), pluginIns.Plugin)
		if err != nil {
			log.Errorf("save err failed: %s", err)
		}
	}

	return &ypb.Empty{}, nil
}

func (s *Server) DownloadOnlinePlugins(req *ypb.DownloadOnlinePluginsRequest, stream ypb.Yak_DownloadOnlinePluginsServer) error {
	err := yaklib.DownloadOnlineAuthProxy(consts.GetOnlineBaseUrl())
	if err != nil {
		return utils.Errorf("download failed: %s", err.Error())
	}
	client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
	var ch *yaklib.OnlineDownloadStream
	ch = client.DownloadOnlinePluginsBatch(stream.Context(), req.Token, req.IsPrivate, req.Keywords, req.PluginType, req.Tags, req.UserName, req.UserId,
		req.TimeSearch, req.Group, req.ListType, req.Status, req.UUID, req.ScriptName)

	if ch == nil {
		return utils.Error("BUG: download stream error: empty")
	}

	var progress float64
	var count float64 = 0
	stream.Send(&ypb.DownloadOnlinePluginProgress{
		Progress: 0,
		Log:      "initializing",
	})
	defer func() {
		stream.Send(&ypb.DownloadOnlinePluginProgress{
			Progress: 1,
			Log:      "finished",
		})
	}()
	for resultIns := range ch.Chan {
		result := resultIns.Plugin
		total := resultIns.Total
		if total > 0 {
			progress = count / float64(total)
		}
		count++
		err = client.Save(s.GetProfileDatabase(), result)
		if err != nil {
			stream.Send(&ypb.DownloadOnlinePluginProgress{
				Progress: progress,
				Log:      fmt.Sprintf("save [%s] to local db failed: %s", result.ScriptName, err),
			})
		} else {
			stream.Send(&ypb.DownloadOnlinePluginProgress{
				Progress: progress,
				Log:      fmt.Sprintf("save [%s] to local db finished", result.ScriptName),
			})
		}
	}
	return nil
}

func (s *Server) DownloadOnlinePluginByPluginName(ctx context.Context, req *ypb.DownloadOnlinePluginByScriptNamesRequest) (*ypb.DownloadOnlinePluginByScriptNamesResponse, error) {
	err := yaklib.DownloadOnlineAuthProxy(consts.GetOnlineBaseUrl())
	if err != nil {
		return nil, utils.Errorf("download failed: %s", err.Error())
	}
	YakPlugin := QueryYakScriptByNames(s.GetProfileDatabase(), req.GetScriptNames())
	var scriptNames []string
	for _, v := range req.GetScriptNames() {
		if !inContain(YakPlugin, v) {
			scriptNames = append(scriptNames, v)
		}
	}
	if scriptNames != nil {
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
		plugins := client.DownloadOnlinePluginByPluginName(ctx, req.Token, req.ScriptNames)

		for pluginIns := range plugins.Chan {
			err := client.Save(s.GetProfileDatabase(), pluginIns.Plugin)
			if err != nil {
				log.Errorf("save err failed: %s", err)
			}
		}
		YakPlugin = append(YakPlugin, QueryYakScriptByNames(s.GetProfileDatabase(), scriptNames)...)
	}

	return &ypb.DownloadOnlinePluginByScriptNamesResponse{Data: YakPlugin}, nil
}

func (s *Server) SaveYakScriptToOnline(req *ypb.SaveYakScriptToOnlineRequest, stream ypb.Yak_SaveYakScriptToOnlineServer) error {
	if req.Token == "" {
		return utils.Errorf("未登录,请登录")
	}
	db := consts.GetGormProfileDatabase()
	if !req.GetAll() {
		if len(req.ScriptNames) <= 0 {
			return utils.Errorf("请选择上传插件")
		}
		db = bizhelper.ExactQueryStringArrayOr(db, "script_name", req.GetScriptNames())
	}
	ch := yakit.YieldYakScripts(db, context.Background())

	var progress float64
	var count float64 = 0
	errorCount := 0
	successCount := 0
	var message string
	messageType := "success"
	stream.Send(&ypb.SaveYakScriptToOnlineResponse{
		Progress:    0,
		Message:     "initializing",
		MessageType: "",
	})
	defer func() {
		if errorCount > 0 {
			message += fmt.Sprintf("执行失败: %v 个", errorCount)
			messageType = "finalError"
		}
		if successCount > 0 {
			message += fmt.Sprintf("执行成功: %v 个", successCount)
		}
		if message == "" {
			message = "finished"
		}
		stream.Send(&ypb.SaveYakScriptToOnlineResponse{
			Progress:    1,
			Message:     message,
			MessageType: messageType,
		})
	}()
	for result := range ch {
		total := len(req.ScriptNames)
		if total > 0 {
			progress = count / float64(total)
		}
		count++
		err := yaklib.DownloadOnlineAuthProxy(consts.GetOnlineBaseUrl())
		if err != nil {
			return utils.Errorf("save to online failed: %s", err.Error())
		}
		/*if result.OnlineBaseUrl == consts.GetOnlineBaseUrl() {
			errorCount++
			stream.Send(&ypb.SaveYakScriptToOnlineResponse{
				Progress:    progress,
				Message:     fmt.Sprintf("save [%s] to online db failed: %s", result.ScriptName, "该插件名已被占用"),
				MessageType: "error",
			})
		}*/
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
		err = client.SaveToOnline(stream.Context(), req, result)
		if err != nil {
			errorCount++
			stream.Send(&ypb.SaveYakScriptToOnlineResponse{
				Progress:    progress,
				Message:     fmt.Sprintf("save [%s] to online db failed: %s", result.ScriptName, err),
				MessageType: "error",
			})
		} else {
			successCount++
			stream.Send(&ypb.SaveYakScriptToOnlineResponse{
				Progress:    progress,
				Message:     fmt.Sprintf("save [%s] to online db finished", result.ScriptName),
				MessageType: "success",
			})

			plugins := client.DownloadOnlinePluginsBatch(context.Background(), req.Token, []bool{}, "", []string{}, []string{}, "", 0,
				"", []string{}, "mine", []int64{}, []string{}, []string{result.ScriptName})
			for pluginIns := range plugins.Chan {
				err = client.Save(s.GetProfileDatabase(), pluginIns.Plugin)
				if err != nil {
					log.Errorf("save err failed: %s", err)
				}
			}
		}
	}
	return nil
}
