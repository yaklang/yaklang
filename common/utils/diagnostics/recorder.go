package diagnostics

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/format"
)

// TrackKind 测量种类，用于日志输出时带标签归类
type TrackKind string

const (
	TrackKindHeap    TrackKind = "Heap" // 堆内存快照
	TrackKindGeneral TrackKind = ""    // 未分类（默认）
)

type Measurement struct {
	Name       string
	Kind       TrackKind // 种类标签，日志输出时带上
	Total      time.Duration
	Min        time.Duration
	Max        time.Duration
	Count      uint64
	ErrorCount uint64
	Steps      []time.Duration
	Size       int64 // 文件大小（字节），0 表示未知，用于计算 ms/KB 比例
	Depth      int   // 层级深度，用于 Build 表格缩进；0 表示未设置，由 inferBuildDepth 推断
}

func (m Measurement) Average() time.Duration {
	if m.Count == 0 {
		return 0
	}
	return m.Total / time.Duration(m.Count)
}

// MsPerKB 返回 ms/KB 比例，无 Size 或 Total 时返回 0
func (m Measurement) MsPerKB() float64 {
	if m.Size <= 0 || m.Total <= 0 {
		return 0
	}
	return float64(m.Total.Milliseconds()) / (float64(m.Size) / 1024)
}

func (m Measurement) String() string {
	var builder strings.Builder
	header := m.Name
	if m.Kind != TrackKindGeneral {
		header = fmt.Sprintf("[%s] %s", m.Kind, m.Name)
	}
	builder.WriteString(fmt.Sprintf("----------- Measurement [%s] --------------------\n", header))
	builder.WriteString(fmt.Sprintf("-------- Measurement %s\tCount %v\n", header, m.Count))
	if m.Count == 0 {
		return builder.String()
	}

	builder.WriteString(fmt.Sprintf("%s--all\tTime: %v\tCount: %v\tAvg: %v\n",
		header, m.Total, m.Count, m.Average(),
	))
	builder.WriteString(fmt.Sprintf("%s--range\tMin: %v\tMax: %v\n",
		header, m.Min, m.Max,
	))

	if m.Count > 1 {
		for index, t := range m.Steps {
			stepAvg := t / time.Duration(m.Count)
			builder.WriteString(fmt.Sprintf("%s-%-4d\tTime: %v\tCount: %v\tAvg: %v\n",
				header, index+1, t, m.Count, stepAvg,
			))
		}
	}
	return builder.String()
}

type measurementData struct {
	mu          sync.Mutex
	stepCap     uint32
	measurement Measurement
}

func newMeasurementData(name string, kind TrackKind, stepCapacity int) *measurementData {
	steps := make([]time.Duration, stepCapacity)
	return &measurementData{
		stepCap: uint32(stepCapacity),
		measurement: Measurement{
			Name:       name,
			Kind:       kind,
			Steps:      steps,
			Total:      0,
			Min:        0,
			Max:        0,
			Count:      0,
			ErrorCount: 0,
		},
	}
}

