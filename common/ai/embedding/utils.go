package embedding

import (
	"fmt"
	"sort"
	"strings"
)

// QuadrantChartOptions 配置 Quadrant Chart 头部信息（仅输出 quadrant-1）
type QuadrantChartOptions struct {
	// 标题
	Title string
	// X 轴两端说明：Left --> Right
	XLeft  string
	XRight string
	// Y 轴两端说明：Bottom --> Top
	YBottom string
	YTop    string
	// quadrant-1 的标签
	Quadrant1 string
}

func defaultQuadrantChartOptions() QuadrantChartOptions {
	return QuadrantChartOptions{
		Title:     "Reach and engagement of campaigns",
		XLeft:     "Low Reach",
		XRight:    "High Reach",
		YBottom:   "Low Engagement",
		YTop:      "High Engagement",
		Quadrant1: "",
	}
}

// GenerateQuadrantChartCode 生成 Quadrant Chart DSL 文本。
// 入参 data: key 为标签，value 为该标签对应的二维点坐标列表（每个点为 [x, y]）。
// 输出示例：
//
//	quadrantChart
//	    title Reach and engagement of campaigns
//	    x-axis Low Reach --> High Reach
//	    y-axis Low Engagement --> High Engagement
//	    quadrant-1 We should expand
//	    quadrant-2 Need to promote
//	    quadrant-3 Re-evaluate
//	    quadrant-4 May be improved
//	    Campaign A: [0.3, 0.6]
//	    Campaign B: [0.45, 0.23]
//
// 默认使用与示例一致的标题与轴/象限描述。若需要自定义，可在外层对返回字符串再做替换。
func GenerateQuadrantChartCode(data map[string][][]float32) string {
	const indent = "    "

	var b strings.Builder
	b.Grow(256)

	b.WriteString("quadrantChart\n")
	b.WriteString(indent)
	b.WriteString("title Reach and engagement of campaigns\n")
	b.WriteString(indent)
	b.WriteString("x-axis Low Reach --> High Reach\n")
	b.WriteString(indent)
	b.WriteString("y-axis Low Engagement --> High Engagement\n")
	b.WriteString(indent)
	b.WriteString("quadrant-1 We should expand\n")
	b.WriteString(indent)
	b.WriteString("quadrant-2 Need to promote\n")
	b.WriteString(indent)
	b.WriteString("quadrant-3 Re-evaluate\n")
	b.WriteString(indent)
	b.WriteString("quadrant-4 May be improved\n")

	// 稳定输出顺序：对 key 排序
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, label := range keys {
		points := data[label]
		if len(points) == 0 {
			continue
		}
		for _, p := range points {
			if len(p) < 2 {
				continue
			}
			x := trimFloat(float64(p[0]))
			y := trimFloat(float64(p[1]))
			b.WriteString(indent)
			b.WriteString(fmt.Sprintf("%s: [%s, %s]\n", label, x, y))
		}
	}
	return b.String()
}

