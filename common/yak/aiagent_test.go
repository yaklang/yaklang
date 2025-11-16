package yak

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/jinzhu/gorm"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/aireducer"
	"github.com/yaklang/yaklang/common/chunkmaker"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/utils/omap"
	"github.com/yaklang/yaklang/common/utils/permutil"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestBuildInForge(t *testing.T) {
	yakit.InitialDatabase()
	res, err := ExecuteForge("long_text_summarizer",
		[]*ypb.ExecParamItem{
			//{Key: "text", Value: monOncleJules},
			{Key: "filePath", Value: "我的叔叔于勒.txt"},
		},
		WithAICallback(aiforge.GetHoldAICallback()),
		WithExtendAICommonOptions(
			aicommon.WithDebugPrompt(true),
		),
	)
	require.NoError(t, err)
	spew.Dump(res)

	//res, err := ExecuteForge("yaklang_writer",
	//	"写一个目录扫描脚本",
	//	aicommon.WithDebugPrompt(true),
	//	aid.WithAICallback(aiforge.GetHoldAICallback()),
	//	aid.WithAgreeYOLO(true),
	//)
	//require.NoError(t, err)
	//spew.Dump(res)
}

func TestReducerAI(t *testing.T) {
	yakit.InitialDatabase()
	raw, err := os.ReadFile("我的叔叔于勒.txt")
	require.NoError(t, err)
	memory := aid.GetDefaultContextProvider()

	key := "前情提要"
	reducer, err := aireducer.NewReducerFromString(
		string(raw),
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			textSnippet := string(chunk.Data())
			preData, _ := memory.GetPersistentData(key)
			if preData != "" {
				textSnippet = key + " : " + preData + "\n" + textSnippet
			}
			res, err := ExecuteForge("fragment_summarizer",
				[]*ypb.ExecParamItem{
					{
						Key: "textSnippet", Value: textSnippet,
					}, {
						Key: "limit", Value: "1000",
					},
				},
				WithAICallback(aiforge.GetHoldAICallback()),
				WithExtendAICommonOptions(
					aicommon.WithDebugPrompt(true),
				),
				aicommon.WithDisallowRequireForUserPrompt(),
			)
			if err != nil {
				return err
			}
			memory.SetPersistentData(key, utils.InterfaceToString(res))
			spew.Dump(res)
			return nil
		}),
		aireducer.WithMemory(memory),
	)
	require.NoError(t, err)
	err = reducer.Run()
	require.NoError(t, err)

	spew.Dump(memory.GetPersistentData(key))

}

func TestReducerAI2(t *testing.T) {
	yakit.InitialDatabase()
	raw, err := os.ReadFile("我的叔叔于勒.txt")
	require.NoError(t, err)
	memory := aid.GetDefaultContextProvider()

	reducer, err := aireducer.NewReducerFromString(
		string(raw),
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			textSnippet := string(chunk.Data())
			res, err := ExecuteForge("biography",
				[]*ypb.ExecParamItem{
					{
						Key: "text", Value: textSnippet,
					},
				},
				WithAICallback(aiforge.GetHoldAICallback()),
				WithExtendAICommonOptions(
					aicommon.WithDebugPrompt(true),
				),
				WithDisallowRequireForUserPrompt(),
				WithPromptContextProvider(memory.CopyReducibleMemory()),
			)
			if err != nil {
				return err
			}
			memory.ApplyOp(res.(*aiforge.ForgeResult).Action)
			return nil
		}),
		aireducer.WithMemory(memory),
	)
	require.NoError(t, err)
	err = reducer.Run()
	require.NoError(t, err)
	fmt.Println(memory.PersistentMemory())
}

