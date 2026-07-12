package aistats

import (
	"sync"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

const (
	// DefaultUserKey 是无法解析出 PersistentSessionId 时的回退 user 标识.
	DefaultUserKey = "default"

	// 有界队列 + worker 配置. 队列满直接丢弃以保证非阻塞.
	statsQueueSize = 2048
	statsWorkerNum = 2
)

// statsJob 是一次统计写入任务 (有向 union, 各字段对不同任务类型有意义).
type statsJob struct {
	kind    statsJobKind
	userKey string
	day     string
	entity  string // entityType
	name    string // entityName / actionType / model
	source  string // hit source (for tool/skill)
	usage   *aispec.ChatUsage
}

type statsJobKind uint8

const (
	statsJobToolHit statsJobKind = iota
	statsJobSkillHit
	statsJobAction
	statsJobAICall
)

// statsPool 是进程级的有界队列 + 固定 worker pool (与 aive valueFeedbackPool 同构).
type statsPool struct {
	queue chan *statsJob
	once  sync.Once
}

var globalStatsPool = &statsPool{
	queue: make(chan *statsJob, statsQueueSize),
}

func (p *statsPool) start() {
	p.once.Do(func() {
		for i := 0; i < statsWorkerNum; i++ {
			go p.worker(i)
		}
		log.Infof("aistats pool started: workers=%d queue=%d", statsWorkerNum, statsQueueSize)
	})
}

func (p *statsPool) worker(id int) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("aistats worker[%d] panic recovered, restarting: %v", id, r)
			go p.worker(id)
		}
	}()
	for job := range p.queue {
		p.process(job)
	}
}

func (p *statsPool) process(job *statsJob) {
	defer func() {
		if r := recover(); r != nil {
			log.Warnf("aistats process panic recovered: %v", r)
		}
	}()
	if job == nil {
		return
	}
	db := consts.GetGormProfileDatabase()
	if db == nil {
		return
	}

	switch job.kind {
	case statsJobToolHit:
		if err := yakit.IncrementEntityHit(db, schema.AIStatsEntityTypeTool, job.name, job.source); err != nil {
			log.Debugf("aistats tool hit (%s/%s) failed: %v", job.name, job.source, err)
		}
		// 工具命中也累计每日 tool_calls_total.
		_ = yakit.IncrementDailyStats(db, job.userKey, job.day, map[string]interface{}{"tool_calls_total": 1})

	case statsJobSkillHit:
		if err := yakit.IncrementEntityHit(db, schema.AIStatsEntityTypeSkill, job.name, job.source); err != nil {
			log.Debugf("aistats skill hit (%s/%s) failed: %v", job.name, job.source, err)
		}
		// SKILL 命中累计每日 skills_loaded (无论来源).
		_ = yakit.IncrementDailyStats(db, job.userKey, job.day, map[string]interface{}{"skills_loaded": 1})

	case statsJobAction:
		_ = yakit.IncrementDailyStats(db, job.userKey, job.day, map[string]interface{}{"actions": 1})

	case statsJobAICall:
		inc := map[string]interface{}{"ai_calls": 1}
		if job.usage != nil {
			inc["tokens_input"] = int64(job.usage.PromptTokens)
			inc["tokens_output"] = int64(job.usage.CompletionTokens)
		}
		_ = yakit.IncrementDailyStats(db, job.userKey, job.day, inc)
	}
}

// tryEnqueue 非阻塞投递. 队列满直接丢弃 + 告警, 绝不阻塞调用方.
func (p *statsPool) tryEnqueue(job *statsJob) {
	p.start()
	select {
	case p.queue <- job:
	default:
		log.Warnf("aistats queue full, drop job: kind=%d name=%s", job.kind, job.name)
	}
}

// recorder 是注册进 aicommon 的 StatsRecorder 实现 (非阻塞投递).
type recorder struct{}

// RecordToolHit 实现 aicommon.StatsRecorder.
func (recorder) RecordToolHit(cfg *aicommon.Config, toolName, source string) {
	globalStatsPool.tryEnqueue(&statsJob{
		kind:    statsJobToolHit,
		userKey: resolveUserKey(cfg),
		day:     today(),
		name:    toolName,
		source:  source,
	})
}

// RecordSkillHit 实现 aicommon.StatsRecorder.
func (recorder) RecordSkillHit(cfg *aicommon.Config, skillName, source string) {
	globalStatsPool.tryEnqueue(&statsJob{
		kind:    statsJobSkillHit,
		userKey: resolveUserKey(cfg),
		day:     today(),
		name:    skillName,
		source:  source,
	})
}

// RecordAction 实现 aicommon.StatsRecorder.
func (recorder) RecordAction(cfg *aicommon.Config, actionType string) {
	globalStatsPool.tryEnqueue(&statsJob{
		kind:    statsJobAction,
		userKey: resolveUserKey(cfg),
		day:     today(),
		name:    actionType,
	})
}

// RecordAICall 实现 aicommon.StatsRecorder.
func (recorder) RecordAICall(cfg *aicommon.Config, model string, usage *aispec.ChatUsage) {
	globalStatsPool.tryEnqueue(&statsJob{
		kind:    statsJobAICall,
		userKey: resolveUserKey(cfg),
		day:     today(),
		name:    model,
		usage:   usage,
	})
}
