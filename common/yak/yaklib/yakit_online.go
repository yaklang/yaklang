package yaklib

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jinzhu/gorm"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type QueryOnlinePluginRequest struct {
	Type       string   `json:"type"`
	UUID       []string `json:"uuid"`
	Token      string   `json:"token"`
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	BindMe     bool     `json:"bind_me"`
	Keywords   string   `json:"keywords"`
	PluginType string   `json:"plugin_type"`
	Status     string   `json:"status"`
	User       bool     `json:"user"`
	IsPrivate  string   `json:"is_private"`
	Tags       string   `json:"tags"`
	UserId     int64    `json:"user_id"`
	UserName   string   `json:"user_name"`
	ScriptName []string `json:"script_name"`
	TimeSearch string   `json:"time_search"`
	Group      string   `json:"group"`
}

type DownloadOnlinePluginRequest struct {
	Token      string   `json:"token"`
	IsPrivate  []bool   `json:"is_private"`
	Keywords   string   `json:"keywords"`
	PluginType []string `json:"plugin_type"`
	Tags       []string `json:"tags"`
	UserName   string   `json:"user_name"`
	UserId     int64    `json:"user_id"`
	TimeSearch string   `json:"time_search"`
	Group      []string `json:"group"`
	ListType   string   `json:"listType"`
	Page       int      `json:"page"`
	Limit      int      `json:"limit"`
	UUID       []string `json:"uuid"`
	Status     []int64  `json:"status"`
	ScriptName []string `json:"script_name"`
}

type OnlineClient struct {
	// https://192.168.1.1:8080
	BaseUrl string
	client  *http.Client
}

func NewOnlineClient(baseUrl string) *OnlineClient {
	if proxy := strings.TrimSpace(consts.GetOnlineBaseUrlProxy()); proxy != "" {
		return &OnlineClient{
			BaseUrl: baseUrl,
			client:  utils.NewDefaultHTTPClientWithProxy(proxy),
		}
	}
	return &OnlineClient{
		BaseUrl: baseUrl,
		client:  utils.NewDefaultHTTPClient(),
	}
}

func DownloadOnlineAuthProxy(baseUrl string) error {
	host, port, err := utils.ParseStringToHostPort(baseUrl)
	if err != nil {
		return utils.Errorf("parse url[%s] failed: %s", baseUrl, err)
	}
	proxy := strings.TrimSpace(consts.GetOnlineBaseUrlProxy())
	if proxy != "" {
		conn, err := netx.DialTCPTimeoutForceProxy(10*time.Second, utils.HostPort(host, port), proxy)
		if err != nil {
			return utils.Errorf("connect to [%s] via proxy[%v] failed: %s", consts.GetOnlineBaseUrl(), proxy, err.Error())
		}
		conn.Close()
	}
	return nil
}

type OnlinePluginParam struct {
	Field        string `json:"field"`
	DefaultValue string `json:"default_value"`
	TypeVerbose  string `json:"type_verbose"`
	FieldVerbose string `json:"field_verbose"`
	Help         string `json:"help"`
	Required     bool   `json:"required"`
	Group        string `json:"group"`
	ExtraSetting string `json:"extra_setting"`
}

type OnlinePaging struct {
	Page      int `json:"page"`
	Total     int `json:"total"`
	TotalPage int `json:"total_page"`
	Limit     int `json:"limit"`
}

type OnlineRiskDetail struct {
	CweId       string `json:"cwe_id"`
	RiskType    string `json:"risk_type"`
	Description string `json:"description"`
	Solution    string `json:"solution"`
}