func TestReducerIntentRecognition(t *testing.T) {
	yakit.InitialDatabase()
	raw := "我想做渗透测试\n\n可能需要用到xss攻击。\n\nsql注入\n\n还是不测xss了"

	ctx := context.Background()
	cod, err := aid.NewCoordinatorContext(ctx, "", aicommon.WithAICallback(aiforge.GetHoldAICallback()))
	require.NoError(t, err)
	memory := cod.Memory

	searchHandler := func(query string, searchList []*schema.AIForge) ([]*schema.AIForge, error) {
		keywords := omap.NewOrderedMap[string, []string](nil)
		forgeMap := map[string]*schema.AIForge{}
		for _, forge := range searchList {
			keywords.Set(forge.GetName(), forge.GetKeywords())
			forgeMap[forge.GetName()] = forge
		}
		searchResults, err := cod.HandleSearch(query, keywords)
		if err != nil {
			return nil, err
		}
		forges := []*schema.AIForge{}
		for _, result := range searchResults {
			forges = append(forges, forgeMap[result.Key])
		}
		return forges, nil
	}

	getForge := func() []*schema.AIForge {
		forgeList, err := yakit.GetAllAIForge(consts.GetGormProfileDatabase())
		if err != nil {
			log.Errorf("yakit.GetAllAIForge: %v", err)
			return nil
		}
		return forgeList
	}

	reducer, err := aireducer.NewReducerFromString(
		raw,
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			query := string(chunk.Data())
			go func() {
				//subCtx, cancel := context.WithCancel(ctx)
				//defer cancel()
				res, err := ExecuteForge("intent_recognition",
					query,
					WithAICallback(aiforge.GetHoldAICallback()),
					WithDisallowRequireForUserPrompt(),
					WithPromptContextProvider(memory),
				)
				if err != nil {
					log.Errorf("ExecuteForge: %v", err)
					return
				}

				resString := utils.InterfaceToString(res)
				fmt.Println(resString)
				if resString != "" {
					forgeList, err := searchHandler(resString, getForge())
					if err != nil {
						log.Errorf("searchHandler: %v", err)
						return
					}
					//var opts []*aid.RequireInteractiveRequestOption
					for idx, opt := range forgeList {
						_ = idx
						fmt.Printf("%d\t%s:[%s]\n", idx, opt.ForgeName, opt.Description)
						//opts = append(opts, &aid.RequireInteractiveRequestOption{
						//	Index:  idx,
						//	Prompt: opt.ForgeName,
						//})
					}
					//param, _, err := cod.GetConfig().RequireUserPromptWithEndpointResultEx(subCtx, "")
					//if err != nil {
					//	return
					//}
					//spew.Dump(param)
				}
			}()
			memory.PushUserInteraction(aicommon.UserInteractionStage_FreeInput, cod.AcquireId(), "", query) // push user input timeline
			return nil
		}),
		aireducer.WithSeparatorTrigger("\n\n"),
		aireducer.WithContext(ctx),
		aireducer.WithMemory(memory),
	)
	require.NoError(t, err)
	err = reducer.Run()
	require.NoError(t, err)
	select {}
}

func checkAndTryFixDatabase(path string) error {

	if exist, err := utils.PathExists(path); err != nil {
		log.Errorf("check dir[%v] if exist failed: %s", path, err)
	} else if !exist {
		_, err := os.Create(path)
		if err != nil {
			log.Errorf("make dir[%v] failed: %s", path, err)
		}
	}

	if runtime.GOOS == "darwin" {
		if permutil.IsFileUnreadAndUnWritable(path) {
			log.Infof("打开数据库[%s]遇到权限问题，尝试自主修复数据库权限错误", path)
			if err := permutil.DarwinSudo(
				"chmod +rw "+strconv.Quote(path),
				permutil.WithVerbose(fmt.Sprintf("修复 Yakit 数据库[%s]权限", path)),
			); err != nil {
				log.Errorf("sudo chmod +rw %v failed: %v", strconv.Quote(path), err)
			}
			if permutil.IsFileUnreadAndUnWritable(path) {
				log.Errorf("No Permission for %v", path)
			}
		}
	}
	err := os.Chmod(path, 0o666)
	if err != nil {
		log.Errorf("chmod +rw failed: %s", err)
	}
	return nil
}

func NewTestWebLogEventDB(path string) (*gorm.DB, error) {
	err := checkAndTryFixDatabase(path)
	if err != nil {
		return nil, err
	}
	path = fmt.Sprintf("%s?cache=shared&mode=rwc", path)
	db, err := gorm.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	db.AutoMigrate(WebLogEvent{}, Entity{})
	return db, nil
}

type WebLogEvent struct {
	gorm.Model
	SourceIP       string
	RequestMethod  string
	RequestURI     string
	EventTime      time.Time
	UserAgent      string
	StatusCode     int64
	InferredStatus string
	ErrorMessage   string
	LogType        string
}

func parseISO(isoTime string) time.Time {
	t, err := time.Parse(time.RFC3339, isoTime) // 尝试RFC3339格式
	if err != nil {
		// 尝试无时区格式（作为UTC处理）
		t, err = time.ParseInLocation("2006-01-02T15:04:05", isoTime, time.UTC)
	}
	if err != nil {
		return time.Time{}
	}
	return t
}

