package yakgrpc

import (
	"bytes"
	"container/list"
	"context"
	_ "embed"
	uuid "github.com/satori/go.uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/mfreader"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"sync"
	"sync/atomic"
)

func (s *Server) hybridScanNewTask(stream ypb.Yak_HybridScanServer, firstRequest *ypb.HybridScanRequest) error {
	var (
		concurrent = firstRequest.GetConcurrent()
	)
	if concurrent <= 0 {
		concurrent = 20
	}
	swg := utils.NewSizedWaitGroup(int(concurrent))

	// read targets and plugin
	var target *ypb.HybridScanInputTarget
	var plugin *ypb.HybridScanPluginConfig
	for plugin == nil || target == nil {
		rsp, err := stream.Recv()
		if err != nil {
			return err
		}
		if target == nil {
			target = rsp.GetTargets()
		}
		if plugin == nil {
			plugin = rsp.GetPlugin()
		}
	}

	targetChan, err := s.TargetGenerator(stream.Context(), target)
	if err != nil {
		return utils.Errorf("TargetGenerator failed: %s", err)
	}

	cache := list.New()
	taskId := uuid.NewV4().String()

	/*
		统计状态
	*/
	var totalTarget = int64(len(utils.ParseStringToLines(target.String())))
	var targetCount int64 = 0
	addTargetCount := func() {
		if atomic.AddInt64(&targetCount, 1) > totalTarget {
			atomic.AddInt64(&totalTarget, 1)
		}
	}
	var targetFinished int64 = 0
	var taskFinished int64 = 0
	var totalPlugin int64 = s.CalcTotalPlugin(plugin)
	var getTotalTasks = func() int64 {
		return totalTarget * totalPlugin
	}
	var activeTask int64
	var activeTarget int64

	for __currentTarget := range targetChan {
		addTargetCount()
		atomic.AddInt64(&activeTarget, 1)

		pluginChan, err := s.PluginGenerator(cache, stream.Context(), plugin)
		if err != nil {
			return utils.Errorf("generate plugin queue failed: %s", err)
		}
		targetWg := new(sync.WaitGroup)
		for __pluginInstance := range pluginChan {
			targetRequestInstance := __currentTarget
			pluginInstance := __pluginInstance
			targetWg.Add(1)
			swg.Add()
			atomic.AddInt64(&activeTask, 1)
			go func() {
				defer swg.Done()

				defer targetWg.Done()
				defer func() {
					atomic.AddInt64(&taskFinished, 1)
					atomic.AddInt64(&activeTask, -1)
				}()
				err := s.ScanTargetWithPlugin(taskId, stream.Context(), targetRequestInstance, pluginInstance, func(result *ypb.ExecResult) {
					stream.Send(&ypb.HybridScanResponse{
						TotalTargets:      totalTarget,
						TotalPlugins:      totalPlugin,
						TotalTasks:        getTotalTasks(),
						FinishedTasks:     taskFinished,
						FinishedTargets:   targetFinished,
						ActiveTasks:       activeTask,
						ActiveTargets:     activeTarget,
						HybridScanTaskId:  taskId,
						CurrentPluginName: pluginInstance.ScriptName,
						ExecResult:        result,
					})
				})
				if err != nil {
					log.Warnf("scan target failed: %s", err)
				}
			}()
		}
		go func() {
			targetWg.Wait()
			atomic.AddInt64(&targetFinished, 1)
			atomic.AddInt64(&activeTarget, -1)
		}()
	}
	swg.Wait()
	return nil
}

//go:embed grpc_z_hybrid_scan.yak
var execTargetWithPluginScript string