type OnlinePlugin struct {
	Id                   int64                     `json:"id"`
	UpdatedAt            int64                     `json:"updated_at"`
	Type                 string                    `json:"type"`
	ScriptName           string                    `json:"script_name"`
	Content              string                    `json:"content"`
	PublishedAt          int64                     `json:"published_at"`
	Tags                 string                    `json:"tags"`
	DefaultOpen          bool                      `json:"default_open"`
	DownloadedTotal      int64                     `json:"downloaded_total"`
	Stars                int64                     `json:"stars"`
	Status               int64                     `json:"status"`
	Official             bool                      `json:"official"`
	IsPrivate            bool                      `json:"is_private"`
	Params               []*OnlinePluginParam      `json:"params"`
	UserId               int64                     `json:"user_id"`
	Author               string                    `json:"authors"`
	Help                 string                    `json:"help"`
	EnablePluginSelector bool                      `json:"enable_plugin_selector"`
	PluginSelectorTypes  string                    `json:"plugin_selector_types"`
	IsGeneralModule      bool                      `json:"is_general_module"`
	OnlineContributors   string                    `json:"online_contributors"`
	UUID                 string                    `json:"uuid"`
	HeadImg              string                    `json:"head_img"`
	BasePluginId         int64                     `json:"base_plugin_id"`
	Group                string                    `json:"group"`
	RiskType             string                    `json:"riskType"`
	RiskDetail           *OnlineRiskDetail         `json:"riskDetail"`
	RiskAnnotation       string                    `json:"risk_annotation"`
	IsCorePlugin         bool                      `json:"isCorePlugin"`
	CollaboratorInfo     []*OnlineCollaboratorInfo `json:"collaborator"`
}

type SaveYakScriptOnlineRequest struct {
	ScriptName           string               `json:"script_name"`
	Type                 string               `json:"type"`
	Content              string               `json:"content"`
	Params               []*OnlinePluginParam `json:"params"`
	Help                 string               `json:"help"`
	Tags                 []string             `json:"tags"`
	EnablePluginSelector bool                 `json:"enable_plugin_selector"`
	PluginSelectorTypes  string               `json:"plugin_selector_types"`
	IsGeneralModule      bool                 `json:"is_general_module"`
	IsPrivate            bool                 `json:"is_private"`
	RiskType             string               `json:"riskType"`
	RiskDetail           *OnlineRiskDetail    `json:"riskDetail"`
	RiskAnnotation       string               `json:"annotation"`
	IsCorePlugin         bool                 `json:"isCorePlugin"`
}

type OnlinePluginItem struct {
	Plugin *OnlinePlugin
	Total  int64
}

type OnlineDownloadStream struct {
	Total     int64
	Page      int64
	PageTotal int64
	Limit     int64
	Chan      chan *OnlinePluginItem
}

type OnlineCollaboratorInfo struct {
	HeadImg  string `json:"head_img"`
	UserName string `json:"user_name"`
}

func (s *OnlineClient) DownloadYakitPluginById(id string, token string) (*OnlinePlugin, error) {
	plugins, _, err := s.downloadYakitPlugin("", []string{id}, token, 1, 5, false, "", "", "", "", "", 0, "", nil, "", "")
	if err != nil {
		log.Errorf("download yakit plugin failed: %s", err)
		return nil, utils.Errorf("download yakit plugin failed: %s", err)
	}
	if len(plugins) > 0 {
		return plugins[0], nil
	}
	return nil, utils.Error("empty result for download yakit plugin...")
}

func (s *OnlineClient) DownloadYakitPluginByIdWithToken(id string, token string) (*OnlinePlugin, error) {
	plugins, _, err := s.downloadYakitPlugin("", []string{id}, token, 1, 5, false, "", "", "", "", "", 0, "", nil, "", "")
	if err != nil {
		log.Errorf("download yakit plugin failed: %s", err)
		return nil, utils.Errorf("download yakit plugin failed: %s", err)
	}
	if len(plugins) > 0 {
		return plugins[0], nil
	}
	return nil, utils.Error("empty result for download yakit plugin...")
}

func (s *OnlineClient) DownloadYakitPluginAll(
	ctx context.Context,
) *OnlineDownloadStream {
	return s.DownloadYakitPluginsEx(ctx, true, nil, "", false, "", "", "", "", "", 0, "", nil, "", "")
}