func SaveEvent(db *gorm.DB, event *WebLogEvent) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	result := db.Create(event)
	if result.Error != nil {
		return fmt.Errorf("failed to save event: %v", result.Error)
	}
	return nil
}

func QueryRecentEvent(db *gorm.DB, sourceIP string, duration time.Duration) ([]*WebLogEvent, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection is nil")
	}
	var events []*WebLogEvent
	//if db := db.Where("source_ip = ?", sourceIP).Where("event_time > ?", time.Now().Truncate(duration)).Find(&events); db.Error != nil {
	//	return nil, fmt.Errorf("failed to query events: %v", db.Error)
	//}
	if db := db.Where("source_ip = ?", sourceIP).Find(&events); db.Error != nil {
		return nil, fmt.Errorf("failed to query events: %v", db.Error)
	}
	return events, nil
}

type Entity struct {
	Value  string `json:"value"`
	Type   string `json:"type"`
	Remark string `json:"remark"`
}

func SaveEntity(db *gorm.DB, e *Entity) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if e == nil {
		return fmt.Errorf("event is nil")
	}
	if db := db.Where("value = ? AND type = ?", e.Value, e.Type).Assign(e).FirstOrCreate(&Entity{}); db.Error != nil {
		return utils.Errorf("create/update enity failed: %s", db.Error)
	}
	return nil
}

func UpdateEntityRemark(db *gorm.DB, e *Entity, remark string) error {
	if db == nil {
		return fmt.Errorf("database connection is nil")
	}
	if e == nil {
		return fmt.Errorf("event is nil")
	}
	if db := db.Where("value = ? AND type = ?", e.Value, e.Type).UpdateColumn("remark", remark); db.Error != nil {
		return utils.Errorf("create/update enity failed: %s", db.Error)
	}
	return nil
}

type EntityWatcher struct {
	triggerTime  time.Duration
	triggerCount int
	watchingMap  map[string]*chanx.UnlimitedChan[struct{}]
	mu           sync.Mutex
}

func NewEntityWatcher(triggerTime time.Duration, triggerCount int) *EntityWatcher {
	return &EntityWatcher{
		triggerTime:  triggerTime,
		triggerCount: triggerCount,
		watchingMap:  make(map[string]*chanx.UnlimitedChan[struct{}]),
	}
}
func (ew *EntityWatcher) StopWatch(entityValue string) {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	if ch, exists := ew.watchingMap[entityValue]; exists {
		ch.CloseForce()
		delete(ew.watchingMap, entityValue)
	}
}

func (ew *EntityWatcher) WatchEntity(entityValue string, callback func(entityValue string)) {
	ew.mu.Lock()
	defer ew.mu.Unlock()

	if ch, exists := ew.watchingMap[entityValue]; !exists {
		ctx, cancel := context.WithCancel(context.Background())
		watchChannel := chanx.NewUnlimitedChan[struct{}](ctx, 2)
		ew.watchingMap[entityValue] = watchChannel
		go func() {
			defer ew.StopWatch(entityValue)
			defer cancel()
			triggerCount := ew.triggerCount
			count := 0

			tr := time.NewTimer(ew.triggerTime)
			var ok bool
			for !ok {
				select {
				case <-watchChannel.OutputChannel():
					count++
					if count >= triggerCount {
						ok = true
					}
				case <-tr.C:
					ok = true
				}
			}

			callback(entityValue)

		}()
	} else {
		ch.SafeFeed(struct{}{})
	}
}

//go:embed testdata/test_ai_weblog.gz
var testAIWeblogGZIP []byte

