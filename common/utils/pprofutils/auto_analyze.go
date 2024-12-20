package pprofutils

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/google/pprof/profile"
	"github.com/yaklang/yaklang/common/utils"
)

type FunctionStat struct {
	Name         string
	FileLocation string
	Value        int64
	Calls        int64
	Percent      float64
	SampleType   string // CPU或Memory类型
}

func (f *FunctionStat) Dump() string {
	var valueStr string
	if f.SampleType == "CPU" {
		valueStr = fmt.Sprintf("Time Cost: %v", time.Duration(f.Value)*time.Millisecond)
	} else {
		valueStr = fmt.Sprintf("Memory Usage: %v", utils.ByteSize(uint64(f.Value)))
	}

	return fmt.Sprintf(
		"[%s Analysis Result]\n"+
			"  Function Name: %s\n"+
			"  File Location: %s\n"+
			"  %s (Percentage: %.2f%%)\n"+
			"  Call Count: %d\n",
		f.SampleType,
		f.Name,
		f.FileLocation,
		valueStr,
		f.Percent*100,
		f.Calls,
	)
}

func analyzePprof(prof *profile.Profile) []FunctionStat {
	// 确定采样类型的索引
	var typeIndex = -1
	var sampleType string
	var isCPU = false

	for i, st := range prof.SampleType {
		sampleTypeLower := strings.ToLower(st.Type)
		if strings.Contains(sampleTypeLower, "cpu") || strings.Contains(sampleTypeLower, "samples") {
			typeIndex = i
			sampleType = "CPU"
			isCPU = true
			break
		}
		if strings.Contains(sampleTypeLower, "alloc") || strings.Contains(sampleTypeLower, "heap") || strings.Contains(sampleTypeLower, "inuse") {
			typeIndex = i
			sampleType = "Memory"
			break
		}
	}

	if typeIndex == -1 {
		return nil
	}

	funcMap := make(map[string]*FunctionStat)
	var total int64

	if isCPU {
		total = (time.Duration(prof.DurationNanos) * time.Nanosecond).Milliseconds()
	} else {
		for _, sample := range prof.Sample {
			total += sample.Value[typeIndex]
		}
	}

	for _, sample := range prof.Sample {
		value := sample.Value[typeIndex]
		if isCPU {
			value *= 10
		}
		for _, loc := range sample.Location {
			for _, line := range loc.Line {
				if line.Function != nil {
					name := line.Function.Name
					fileLocation := fmt.Sprintf("%s:%d(memloc: 0x%x)", line.Function.Filename, line.Line, loc.Address)
					if stat, existed := funcMap[name]; existed {
						stat.Value += value
						stat.Calls++
						stat.Percent = float64(stat.Value) / float64(total)
					} else {
						funcMap[name] = &FunctionStat{
							Name:         name,
							Value:        value,
							Calls:        1,
							FileLocation: fileLocation,
							Percent:      float64(value) / float64(total),
							SampleType:   sampleType,
						}
					}
				}
			}
		}
	}

	var funcStats []FunctionStat
	for _, stat := range funcMap {
		threshold := int64(100)
		if !isCPU {
			threshold = 1024 * 1024 * 10 // 1MB for memory
		}
		if stat.Value > threshold {
			funcStats = append(funcStats, *stat)
		}
	}

	sort.Slice(funcStats, func(i, j int) bool {
		return funcStats[i].Value > funcStats[j].Value
	})

	return funcStats
}

// AutoAnalyzeFile 分析指定的 pprof 文件并返回人类可读的分析结果
func AutoAnalyzeFile(filename string) (string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %v", err)
	}

	prof, err := profile.Parse(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("解析 pprof 文件失败: %v", err)
	}

	stats := analyzePprof(prof)
	if len(stats) == 0 {
		return "", fmt.Errorf("未发现性能数据")
	}

	var buf strings.Builder
	buf.WriteString(fmt.Sprintf("分析文件: %s\n", filename))
	buf.WriteString("----------------------------------------\n")
	buf.WriteString(DumpFunctionStats(stats))
	return buf.String(), nil
}

func AutoAnalyzeRaw(reader io.Reader) ([]FunctionStat, error) {
	prof, err := profile.Parse(reader)
	if err != nil {
		return nil, fmt.Errorf("解析 pprof 文件失败: %v", err)
	}

	stats := analyzePprof(prof)
	if len(stats) == 0 {
		return nil, fmt.Errorf("未发现性能数据")
	}

	return stats, nil
}

func DumpFunctionStats(stats []FunctionStat) string {
	var buf strings.Builder
	for _, f := range stats {
		buf.WriteString(f.Dump())
		buf.WriteString("\n")
	}
	return buf.String()
}