func (s *OnlineClient) DownloadYakitPluginAllWithToken(
	ctx context.Context, token string, keywords string, pluginType string, status string, isPrivate string, tags string, userId int64, userName string, timeSearch string, group string,
) *OnlineDownloadStream {
	return s.DownloadYakitPluginsEx(ctx, false, nil, token, false, keywords, pluginType, status, isPrivate, tags, userId, userName, nil, timeSearch, "")
}

func (s *OnlineClient) DownloadYakitPluginWithTokenBindMe(
	ctx context.Context, token string, keywords string, pluginType string, status string, isPrivate string, tags string, userId int64, userName string, timeSearch string, group string,
) *OnlineDownloadStream {
	return s.DownloadYakitPluginsEx(ctx, false, nil, token, true, keywords, pluginType, status, isPrivate, tags, userId, userName, nil, timeSearch, "")
}

func (s *OnlineClient) DownloadYakitPluginIDWithToken(
	ctx context.Context, token string, ids ...string) *OnlineDownloadStream {
	return s.DownloadYakitPluginsEx(ctx, false, ids, token, false, "", "", "", "", "", 0, "", nil, "", "")
}

func (s *OnlineClient) DownloadYakitScriptName(
	ctx context.Context, token string, scriptName ...string) *OnlineDownloadStream {
	return s.DownloadYakitPluginsEx(ctx, false, nil, token, false, "", "", "", "", "", 0, "", scriptName, "", "")
}

func (s *OnlineClient) DownloadYakitPluginsEx(
	ctx context.Context,
	all bool,
	ids []string,
	userToken string,
	bindMe bool,
	keywords string,
	pluginType string,
	status string,
	isPrivate string,
	tags string,
	userId int64,
	userName string,
	scriptName []string,
	timeSearch string,
	group string,
) *OnlineDownloadStream {
	var ch = make(chan *OnlinePluginItem, 10)
	var rsp = &OnlineDownloadStream{
		Total:     0,
		Page:      0,
		PageTotal: 0,
		Limit:     0,
		Chan:      ch,
	}
	go func() {
		defer close(ch)
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("recover online_plugin failed: %s", err)
			}
		}()

		var queryType = ""
		var list []string
		var token string
		if all {
			queryType = "all"
			list = nil
			token = userToken
		} else {
			list = ids
			token = userToken
		}

		var page = 0
		var retry = 0
		for {
			select {
			case <-ctx.Done():
			default:
			}
			page++

			// 设置超时处理的问题
		RETRYDOWNLOAD:
			plugins, paging, err := s.downloadYakitPlugin(queryType, list, token, page, 30, bindMe, keywords, pluginType, status, isPrivate, tags, userId, userName, scriptName, timeSearch, group)
			if err != nil {
				retry++
				if retry <= 5 {
					log.Errorf("[RETRYING]: download yakit plugin failed: %s", err)
					goto RETRYDOWNLOAD
				} else {
					break
				}
			} else {
				retry = 0
			}

			if paging != nil && rsp.Total <= 0 {
				rsp.Page = int64(paging.Page)
				rsp.Limit = int64(paging.Limit)
				rsp.PageTotal = int64(paging.TotalPage)
				rsp.Total = int64(paging.Total)
			}

			if len(plugins) > 0 {
				for _, plugin := range plugins {
					select {
					case ch <- &OnlinePluginItem{
						Plugin: plugin,
						Total:  rsp.Total,
					}:
					case <-ctx.Done():
						return
					}
				}
			} else {
				break
			}
		}
	}()
	return rsp
}

