package syntaxflow_scan

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/log"
)

type instructionStat struct {
	name  string
	total time.Duration
	count int64
}

type instructionProfiler struct {
	ruleName    string
	programName string

	lastTime  time.Time
	lastLabel string

	stats map[string]*instructionStat

	summary *instructionSummary
}

func newInstructionProfiler(ruleName, programName string, summary *instructionSummary) *instructionProfiler {
	return &instructionProfiler{
		ruleName:    ruleName,
		programName: programName,
		stats:       make(map[string]*instructionStat),
		summary:     summary,
	}
}

func (p *instructionProfiler) Observe(label string) {
	p.addDuration(time.Now())
	p.lastLabel = label
	p.lastTime = time.Now()
}

func (p *instructionProfiler) Finish() {
	p.addDuration(time.Now())
	p.lastLabel = ""
	p.lastTime = time.Time{}
	if p.summary != nil {
		p.summary.Merge(p.stats)
	}
}

func (p *instructionProfiler) addDuration(now time.Time) {
	if p.lastLabel == "" || p.lastTime.IsZero() {
		return
	}
	dur := now.Sub(p.lastTime)
	if dur <= 0 {
		return
	}

	key := classifyInstructionLabel(p.lastLabel)
	if key == "" {
		return
	}

	stat := p.stats[key]
	if stat == nil {
		stat = &instructionStat{name: key}
		p.stats[key] = stat
	}
	stat.total += dur
	stat.count++
}

func (p *instructionProfiler) LogSummary() {
	if len(p.stats) == 0 {
		return
	}
	entries := make([]*instructionStat, 0, len(p.stats))
	for _, stat := range p.stats {
		entries = append(entries, stat)
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].total > entries[j].total
	})

	log.Infof("----------- SyntaxFlow Instruction Performance Rule[%s].Prog[%s] -----------", p.ruleName, p.programName)

	const maxShow = 10
	show := maxShow
	if len(entries) < maxShow {
		show = len(entries)
	}
	for i := 0; i < show; i++ {
		stat := entries[i]
		avg := time.Duration(0)
		if stat.count > 0 {
			avg = stat.total / time.Duration(stat.count)
		}
		log.Infof("Instr[%s] Time: %v Count: %d Avg: %v", stat.name, stat.total, stat.count, avg)
	}
	log.Infof("---------------------------------------------------------------------------")
}

func classifyInstructionLabel(label string) string {
	label = strings.TrimSpace(label)
	if label == "" {
		return ""
	}
	lower := strings.ToLower(label)

	switch {
	case strings.Contains(lower, "iter"):
		return "Iterator"
	case strings.Contains(lower, "getcallargs"):
		return "GetCallArgs"
	case strings.Contains(lower, "getcall"):
		return "GetCall"
	case strings.Contains(lower, "users"):
		return "Users"
	case strings.Contains(lower, "defs"):
		return "Defs"
	case strings.Contains(lower, "native"):
		return "NativeCall"
	case strings.Contains(lower, "alert"):
		return "Alert"
	case strings.Contains(lower, "desc"):
		return "Description"
	case strings.Contains(lower, "compare"):
		return "Compare"
	case strings.Contains(lower, "condition"):
		return "Condition"
	case strings.Contains(lower, "check"):
		return "Check"
	case strings.Contains(lower, "push"):
		return "Push"
	case strings.Contains(lower, "pop"):
		return "Pop"
	case strings.Contains(lower, "save result"):
		return "SaveResult"
	case strings.Contains(lower, "load or compile syntaxflow rule"):
		return "LoadRule"
	case strings.Contains(lower, "start query syntaxflow"):
		return "StartQuery"
	case strings.Contains(lower, "end query syntaxflow"):
		return ""
	default:
		fields := strings.Fields(label)
		if len(fields) > 0 {
			return fields[0]
		}
	}
	return "Other"
}

type instructionSummary struct {
	mu    sync.Mutex
	stats map[string]*instructionStat
}

func newInstructionSummary() *instructionSummary {
	return &instructionSummary{
		stats: make(map[string]*instructionStat),
	}
}

func (s *instructionSummary) Merge(stats map[string]*instructionStat) {
	if s == nil || len(stats) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stats == nil {
		s.stats = make(map[string]*instructionStat)
	}
	for key, stat := range stats {
		if stat == nil {
			continue
		}
		item := s.stats[key]
		if item == nil {
			item = &instructionStat{name: stat.name}
			s.stats[key] = item
		}
		item.total += stat.total
		item.count += stat.count
	}
}

func (s *instructionSummary) snapshot() []*instructionStat {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]*instructionStat, 0, len(s.stats))
	for _, stat := range s.stats {
		result = append(result, &instructionStat{
			name:  stat.name,
			total: stat.total,
			count: stat.count,
		})
	}
	return result
}

var importantInstructionLabels = []string{
	"LoadRule",
	"StartQuery",
	"Iterator",
	"GetCallArgs",
	"GetCall",
	"Users",
	"Defs",
	"NativeCall",
	"Condition",
	"Compare",
	"Push",
	"Pop",
	"SaveResult",
	"Alert",
}

func (s *instructionSummary) LogGlobal() {
	entries := s.snapshot()
	if len(entries) == 0 {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].total > entries[j].total
	})

	log.Infof("=========== SyntaxFlow Instruction Performance (Global) ===========")
	index := make(map[string]*instructionStat, len(entries))
	for _, stat := range entries {
		index[stat.name] = stat
	}

	highlighted := map[string]struct{}{}
	for _, label := range importantInstructionLabels {
		if stat, ok := index[label]; ok {
			log.Infof("[关键] Instr[%s] Time: %v Count: %d Avg: %v", stat.name, stat.total, stat.count, averageDuration(stat))
			highlighted[label] = struct{}{}
		}
	}

	const maxTop = 15
	topPrinted := 0
	for _, stat := range entries {
		if _, skip := highlighted[stat.name]; skip {
			continue
		}
		log.Infof("Instr[%s] Time: %v Count: %d Avg: %v", stat.name, stat.total, stat.count, averageDuration(stat))
		topPrinted++
		if topPrinted >= maxTop {
			break
		}
	}
	log.Infof("===================================================================")
}

func averageDuration(stat *instructionStat) time.Duration {
	if stat == nil || stat.count == 0 {
		return 0
	}
	return stat.total / time.Duration(stat.count)
}