func TestWebLogMonitor(t *testing.T) {
	db, err := NewTestWebLogEventDB(filepath.Join(consts.GetDefaultYakitBaseTempDir(), uuid.New().String()))
	if err != nil {
		return
	}
	aiCB := aiforge.GetOpenRouterAICallbackGemini2_5flash()
	//aiCB = aiforge.GetAIBalance()

	update := func(attackType string, entityValue string) {
		entity := &Entity{
			Value: entityValue,
			Type:  "ip_address",
		}
		err := UpdateEntityRemark(db, entity, fmt.Sprintf("Detected %s attack from %s", attackType, entityValue))
		if err != nil {
			log.Errorf("UpdateEntityRemark failed: %v", err)
			return
		}
	}

	analyzerWebRequest := func(sourceIP string) {
		event, err := QueryRecentEvent(db, sourceIP, time.Minute*5)
		if err != nil {
			return
		}
		if len(event) == 0 {
			return
		}
		eventJsonString, err := json.Marshal(event)
		if err != nil {
			return
		}

		res, err := ExecuteForge("event_analyzer",
			string(eventJsonString),
			WithAICallback(aiCB),
			WithDisallowRequireForUserPrompt(),
		)
		if err != nil {
			return
		}
		report := res.(aitool.InvokeParams)
		if report.GetBool("is_malicious") {
			fmt.Printf("!!![ATTACK]: detect %s %s \n[confidence_score]:%f\n[behavior_summary]:%s\n[key_evidence]:%s\n", sourceIP, report.GetString("attack_type"), report.GetFloat("confidence_score"), report.GetString("behavior_summary"), spew.Sdump(report.GetStringSlice("key_evidence")))
		}
		update(report.GetString("attack_type"), sourceIP)
	}

	yakit.InitialDatabase()
	content, err := utils.GzipDeCompress(testAIWeblogGZIP)
	require.NoError(t, err)
	fp := bytes.NewReader(content)

	ctx := context.Background()
	cod, err := aid.NewCoordinatorContext(ctx, "", aicommon.WithAICallback(aiCB))
	require.NoError(t, err)
	memory := cod.Memory

	var cacheBuffer []string

	entityMarshal := func(e aitool.InvokeParams) *Entity {
		return &Entity{
			Value: e.GetString("entity_value"),
			Type:  e.GetString("entity_type"),
		}
	}

	eventMarshal := func(e aitool.InvokeParams) *WebLogEvent {
		return &WebLogEvent{
			SourceIP:       e.GetString("source_ip"),
			RequestMethod:  e.GetString("request_method"),
			RequestURI:     e.GetString("request_uri"),
			EventTime:      parseISO(e.GetString("timestamp")),
			UserAgent:      e.GetString("user_agent"),
			StatusCode:     e.GetInt("status_code"),
			InferredStatus: e.GetString("inferred_status"),
			ErrorMessage:   e.GetString("error_message"),
			LogType:        e.GetString("log_type"),
		}
	}

	ew := NewEntityWatcher(time.Minute*5, 20)

	reducer, err := aireducer.NewReducerFromReader(
		fp,
		aireducer.WithReducerCallback(func(config *aireducer.Config, memory *aid.PromptContextProvider, chunk chunkmaker.Chunk) error {
			cacheBuffer = append(cacheBuffer, string(chunk.Data()))
			if len(cacheBuffer) < 10 {
				return nil
			}
			defer func() {
				cacheBuffer = make([]string, 0)
			}()
			logBuffer := strings.Join(cacheBuffer, "\n")
			wg := sync.WaitGroup{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				res, err := ExecuteForge("entity_identify",
					logBuffer,
					WithAICallback(aiCB),
					WithDisallowRequireForUserPrompt(),
					WithPromptContextProvider(memory),
				)
				if err != nil {
					return
				}
				for _, params := range res.([]aitool.InvokeParams) {
					err := SaveEntity(db, entityMarshal(params))
					if err != nil {
						log.Errorf("failed to save entity: %v", err)
						return
					}
				}
			}()

			wg.Add(1)
			go func() {
				defer wg.Done()
				res, err := ExecuteForge("log_event_formatter",
					logBuffer,
					WithAICallback(aiCB),
					WithDisallowRequireForUserPrompt(),
					WithPromptContextProvider(memory),
				)
				if err != nil {
					return
				}
				for _, params := range res.([]aitool.InvokeParams) {
					event := eventMarshal(params)
					if event.LogType == "WEB_REQUEST" {
						ew.WatchEntity(event.SourceIP, analyzerWebRequest)
					}
					err := SaveEvent(db, event)
					if err != nil {
						log.Errorf("failed to save event: %v", err)
						return
					}
				}
			}()

			wg.Wait()
			return nil
		}),
		aireducer.WithSeparatorTrigger("[INFO]"),
		aireducer.WithContext(ctx),
		aireducer.WithMemory(memory),
	)
	require.NoError(t, err)
	err = reducer.Run()
	require.NoError(t, err)
	select {}
}

func TestWebLogMonitorForge(t *testing.T) {
	yakit.InitialDatabase()
	_, err := ExecuteForge("web_log_monitor", []*ypb.ExecParamItem{
		//{Key: "text", Value: monOncleJules},

	}, WithAICallback(aiforge.GetOpenRouterAICallbackGemini2_5flash()))
	if err != nil {
		return
	}

}