func (s *OnlineClient) downloadYakitPlugin(
	typeStr string, remoteId []string, token string,
	page int, limit int, bindMe bool, keywords string, pluginType string,
	status string, isPrivate string, tags string, userId int64, userName string, scriptName []string, timeSearch string, group string,
) ([]*OnlinePlugin, *OnlinePaging, error) {
	urlIns, err := url.Parse(s.genUrl("/api/plugin/download"))
	if err != nil {
		return nil, nil, utils.Errorf("parse url-instance failed: %s", err)
	}

	raw, err := json.Marshal(QueryOnlinePluginRequest{
		Type:       typeStr,
		UUID:       remoteId,
		Token:      token,
		Page:       page,
		Limit:      limit,
		BindMe:     bindMe,
		Keywords:   keywords,
		PluginType: pluginType,
		Status:     status,
		IsPrivate:  isPrivate,
		Tags:       tags,
		UserId:     userId,
		UserName:   userName,
		ScriptName: scriptName,
		TimeSearch: timeSearch,
		Group:      group,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal params failed: %s", err)
	}
	rsp, err := s.client.Post(urlIns.String(), "application/json", bytes.NewBuffer(raw))
	if err != nil {
		return nil, nil, utils.Errorf("HTTP Post %v failed: %v params:%s", urlIns.String(), err, string(raw))
	}
	rawResponse, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, nil, utils.Errorf("read body failed: %s", err)
	}
	type PluginDownloadResponse struct {
		Data     []*OnlinePlugin `json:"data"`
		Pagemeta *OnlinePaging   `json:"pagemeta"`
	}

	var _container PluginDownloadResponse
	err = json.Unmarshal(rawResponse, &_container)
	if err != nil {
		return nil, nil, utils.Errorf("unmarshal plugin response failed: %s", err)
	}
	return _container.Data, _container.Pagemeta, nil
}

func (s *OnlineClient) genUrl(path string) string {
	s.BaseUrl = strings.TrimRight(s.BaseUrl, "/")
	path = strings.TrimLeft(path, "/")
	return fmt.Sprintf("%v/%v", s.BaseUrl, path)
}

