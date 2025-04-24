package aid

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/linktable"
	"github.com/yaklang/yaklang/common/utils/omap"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

type memoryTimeline struct {
	ai             AICaller
	clearCount     int
	m              *sync.Mutex
	Timestamp      []int64
	tsToToolResult *omap.OrderedMap[int64, *aitool.ToolResult]
	idToToolResult *omap.OrderedMap[string, *aitool.ToolResult]

	summaryMutex *sync.RWMutex
	summary      *omap.OrderedMap[string, *linktable.LinkTable[string]]
}

func newMemoryTimeline(clearCount int, ai AICaller) *memoryTimeline {
	return &memoryTimeline{
		ai:             ai,
		clearCount:     clearCount,
		m:              new(sync.Mutex),
		Timestamp:      []int64{},
		tsToToolResult: omap.NewOrderedMap(map[int64]*aitool.ToolResult{}),
		idToToolResult: omap.NewOrderedMap(map[string]*aitool.ToolResult{}),

		summaryMutex: new(sync.RWMutex),
		summary:      omap.NewOrderedMap(map[string]*linktable.LinkTable[string]{}),
	}
}

func (m *memoryTimeline) PushToolResult(toolResult *aitool.ToolResult) {
	ts := time.Now().UnixMilli()
	m.m.Lock()
	defer m.m.Unlock()

	if m.tsToToolResult.Have(ts) {
		time.Sleep(time.Millisecond * 100)
		ts = time.Now().UnixMilli()
	}

	m.tsToToolResult.Set(ts, toolResult)
	m.idToToolResult.Set(toolResult.GetID(), toolResult)
	m.Timestamp = append(m.Timestamp, ts)
}

func (m *memoryTimeline) Dump() string {
	m.m.Lock()
	defer m.m.Unlock()

	buf := bytes.NewBuffer(nil)
	initOnce := sync.Once{}
	count := 0
	m.tsToToolResult.ForEach(func(key int64, value *aitool.ToolResult) bool {
		t := time.Unix(0, key*int64(time.Millisecond))
		initOnce.Do(func() {
			buf.WriteString("timeline:\n")
		})
		buf.WriteString(fmt.Sprintf("├─[%s]\n", t.Format(utils.DefaultTimeFormat2)))
		raw := value.String()
		for _, line := range utils.ParseStringToLines(raw) {
			buf.WriteString(fmt.Sprintf("│    %s\n", line))
		}
		count++
		return true
	})
	if count > 0 {
		return buf.String()
	}

	buf.WriteString("no timeline\n")
	return buf.String()
}