// GenerateQuadrantChartCodeWithOptions 生成 Quadrant Chart（可配置标题与轴名），且只包含 quadrant-1。
func GenerateQuadrantChartCodeWithOptions(data map[string][][]float32, opts *QuadrantChartOptions) string {
	const indent = "\t\t\t\t" // 统一使用 4 级缩进的 tab 可读性更高

	o := defaultQuadrantChartOptions()
	if opts != nil {
		if opts.Title != "" {
			o.Title = opts.Title
		}
		if opts.XLeft != "" {
			o.XLeft = opts.XLeft
		}
		if opts.XRight != "" {
			o.XRight = opts.XRight
		}
		if opts.YBottom != "" {
			o.YBottom = opts.YBottom
		}
		if opts.YTop != "" {
			o.YTop = opts.YTop
		}
		if opts.Quadrant1 != "" {
			o.Quadrant1 = opts.Quadrant1
		}
	}

	var b strings.Builder
	b.Grow(256)

	b.WriteString("quadrantChart\n")
	b.WriteString(indent)
	b.WriteString("title ")
	b.WriteString(o.Title)
	b.WriteString("\n")
	b.WriteString(indent)
	b.WriteString("x-axis ")
	b.WriteString(o.XLeft)
	b.WriteString(" --> ")
	b.WriteString(o.XRight)
	b.WriteString("\n")
	b.WriteString(indent)
	b.WriteString("y-axis ")
	b.WriteString(o.YBottom)
	b.WriteString(" --> ")
	b.WriteString(o.YTop)
	b.WriteString("\n")
	b.WriteString(indent)
	b.WriteString("quadrant-1 ")
	b.WriteString(o.Quadrant1)
	b.WriteString("\n")

	// 稳定输出顺序：对 key 排序
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, label := range keys {
		points := data[label]
		if len(points) == 0 {
			continue
		}
		for _, p := range points {
			if len(p) < 2 {
				continue
			}
			x := trimFloat(float64(p[0]))
			y := trimFloat(float64(p[1]))
			b.WriteString(indent)
			b.WriteString(fmt.Sprintf("%s: [%s, %s]\n", label, x, y))
		}
	}
	return b.String()
}

// GenerateQuadrantChartCodeFromEmbeddings 接受高维 embedding（每个 label 下可包含多个向量），
// 先通过 PCA 将所有样本整体降到二维，再按 label 输出 Quadrant Chart DSL。
func GenerateQuadrantChartFromEmbeddings(data map[string][][]float32) string {
	// 稳定 key 顺序
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 展平为样本列表，并记录每个 label 的样本数
	counts := make(map[string]int, len(data))
	var all [][]float32
	for _, k := range keys {
		pts := data[k]
		counts[k] = len(pts)
		all = append(all, pts...)
	}

	if len(all) == 0 {
		// 无数据，直接返回带默认头部（仅 quadrant-1）的模板
		return GenerateQuadrantChartCodeWithOptions(map[string][][]float32{}, nil)
	}

	// 降到二维
	coords2D, err := ReduceTo2D(all)
	if err != nil {
		return GenerateQuadrantChartCodeWithOptions(map[string][][]float32{}, nil)
	}

	// 还原为每个 label 的点集
	out2D := make(map[string][][]float32, len(data))
	idx := 0
	for _, k := range keys {
		c := counts[k]
		if c == 0 {
			continue
		}
		slice := make([][]float32, c)
		for i := 0; i < c; i++ {
			slice[i] = coords2D[idx]
			idx++
		}
		out2D[k] = slice
	}

	return GenerateQuadrantChartCodeWithOptions(out2D, nil)
}

// GenerateQuadrantChartFromEmbeddingsWithOptions 与上类似，但允许自定义标题、轴名称以及 quadrant-1 名称。
func GenerateQuadrantChartFromEmbeddingsWithOptions(data map[string][][]float32, opts *QuadrantChartOptions) string {
	// 稳定 key 顺序
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 展平为样本列表，并记录每个 label 的样本数
	counts := make(map[string]int, len(data))
	var all [][]float32
	for _, k := range keys {
		pts := data[k]
		counts[k] = len(pts)
		all = append(all, pts...)
	}

	if len(all) == 0 {
		return GenerateQuadrantChartCodeWithOptions(map[string][][]float32{}, opts)
	}

	coords2D, err := ReduceTo2D(all)
	if err != nil {
		return GenerateQuadrantChartCodeWithOptions(map[string][][]float32{}, opts)
	}

	out2D := make(map[string][][]float32, len(data))
	idx := 0
	for _, k := range keys {
		c := counts[k]
		if c == 0 {
			continue
		}
		slice := make([][]float32, c)
		for i := 0; i < c; i++ {
			slice[i] = coords2D[idx]
			idx++
		}
		out2D[k] = slice
	}
	return GenerateQuadrantChartCodeWithOptions(out2D, opts)
}

func trimFloat(v float64) string {
	s := fmt.Sprintf("%.6f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}