func (s *OnlineClient) Save(db *gorm.DB, plugins ...*OnlinePlugin) error {
	if db == nil {
		return utils.Error("empty database")
	}

	scripts := funk.Map(plugins, func(i *OnlinePlugin) *yakit.YakScript {
		var params []*ypb.YakScriptParam
		for _, paramInstance := range i.Params {
			params = append(params, &ypb.YakScriptParam{
				Field:        paramInstance.Field,
				DefaultValue: paramInstance.DefaultValue,
				TypeVerbose:  paramInstance.TypeVerbose,
				FieldVerbose: paramInstance.FieldVerbose,
				Help:         paramInstance.Help,
				Required:     paramInstance.Required,
				Group:        paramInstance.Group,
				ExtraSetting: paramInstance.ExtraSetting,
			})
		}
		raw, _ := json.Marshal(params)
		paramsStr := strconv.Quote(string(raw))

		existedYakScript, _ := yakit.GetYakScriptByName(db, i.ScriptName)
		if existedYakScript != nil {
			yakit.DeleteYakScriptByName(db, existedYakScript.ScriptName)
		}
		var scriptName = i.ScriptName

		var tags []string
		_ = json.Unmarshal([]byte(i.Tags), &tags)
		if len(tags) > 0 {
			tags = utils.RemoveRepeatStringSlice(tags)
		} else {
			tags = utils.RemoveRepeatStringSlice(utils.PrettifyListFromStringSplited(i.Tags, ","))
		}

		var onlineGroup []string
		_ = json.Unmarshal([]byte(i.Group), &onlineGroup)
		if len(onlineGroup) > 0 {
			onlineGroup = utils.RemoveRepeatStringSlice(onlineGroup)
		} else {
			onlineGroup = utils.RemoveRepeatStringSlice(utils.PrettifyListFromStringSplited(i.Group, ","))
		}

		y := &yakit.YakScript{
			ScriptName:           scriptName,
			OnlineScriptName:     i.ScriptName,
			Type:                 i.Type,
			Content:              i.Content,
			Params:               paramsStr,
			Help:                 i.Help,
			Author:               i.Author,
			Tags:                 strings.Join(tags, ","),
			IsGeneralModule:      i.IsGeneralModule,
			EnablePluginSelector: i.EnablePluginSelector,
			PluginSelectorTypes:  i.PluginSelectorTypes,
			OnlineId:             i.Id,
			OnlineContributors:   i.OnlineContributors,
			OnlineIsPrivate:      i.IsPrivate,
			UserId:               i.UserId,
			Uuid:                 i.UUID,
			HeadImg:              i.HeadImg,
			OnlineBaseUrl:        s.BaseUrl,
			BaseOnlineId:         i.BasePluginId,
			OnlineOfficial:       i.Official,
			OnlineGroup:          strings.Join(onlineGroup, ","),
			IsCorePlugin:         i.IsCorePlugin,
			RiskAnnotation:       i.RiskAnnotation,
			RiskType:             i.RiskType,
		}
		if i.RiskDetail != nil {
			risk := &ypb.QueryYakScriptRiskDetailByCWEResponse{
				CWEId:       i.RiskDetail.CweId,
				RiskType:    i.RiskDetail.RiskType,
				Description: i.RiskDetail.Description,
				CWESolution: i.RiskDetail.Solution,
			}
			rawRisk, _ := json.Marshal(risk)
			y.RiskDetail = string(rawRisk)
		}

		if i.CollaboratorInfo != nil {
			var collaboratorInfo []*ypb.Collaborator
			for _, collaborator := range i.CollaboratorInfo {
				collaboratorInfo = append(collaboratorInfo, &ypb.Collaborator{
					HeadImg:  collaborator.HeadImg,
					UserName: collaborator.UserName,
				})
			}
			collaboratorInfoRaw, _ := json.Marshal(collaboratorInfo)
			collaboratorInfoStr := strconv.Quote(string(collaboratorInfoRaw))
			y.CollaboratorInfo = collaboratorInfoStr
		}

		if y.OnlineContributors != "" && y.OnlineContributors != y.Author {
			y.Author = strings.Join([]string{y.Author, y.OnlineContributors}, ",")
			y.Author = strings.Join(utils.RemoveRepeatStringSlice(utils.PrettifyListFromStringSplited(y.Author, ",")), ",")
		}
		return y
	}).([]*yakit.YakScript)
	if len(scripts) < 0 {
		return utils.Error("empty plugins...")
	}

	if len(scripts) == 1 {
		//err := yakit.CreateOrUpdateYakScriptByOnlineId(db, scripts[0].OnlineId, scripts[0])
		err := yakit.CreateOrUpdateYakScriptByName(db, scripts[0].ScriptName, scripts[0])
		if err != nil {
			log.Errorf("save [%s] to local failed: %s", scripts[0].ScriptName, err)
			return err
		}
	}

	for _, i := range scripts {
		//err := yakit.CreateOrUpdateYakScriptByOnlineId(db, i.OnlineId, i)
		err := yakit.CreateOrUpdateYakScriptByName(db, i.ScriptName, i)
		if err != nil {
			log.Errorf("save [%s] to local failed: %s", i.ScriptName, err)
		}
	}
	return nil
}

func (s *OnlineClient) DownloadOnlinePluginsBatch(
	ctx context.Context, token string,
	isPrivate []bool,
	keywords string,
	pluginType []string,
	tags []string,
	userName string,
	userId int64,
	timeSearch string,
	group []string,
	listType string,
	status []int64,
	uuid []string,
	scriptName []string,

) *OnlineDownloadStream {
	return s.DownloadNewOnlinePlugins(ctx, token, isPrivate, keywords, pluginType, tags, userName, userId, timeSearch, group, listType, status, uuid, scriptName)
}