func (s *Server) ScanTargetWithPlugin(
	taskId string, ctx context.Context, target *HybridScanTarget, plugin *yakit.YakScript, callback func(result *ypb.ExecResult),
) error {
	feedbackClient := yaklib.NewVirtualYakitClient(func(i *ypb.ExecResult) error {
		callback(i)
		return nil
	})
	engine := yak.NewYakitVirtualClientScriptEngine(feedbackClient)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVar("RUNTIME_ID", taskId)
		yak.BindYakitPluginContextToEngine(engine, &yak.YakitPluginContext{
			PluginName: plugin.ScriptName,
			RuntimeId:  taskId,
			Proxy:      "",
		})
		engine.SetVar("REQUEST", target.Request)
		engine.SetVar("HTTPS", target.IsHttps)
		engine.SetVar("PLUGIN", plugin)
		engine.SetVar("CTX", ctx)
		return nil
	})
	err := engine.ExecuteWithContext(ctx, execTargetWithPluginScript)
	if err != nil {
		return utils.Errorf("execute script failed: %s", err)
	}
	return nil
}

func (s *Server) PluginGenerator(l *list.List, ctx context.Context, plugin *ypb.HybridScanPluginConfig) (chan *yakit.YakScript, error) {
	if l.Len() > 0 {
		// use cache
		front := l.Front()
		outC := make(chan *yakit.YakScript)
		go func() {
			defer func() {
				close(outC)
			}()
			for {
				if front == nil {
					break
				}
				outC <- front.Value.(*yakit.YakScript)
				front = front.Next()
			}
		}()
		return outC, nil
	}
	var outC = make(chan *yakit.YakScript)
	go func() {
		defer close(outC)

		for _, i := range plugin.GetPluginNames() {
			script, err := yakit.GetYakScriptByName(s.GetProfileDatabase().Model(&yakit.YakScript{}), i)
			if err != nil {
				continue
			}
			l.PushBack(script)
			outC <- script
		}
		for pluginInstance := range yakit.YieldYakScripts(yakit.FilterYakScript(
			s.GetProfileDatabase().Model(&yakit.YakScript{}), plugin.GetFilter(),
		), ctx) {
			l.InsertAfter(pluginInstance, l.Back())
			outC <- pluginInstance
		}
	}()
	return outC, nil

}

type HybridScanTarget struct {
	IsHttps bool
	Request []byte
}

func (s *Server) CalcTotalPlugin(c *ypb.HybridScanPluginConfig) int64 {
	var total int64 = 0
	total += int64(len(c.GetPluginNames()))
	var count int64
	yakit.FilterYakScript(consts.GetGormProfileDatabase().Model(&yakit.YakScript{}), c.GetFilter()).Count(&count)
	total += count
	return total
}

func (s *Server) TargetGenerator(ctx context.Context, targetConfig *ypb.HybridScanInputTarget) (chan *HybridScanTarget, error) {
	// handle target
	requestTemplates, err := s.HTTPRequestBuilder(ctx, targetConfig.GetHTTPRequestTemplate())
	if err != nil {
		log.Warn(err)
	}
	var templates = requestTemplates.GetResults()
	if len(templates) == 0 {
		templates = append(templates, &ypb.HTTPRequestBuilderResult{
			HTTPRequest: []byte("GET / HTTP/1.1\r\nHost: {{Hostname}}"),
		})
	}

	var target = targetConfig.GetInput()
	var files = targetConfig.GetInputFile()

	outC := make(chan string)
	go func() {
		defer close(outC)
		if target != "" {
			for _, line := range utils.ParseStringToLines(target) {
				outC <- line
			}
		}
		if len(files) > 0 {
			fr, err := mfreader.NewMultiFileLineReader(files...)
			if err != nil {
				log.Errorf("failed to read file: %v", err)
				return
			}
			for fr.Next() {
				line := fr.Text()
				if err != nil {
					break
				}
				outC <- line
			}
		}
	}()

	outTarget := make(chan *HybridScanTarget)
	go func() {
		defer func() {
			close(outTarget)
		}()
		for target := range outC {
			for _, template := range templates {
				https, hostname := utils.ParseStringToHttpsAndHostname(target)
				if hostname == "" {
					log.Infof("skip invalid target: %v", target)
					continue
				}
				outTarget <- &HybridScanTarget{
					IsHttps: https,
					Request: bytes.ReplaceAll(template.GetHTTPRequest(), []byte("{{Hostname}}"), []byte(hostname)),
				}
			}
		}
	}()
	return outTarget, nil
}