func (m *measurementData) ensureStepCapacity(count int) {
	if count <= 0 {
		return
	}
	if count <= int(atomic.LoadUint32(&m.stepCap)) {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if count <= len(m.measurement.Steps) {
		atomic.StoreUint32(&m.stepCap, uint32(len(m.measurement.Steps)))
		return
	}
	newSteps := make([]time.Duration, count)
	copy(newSteps, m.measurement.Steps)
	m.measurement.Steps = newSteps
	atomic.StoreUint32(&m.stepCap, uint32(count))
}

// record 统一记录耗时和可选 size（size=0 表示不设置）；depth>=0 时设置 Depth，合并时取 min 保留最浅层级
func (m *measurementData) record(total time.Duration, stepDurations []time.Duration, size int64, depth int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(stepDurations) > len(m.measurement.Steps) {
		newSteps := make([]time.Duration, len(stepDurations))
		copy(newSteps, m.measurement.Steps)
		m.measurement.Steps = newSteps
		atomic.StoreUint32(&m.stepCap, uint32(len(newSteps)))
	}
	for i, dur := range stepDurations {
		m.measurement.Steps[i] += dur
	}

	if m.measurement.Count == 0 {
		m.measurement.Min = total
		m.measurement.Max = total
	} else {
		if total < m.measurement.Min {
			m.measurement.Min = total
		}
		if total > m.measurement.Max {
			m.measurement.Max = total
		}
	}
	m.measurement.Total += total
	m.measurement.Count++
	if size > 0 {
		m.measurement.Size = size
	}
	if depth >= 0 {
		if m.measurement.Count == 1 {
			m.measurement.Depth = depth
		} else if depth < m.measurement.Depth {
			m.measurement.Depth = depth
		}
	}
}

func (m *measurementData) addSizeOnly(size int64) {
	if size <= 0 {
		return
	}
	m.mu.Lock()
	m.measurement.Size = size
	m.mu.Unlock()
}

func (m *measurementData) markError() {
	m.mu.Lock()
	m.measurement.ErrorCount++
	m.mu.Unlock()
}

func (m *measurementData) snapshot() Measurement {
	m.mu.Lock()
	defer m.mu.Unlock()

	steps := make([]time.Duration, len(m.measurement.Steps))
	copy(steps, m.measurement.Steps)
	return Measurement{
		Name:       m.measurement.Name,
		Kind:       m.measurement.Kind,
		Total:      m.measurement.Total,
		Min:        m.measurement.Min,
		Max:        m.measurement.Max,
		Count:      m.measurement.Count,
		ErrorCount: m.measurement.ErrorCount,
		Steps:      steps,
		Size:       m.measurement.Size,
		Depth:      m.measurement.Depth,
	}
}

type Recorder struct {
	entries      *utils.SafeMap[*measurementData]
	buildDepthMu sync.Mutex
	buildDepth   int
}

// KindRecorder 绑定 kind 的 Recorder 包装，Track 系列方法不再需要传 kind；支持 AST/Build 并发场景
type KindRecorder struct {
	rec  *Recorder
	kind TrackKind
}

// ForKind 返回绑定指定 kind 的 KindRecorder；调用方持有一份，可并发使用
func (r *Recorder) ForKind(kind TrackKind) *KindRecorder {
	if r == nil {
		return &KindRecorder{rec: nil, kind: kind}
	}
	return &KindRecorder{rec: r, kind: kind}
}

func NewRecorder() *Recorder {
	return &Recorder{entries: utils.NewSafeMap[*measurementData]()}
}

func (r *Recorder) ensureEntry(name string, kind TrackKind, stepCount int) (*measurementData, error) {
	if name == "" {
		return nil, errors.New("diagnostics: measurement name is empty")
	}
	if r == nil {
		return nil, nil
	}
	entry, err := r.entries.GetOrLoad(name, func() (*measurementData, error) {
		return newMeasurementData(name, kind, stepCount), nil
	})
	if err != nil {
		return nil, err
	}
	entry.ensureStepCapacity(stepCount)
	return entry, nil
}

// trackWithDuration 始终执行 steps；等级满足时记录耗时；depth>=0 时写入 Measurement.Depth，depth<0 时从 PushBuildDepth/PopBuildDepth 栈自动获取
func (r *Recorder) trackWithDuration(enabled bool, doLog bool, kind TrackKind, name string, depth int, steps ...func() error) (time.Duration, error) {
	if name == "" {
		return 0, errors.New("diagnostics: measurement name is empty")
	}
	if depth < 0 && r != nil {
		depth = r.GetCurrentBuildDepth()
	}
	var total time.Duration
	durations := make([]time.Duration, len(steps))

	// 始终执行闭包，不受 enabled 影响
	for i, step := range steps {
		if step == nil {
			continue
		}
		start := time.Now()
		if err := step(); err != nil {
			if enabled && r != nil {
				if entry, _ := r.ensureEntry(name, kind, len(steps)); entry != nil {
					entry.markError()
				}
			}
			return total, err
		}
		elapsed := time.Since(start)
		durations[i] = elapsed
		total += elapsed
	}

	// 仅当等级满足时记录计时
	if enabled && r != nil {
		entry, err := r.ensureEntry(name, kind, len(steps))
		if err != nil {
			return total, err
		}
		if entry != nil {
			entry.record(total, durations, 0, depth)
		}
	}

	if doLog && total > 0 {
		LogLow(kind, "", fmt.Sprintf("%s %v", name, total))
	}
	return total, nil
}

// PushBuildDepth 进入 LazyBuild 时调用，返回当前深度并自增；与 PopBuildDepth 配对
func (r *Recorder) PushBuildDepth() int {
	if r == nil {
		return 0
	}
	r.buildDepthMu.Lock()
	d := r.buildDepth
	r.buildDepth++
	r.buildDepthMu.Unlock()
	return d
}

// PopBuildDepth 退出 LazyBuild 时调用，与 PushBuildDepth 配对
func (r *Recorder) PopBuildDepth() {
	if r == nil {
		return
	}
	r.buildDepthMu.Lock()
	if r.buildDepth > 0 {
		r.buildDepth--
	}
	r.buildDepthMu.Unlock()
}

// GetCurrentBuildDepth 返回当前 build 深度（Push 层数 - 1），供 Track 在 depth<0 时自动获取；无 Push 时为 0
func (r *Recorder) GetCurrentBuildDepth() int {
	if r == nil {
		return 0
	}
	r.buildDepthMu.Lock()
	d := r.buildDepth
	r.buildDepthMu.Unlock()
	if d > 0 {
		return d - 1
	}
	return 0
}

// AddSizeToEntry 闭包内调用，设置 entry 的 Size 用于 ms/KB 计算
func (r *Recorder) AddSizeToEntry(name string, size int64) {
	if r == nil || name == "" || size <= 0 {
		return
	}
	entry, err := r.ensureEntry(name, TrackKindGeneral, 1)
	if err != nil || entry == nil {
		return
	}
	entry.addSizeOnly(size)
}

// RunStepsWithoutRecording 执行 steps 但不记录，用于 rec 为 nil 时的降级
func RunStepsWithoutRecording(steps []func() error) error {
	for _, step := range steps {
		if step == nil {
			continue
		}
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

// SortMeasurementsByMsPerKB 按 ms/KB 降序原地排序，供外部展示使用
func SortMeasurementsByMsPerKB(ms []Measurement) {
	slices.SortFunc(ms, CompareMeasurementByMsPerKB)
}

// DepthInferrer 根据 measurement 名称推断层级，供 SortMeasurementsByDepthAndTotal、MeasurementsToRows 使用；nil 时默认 0
type DepthInferrer func(name string) int

// SortMeasurementsByDepthAndTotal 按 depth 升序、Total 降序排序；depthFn 为 nil 时 depth 均为 0
func SortMeasurementsByDepthAndTotal(ms []Measurement, depthFn DepthInferrer) {
	slices.SortFunc(ms, func(a, b Measurement) int {
		da, db := a.Depth, b.Depth
		if da <= 0 && depthFn != nil {
			da = depthFn(a.Name)
		}
		if db <= 0 && depthFn != nil {
			db = depthFn(b.Name)
		}
		if da != db {
			return da - db
		}
		if a.Total > b.Total {
			return -1
		}
		if a.Total < b.Total {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
}

// CompareMeasurementByMsPerKB 用于 slices.SortFunc：ms/KB 降序，相同则 Total 降序，再按 Name
func CompareMeasurementByMsPerKB(a, b Measurement) int {
	ra, rb := a.MsPerKB(), b.MsPerKB()
	switch {
	case ra > rb:
		return -1
	case ra < rb:
		return 1
	}
	if a.Total != b.Total {
		if a.Total > b.Total {
			return -1
		}
		return 1
	}
	return strings.Compare(a.Name, b.Name)
}

func (r *Recorder) Snapshot() []Measurement {
	if r == nil {
		return nil
	}
	values := r.entries.Values()
	result := make([]Measurement, 0, len(values))
	for _, entry := range values {
		result = append(result, entry.snapshot())
	}
	slices.SortFunc(result, CompareMeasurementByMsPerKB)
	return result
}

func (r *Recorder) Reset() {
	if r == nil {
		return
	}
	r.entries = utils.NewSafeMap[*measurementData]()
}

// titleFromLabel 从 label 取标题，为空则默认 "Measurement Summary"
func titleFromLabel(label ...string) string {
	if len(label) > 0 && label[0] != "" {
		return label[0]
	}
	return "Measurement Summary"
}

// Log 仅打印数据（格式化表格），LevelNormal。内部不调用 LogTable。调用方需确保 rec 非 nil 且有数据。
func (rec *Recorder) Log(label ...string) {
	if rec == nil {
		return
	}
	snapshots := rec.Snapshot()
	if len(snapshots) == 0 {
		return
	}
	headers, rows := MeasurementsToRows(snapshots)
	if len(rows) == 0 {
		return
	}
	payload := &TablePayload{Title: titleFromLabel(label...), Headers: headers, Rows: rows}
	content := payload.Format()
	if content != "" {
		Log(TrackKindGeneral, "", content, false)
	}
}

// LogHigh recorder 为 nil 时输出异常（LevelHigh）；rec 非 nil 时 no-op
func (rec *Recorder) LogHigh(label ...string) {
	if rec != nil {
		return
	}
	LogHigh(TrackKindGeneral, titleFromLabel(label...), "recorder is nil")
}

// LogLow 无数据时简单提示（LevelLow）
func (rec *Recorder) LogLow(label ...string) {
	if rec == nil {
		return
	}
	LogLow(TrackKindGeneral, titleFromLabel(label...), "No performance data")
}

// FilterByTrackKind 筛选指定 Kind 的 Measurement
func FilterByTrackKind(snap []Measurement, kind TrackKind) []Measurement {
	out := make([]Measurement, 0, len(snap))
	for _, m := range snap {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out
}

// MeasurementsToDurMap 从 Measurement 建 ID→duration 表，供 TreePayload 使用；Total==0 的不入表
func MeasurementsToDurMap(ms []Measurement) map[string]time.Duration {
	m := make(map[string]time.Duration, len(ms))
	for _, x := range ms {
		if x.Name != "" && x.Total > 0 {
			m[x.Name] = x.Total
		}
	}
	return m
}

func CompareRecorderCosts(database, memory *Recorder) {
	if database == nil {
		return
	}
	databaseSnapshots := database.Snapshot()
	memorySnapshots := memory.Snapshot()
	memoryIndex := make(map[string]Measurement, len(memorySnapshots))
	for _, snapshot := range memorySnapshots {
		memoryIndex[snapshot.Name] = snapshot
	}

	for _, databaseMeasurement := range databaseSnapshots {
		memoryMeasurement, ok := memoryIndex[databaseMeasurement.Name]
		if !ok {
			log.Debugf("Measurement [%s] not found in memory cost", databaseMeasurement.Name)
			log.Debug(databaseMeasurement.String())
			continue
		}

		if memoryMeasurement.Count == 0 {
			memoryMeasurement.Count = 1
		}
		if databaseMeasurement.Count > memoryMeasurement.Count*5 {
			log.Debugf("Measurement [%s] count mismatch: database %d, memory %d", databaseMeasurement.Name, databaseMeasurement.Count, memoryMeasurement.Count)
			log.Debug(databaseMeasurement.String())
			log.Debug(memoryMeasurement.String())
		}

		if databaseMeasurement.Total > memoryMeasurement.Total*2 {
			log.Debugf("------------------------------------------------------")
			log.Debugf("Measurement [%s] total time mismatch: database %v, memory %v", databaseMeasurement.Name, databaseMeasurement.Total, memoryMeasurement.Total)
			for index, databaseTime := range databaseMeasurement.Steps {
				if index >= len(memoryMeasurement.Steps) {
					log.Debugf("Measurement %s time mismatch at index %d: database %v, memory not found", databaseMeasurement.Name, index, databaseTime)
					log.Debugf("%s-%-4d\t database Time: %v\tCount: %v\tAvg: %v",
						databaseMeasurement.Name, index+1,
						databaseTime,
						databaseMeasurement.Count,
						databaseTime/time.Duration(databaseMeasurement.Count),
					)
					continue
				}

				memoryTime := memoryMeasurement.Steps[index]
				if databaseTime > memoryTime*2 || databaseTime > time.Second {
					log.Debugf("Measurement %s time mismatch at index %d: database %v, memory %v", databaseMeasurement.Name, index, databaseTime, memoryTime)
					log.Debugf("%s-%-4d\t database Time: %v\tCount: %v\tAvg: %v",
						databaseMeasurement.Name, index+1,
						databaseTime,
						databaseMeasurement.Count,
						databaseTime/time.Duration(databaseMeasurement.Count),
					)
					log.Debugf("%s-%-4d\t memory  Time: %v\tCount: %v\tAvg: %v",
						databaseMeasurement.Name, index+1,
						memoryTime,
						memoryMeasurement.Count,
						memoryTime/time.Duration(memoryMeasurement.Count),
					)
				}
			}
		}
	}
}

// TableOption 制表 API 统一配置：FormatTable 样式、MeasurementsToRows 列、MapToRows 表头等
type TableOption func(*tableConfig)

type tableConfig struct {
	cellMaxWidth   int           // FormatTable：单元格最大宽度，0 表示默认 80
	includeSize    bool          // MeasurementsToRows：强制包含 Size/ms/KB 列
	includeCount   bool          // MeasurementsToRows：强制包含 Count 列
	indentByDepth  bool          // MeasurementsToRows：按 depth 缩进 Name 列
	mapHeaders     [2]string     // MapToRows：两列表头，默认 ["Name","Value"]
	leftAlignCols  []int         // FormatTable：左对齐的列下标
	depthAsTree    bool          // MeasurementsToRows：Depth 列用 |-- 模拟树
	depthInferrer  DepthInferrer // MeasurementsToRows：depth 未设置时推断；nil 时用 0
}

// TableCellMaxWidth 设置单元格最大显示宽度
func TableCellMaxWidth(n int) TableOption {
	return func(c *tableConfig) { c.cellMaxWidth = n }
}

// TableIncludeSize 强制 MeasurementsToRows 包含 Size、ms/KB 列
func TableIncludeSize(include bool) TableOption {
	return func(c *tableConfig) { c.includeSize = include }
}

// TableIncludeCount 强制 MeasurementsToRows 包含 Count 列。用于 Database 等并发 batch 场景：Duration 为各 batch 耗时的累加，可能远超墙钟时间。
func TableIncludeCount(include bool) TableOption {
	return func(c *tableConfig) { c.includeCount = include }
}

// TableIndentByDepth 按 depth 缩进 Name 列，用于 Build Performance 层级展示（Build[path]=0，compile /path=1）
func TableIndentByDepth(enable bool) TableOption {
	return func(c *tableConfig) { c.indentByDepth = enable }
}

// TableBuildStyle 表格展示：Name 左对齐、Depth 用 |-- 模拟树
func TableBuildStyle(enable bool) TableOption {
	return func(c *tableConfig) {
		if enable {
			c.depthAsTree = true
			c.leftAlignCols = []int{0, 1}
		}
	}
}

// TableDepthInferrer 设置 depth 推断函数，用于 indentByDepth 时 Measurement.Depth<=0 的 measurement
func TableDepthInferrer(fn DepthInferrer) TableOption {
	return func(c *tableConfig) { c.depthInferrer = fn }
}

// TableMapHeaders 指定 MapToRows 的表头列名
func TableMapHeaders(col1, col2 string) TableOption {
	return func(c *tableConfig) { c.mapHeaders = [2]string{col1, col2} }
}

func applyTableOptions(opts []TableOption) tableConfig {
	cfg := tableConfig{cellMaxWidth: 80, mapHeaders: [2]string{"Name", "Value"}}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

func truncateCell(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}
	return runewidth.Truncate(s, maxWidth, "...")
}

func cellDisplayWidth(s string) int {
	return runewidth.StringWidth(s)
}

// formatTable 通用表格：标题 + 表头 + 数据行，统一边框。仅两种输出格式之一。
func formatTable(title string, headers []string, rows [][]string, emptyMsg string, opts ...TableOption) string {
	if len(rows) == 0 {
		return emptyMsg
	}
	cfg := applyTableOptions(opts)
	cols := len(headers)
	widths := make([]int, cols)
	for i, h := range headers {
		if w := cellDisplayWidth(h); w > widths[i] {
			widths[i] = w
		}
	}
	for _, row := range rows {
		for i := 0; i < cols && i < len(row); i++ {
			c := truncateCell(row[i], cfg.cellMaxWidth)
			if w := cellDisplayWidth(c); w > widths[i] {
				widths[i] = w
			}
		}
	}
	// 预留更多空间：每列最小宽度 14；Depth 列仅需一位数字，用 5
	const minColWidth = 14
	const minDepthColWidth = 5
	for i := range widths {
		minW := minColWidth
		if i < len(headers) && headers[i] == "Depth" {
			minW = minDepthColWidth
		}
		if widths[i] < minW {
			widths[i] = minW
		}
	}
	var total int
	for _, w := range widths {
		total += w + 3
	}
	if total < 40 {
		total = 40
	}
	titleBorder := strings.Repeat("=", total)
	var sb strings.Builder
	sb.WriteString("\n" + titleBorder + "\n")
	sb.WriteString(fmt.Sprintf(" %s\n", title))
	sb.WriteString(titleBorder + "\n")
	sep := "+"
	for _, w := range widths {
		sep += strings.Repeat("-", w+2) + "+"
	}
	sb.WriteString(sep + "\n")
	alignLeft := func(col int) bool {
		for _, c := range cfg.leftAlignCols {
			if c == col {
				return true
			}
		}
		return len(cfg.leftAlignCols) == 0 && col == 0 // 默认首列左对齐
	}
	for i := 0; i < cols; i++ {
		h := headers[i]
		pad := runewidth.FillRight(h, widths[i])
		if !alignLeft(i) {
			pad = runewidth.FillLeft(h, widths[i])
		}
		if i == 0 {
			sb.WriteString("| " + pad + " |")
		} else {
			sb.WriteString(" " + pad + " |")
		}
	}
	sb.WriteString("\n")
	sb.WriteString(sep + "\n")
	for _, row := range rows {
		for i := 0; i < cols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			cell = truncateCell(cell, cfg.cellMaxWidth)
			pad := runewidth.FillRight(cell, widths[i])
			if !alignLeft(i) {
				pad = runewidth.FillLeft(cell, widths[i])
			}
			if i == 0 {
				sb.WriteString("| " + pad + " |")
			} else {
				sb.WriteString(" " + pad + " |")
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString(sep + "\n")
	return sb.String()
}

// FormatTable 唯一制表函数：标题 + 表头 + 数据行，统一边框。支持 TableOption 配置样式。
func FormatTable(title string, headers []string, rows [][]string, opts ...TableOption) string {
	if len(rows) == 0 {
		return fmt.Sprintf("No data for: %s", title)
	}
	return formatTable(title, headers, rows, fmt.Sprintf("No data for: %s", title), opts...)
}

// MeasurementsToRows 将 Measurement 转为表头与行。无 Size 数据时仅输出 Name、Duration 列；有 Size 时包含 Size、ms/KB；includeCount 时包含 Count。
func MeasurementsToRows(ms []Measurement, opts ...TableOption) (headers []string, rows [][]string) {
	cfg := applyTableOptions(opts)
	hasSize := cfg.includeSize
	if !hasSize {
		for _, m := range ms {
			if m.Size > 0 {
				hasSize = true
				break
			}
		}
	}
	headers = []string{"Name", "Duration"}
	if cfg.indentByDepth {
		headers = []string{"Depth", "Name", "Duration"}
	}
	if hasSize {
		headers = append(headers, "Size", "ms/KB")
	}
	if cfg.includeCount {
		headers = append(headers, "Count")
	}
	rows = make([][]string, 0, len(ms))
	for _, m := range ms {
		name := m.Name
		depth := m.Depth
		if depth <= 0 && cfg.depthInferrer != nil {
			depth = cfg.depthInferrer(name)
		}
		var row []string
		if cfg.indentByDepth {
			depthStr := fmt.Sprintf("%d", depth)
			displayName := name
			// depthAsTree 且 depth>0 时，在 Name 列加 |-- 模拟树结构
			if cfg.depthAsTree && depth > 0 {
				displayName = strings.Repeat("  ", depth) + "|-- " + name
			}
			row = []string{depthStr, displayName, format.FormatDuration(m.Total)}
		} else {
			row = []string{name, format.FormatDuration(m.Total)}
		}
		if hasSize {
			sizeStr, ratioStr := "-", "-"
			if m.Size > 0 {
				sizeStr = format.FormatSize(m.Size)
				if m.Total > 0 {
					ratioStr = fmt.Sprintf("%.2f", m.MsPerKB())
				}
			}
			row = append(row, sizeStr, ratioStr)
		}
		if cfg.includeCount {
			row = append(row, fmt.Sprintf("%d", m.Count))
		}
		rows = append(rows, row)
	}
	return headers, rows
}

// MapToRows 将 map[string]string 转为表头与行，供 FormatTable 使用。支持 TableOption 配置。
func MapToRows(data map[string]string, opts ...TableOption) (headers []string, rows [][]string) {
	cfg := applyTableOptions(opts)
	headers = []string{cfg.mapHeaders[0], cfg.mapHeaders[1]}
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows = make([][]string, 0, len(keys))
	for _, k := range keys {
		rows = append(rows, []string{k, data[k]})
	}
	return headers, rows
}