func (s *OnlineClient) DownloadOnlinePluginByPluginName(
	ctx context.Context, token string, scriptName []string) *OnlineDownloadStream {
	return s.DownloadNewOnlinePlugins(ctx, token, nil, "", nil, nil, "", 0, "", nil, "", nil, nil, scriptName)
}

func (s *OnlineClient) DownloadNewOnlinePlugins(
	ctx context.Context,
	token string,
	isPrivate []bool,
	keywords string,
	pluginType []string,
	tags []string,
	userName string,
	userId int64,
	timeSearch string,
	group []string,
	listType string,
	status []int64,
	uuid []string,
	scriptName []string,
) *OnlineDownloadStream {
	var ch = make(chan *OnlinePluginItem, 10)
	var rsp = &OnlineDownloadStream{
		Total:     0,
		Page:      0,
		PageTotal: 0,
		Limit:     0,
		Chan:      ch,
	}
	go func() {
		defer close(ch)
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("recover online_plugin failed: %s", err)
			}
		}()

		var page = 0
		var retry = 0
		for {
			select {
			case <-ctx.Done():
			default:
			}
			page++

			// 设置超时处理的问题
		RETRYDOWNLOAD:
			plugins, paging, err := s.downloadNewOnlinePlugins(token, isPrivate, keywords, pluginType, tags, userName, userId, timeSearch, group, listType, status, uuid, scriptName, page, 30)
			if err != nil {
				retry++
				if retry <= 5 {
					log.Errorf("[RETRYING]: download yakit plugin failed: %s", err)
					goto RETRYDOWNLOAD
				} else {
					break
				}
			} else {
				retry = 0
			}

			if paging != nil && rsp.Total <= 0 {
				rsp.Page = int64(paging.Page)
				rsp.Limit = int64(paging.Limit)
				rsp.PageTotal = int64(paging.TotalPage)
				rsp.Total = int64(paging.Total)
			}

			if len(plugins) > 0 {
				for _, plugin := range plugins {
					select {
					case ch <- &OnlinePluginItem{
						Plugin: plugin,
						Total:  rsp.Total,
					}:
					case <-ctx.Done():
						return
					}
				}
			} else {
				break
			}
		}
	}()
	return rsp
}

func (s *OnlineClient) downloadNewOnlinePlugins(
	token string,
	isPrivate []bool,
	keywords string,
	pluginType []string,
	tags []string,
	userName string,
	userId int64,
	timeSearch string,
	group []string,
	listType string,
	status []int64,
	uuid []string,
	scriptName []string,
	page int, limit int,
) ([]*OnlinePlugin, *OnlinePaging, error) {
	urlIns, err := url.Parse(s.genUrl("/api/plugins/download"))
	if err != nil {
		return nil, nil, utils.Errorf("parse url-instance failed: %s", err)
	}

	raw, err := json.Marshal(DownloadOnlinePluginRequest{
		UUID:       uuid,
		Token:      token,
		Page:       page,
		Limit:      limit,
		Keywords:   keywords,
		PluginType: pluginType,
		Status:     status,
		IsPrivate:  isPrivate,
		Tags:       tags,
		UserId:     userId,
		UserName:   userName,
		ScriptName: scriptName,
		TimeSearch: timeSearch,
		Group:      group,
		ListType:   listType,
	})
	if err != nil {
		return nil, nil, utils.Errorf("marshal params failed: %s", err)
	}
	rsp, err := s.client.Post(urlIns.String(), "application/json", bytes.NewBuffer(raw))
	if err != nil {
		return nil, nil, utils.Errorf("HTTP Post %v failed: %v params:%s", urlIns.String(), err, string(raw))
	}
	rawResponse, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return nil, nil, utils.Errorf("read body failed: %s", err)
	}
	type PluginDownloadResponse struct {
		Data     []*OnlinePlugin `json:"data"`
		Pagemeta *OnlinePaging   `json:"pagemeta"`
	}
	type OnlineErr struct {
		Form   string `json:"form"`
		Reason string `json:"reason"`
		Ok     bool   `json:"ok"`
	}
	var _container PluginDownloadResponse
	var ret OnlineErr
	err = json.Unmarshal(rawResponse, &_container)
	if err != nil {
		return nil, nil, utils.Errorf("unmarshal plugin response failed: %s", err.Error())
	}
	err = json.Unmarshal(rawResponse, &ret)
	if ret.Reason != "" {
		return nil, nil, utils.Errorf("unmarshal plugin response failed: %s", ret.Reason)
	}
	return _container.Data, _container.Pagemeta, nil
}

