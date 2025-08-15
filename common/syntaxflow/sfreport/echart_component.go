package sfreport

// EChartsOption ECharts图表配置结构体
type EChartsOption struct {
	Type   string      `json:"type"`   // 图表类型标识
	Name   string      `json:"name"`   // 图表名称
	Option interface{} `json:"option"` // ECharts原生配置选项
}

// EChartsStackedBarOption 堆叠柱状图ECharts配置
type EChartsStackedBarOption struct {
	Title   echartsTitle    `json:"title"`
	Tooltip echartsTooltip  `json:"tooltip"`
	Legend  echartsLegend   `json:"legend"`
	Grid    echartsGrid     `json:"grid"`
	XAxis   echartsXAxis    `json:"xAxis"`
	YAxis   echartsYAxis    `json:"yAxis"`
	Series  []echartsSeries `json:"series"`
}

// EChartsNightingaleOption 南丁格尔玫瑰图ECharts配置
type EChartsNightingaleOption struct {
	Title   echartsTitle               `json:"title"`
	Tooltip echartsTooltip             `json:"tooltip"`
	Legend  echartsLegend              `json:"legend"`
	Series  []echartsNightingaleSeries `json:"series"`
}

// NewEChartsBarOption 创建堆叠柱状图ECharts配置
func NewEChartsBarOption(option EChartsStackedBarOption) *EChartsOption {
	return &EChartsOption{
		Type:   "e-chart",
		Name:   "bar-graph",
		Option: option,
	}
}

// NewEChartsNightingaleOption 创建南丁格尔玫瑰图ECharts配置
func NewEChartsNightingaleOption(option EChartsNightingaleOption) *EChartsOption {
	return &EChartsOption{
		Type:   "e-chart",
		Name:   "nightingale-rose",
		Option: option,
	}
}

type echartsNightingaleSeries struct {
	Name            string                      `json:"name"`
	Type            string                      `json:"type"`
	Radius          []string                    `json:"radius"`
	Center          []string                    `json:"center"`
	RoseType        string                      `json:"roseType"`
	ItemStyle       echartsNightingaleItemStyle `json:"itemStyle"`
	Label           echartsNightingaleLabel     `json:"label"`
	LabelLine       echartsLabelLine            `json:"labelLine"`
	Data            []echartsNightingaleData    `json:"data"`
	Emphasis        echartsEmphasis             `json:"emphasis"`
	AnimationType   string                      `json:"animationType"`
	AnimationEasing string                      `json:"animationEasing"`
	AnimationDelay  string                      `json:"animationDelay"`
}

type echartsNightingaleItemStyle struct {
	BorderRadius int `json:"borderRadius"`
}

type echartsNightingaleLabel struct {
	Show     bool   `json:"show"`
	Position string `json:"position"`
	//Formatter  string `json:"formatter"`
	FontSize   int    `json:"fontSize"`
	FontWeight string `json:"fontWeight"`
	Color      string `json:"color"`
}

type echartsLabelLine struct {
	Show    bool `json:"show"`
	Length  int  `json:"length"`
	Length2 int  `json:"length2"`
	Smooth  bool `json:"smooth"`
}

type echartsNightingaleData struct {
	Value       int              `json:"value"`
	Name        string           `json:"name"`
	ItemStyle   echartsItemStyle `json:"itemStyle"`
	RealValue   int              `json:"realValue,omitempty"`   // 真实数据值，用于tooltip显示
	RealPercent float64          `json:"realPercent,omitempty"` // 真实百分比，用于formatter显示
}

type echartsEmphasis struct {
	ItemStyle echartsEmphasisItemStyle `json:"itemStyle"`
}

type echartsEmphasisItemStyle struct {
	ShadowBlur    int    `json:"shadowBlur"`
	ShadowOffsetX int    `json:"shadowOffsetX"`
	ShadowColor   string `json:"shadowColor"`
}

type echartsTitle struct {
	Text string `json:"text"`
	Left string `json:"left"`
}

type echartsTooltip struct {
	Trigger     string             `json:"trigger"`
	AxisPointer echartsAxisPointer `json:"axisPointer"`
	//Formatter   string             `json:"formatter,omitempty"`
}

type echartsAxisPointer struct {
	Type string `json:"type"`
}

type echartsLegend struct {
	Data       []string `json:"data"`
	Top        int      `json:"top"`
	ItemWidth  int      `json:"itemWidth"`
	ItemHeight int      `json:"itemHeight"`
}

type echartsGrid struct {
	Left         string `json:"left"`
	Right        string `json:"right"`
	Bottom       string `json:"bottom"`
	Top          string `json:"top"`
	ContainLabel bool   `json:"containLabel"`
}

type echartsXAxis struct {
	Type      string           `json:"type"`
	Data      []string         `json:"data"`
	AxisLabel echartsAxisLabel `json:"axisLabel"`
}

type echartsYAxis struct {
	Type      string           `json:"type"`
	Max       float64          `json:"max,omitempty"`
	AxisLabel echartsAxisLabel `json:"axisLabel"`
	SplitLine echartsSplitLine `json:"splitLine"`
}

type echartsAxisLabel struct {
	FontSize  int    `json:"fontSize,omitempty"`
	Formatter string `json:"formatter,omitempty"`
}

type echartsSplitLine struct {
	Show      bool             `json:"show"`
	LineStyle echartsLineStyle `json:"lineStyle"`
}

type echartsLineStyle struct {
	Color string `json:"color"`
}

type echartsSeries struct {
	Name      string               `json:"name"`
	Type      string               `json:"type"`
	Stack     string               `json:"stack"`
	Data      []echartsBarDataItem `json:"data"`
	ItemStyle echartsItemStyle     `json:"itemStyle"`
	Label     echartsLabel         `json:"label"`
}

type echartsBarDataItem struct {
	Value     float64 `json:"value"`     // 百分比值
	RealTotal int     `json:"realTotal"` // 该风险类型的总数量，用于tooltip计算实际数量
}

type echartsItemStyle struct {
	Color string `json:"color"`
}

type echartsLabel struct {
	Show      bool   `json:"show"`
	Position  string `json:"position"`
	Formatter string `json:"formatter"`
	FontSize  int    `json:"fontSize"`
	Color     string `json:"color"`
}