func (s *OnlineClient) SaveToOnline(ctx context.Context, req *ypb.SaveYakScriptToOnlineRequest, plugin *yakit.YakScript) error {
	err := s.SaveYakScriptToOnline(ctx,
		req.Token,
		plugin.ScriptName,
		plugin.Type,
		plugin.Content,
		plugin.Params,
		plugin.Help,
		plugin.Tags,
		plugin.EnablePluginSelector,
		plugin.PluginSelectorTypes,
		plugin.IsGeneralModule,
		req.IsPrivate,
		plugin.RiskType,
		plugin.RiskDetail,
		plugin.RiskAnnotation,
		plugin.IsCorePlugin,
	)
	if err != nil {
		log.Errorf("save yakScript to online failed: %s", err.Error())
		return utils.Errorf(err.Error())
	}

	return nil
}

func (s *OnlineClient) SaveYakScriptToOnline(ctx context.Context,
	token string, scriptName string, pluginType, content, params, help, tags string, enablePluginSelector bool, pluginSelectorTypes string, isGeneralModule, isPrivate bool, riskType, riskDetail, riskAnnotation string, isCorePlugin bool) error {
	urlIns, err := url.Parse(s.genUrl("/api/plugins"))
	if err != nil {
		return utils.Errorf("parse url-instance failed: %s", err)
	}
	var riskDetailJson *OnlineRiskDetail
	var paramsJson []*OnlinePluginParam
	_ = json.Unmarshal([]byte(riskDetail), &riskDetailJson)
	_ = json.Unmarshal([]byte(params), &paramsJson)
	tagsJson := strings.Split(tags, ",")
	raw, err := json.Marshal(SaveYakScriptOnlineRequest{
		ScriptName:           scriptName,
		Type:                 pluginType,
		Content:              content,
		Params:               paramsJson,
		Help:                 help,
		Tags:                 tagsJson,
		EnablePluginSelector: enablePluginSelector,
		PluginSelectorTypes:  pluginSelectorTypes,
		IsGeneralModule:      isGeneralModule,
		IsPrivate:            isPrivate,
		RiskType:             riskType,
		RiskDetail:           riskDetailJson,
		RiskAnnotation:       riskAnnotation,
		IsCorePlugin:         isCorePlugin,
	})

	if err != nil {
		return utils.Errorf("marshal params failed: %s", err)
	}

	req, err := http.NewRequest("POST", urlIns.String(), bytes.NewBuffer(raw))
	if err != nil {
		return utils.Errorf(err.Error())
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	rsp, err := s.client.Do(req)

	if err != nil {
		return utils.Errorf("HTTP Post %v failed: %v params:%s", urlIns.String(), err, string(raw))
	}
	rawResponse, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return utils.Errorf("read body failed: %s", err)
	}
	var responseData map[string]interface{}
	err = json.Unmarshal(rawResponse, &responseData)
	if err != nil {
		return utils.Errorf("unmarshal upload risk to online response failed: %s", err)
	}
	if utils.MapGetString(responseData, "message") != "" || utils.MapGetString(responseData, "reason") != "" {
		return utils.Errorf("save yakScript to online failed: %s %s", utils.MapGetString(responseData, "reason"), utils.MapGetString(responseData, "message"))
	}

	return nil
}
