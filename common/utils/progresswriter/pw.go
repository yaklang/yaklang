package progresswriter

import (
	"fmt"
	"strconv"
)

type ProgressWriter struct {
	Total uint64
	Count uint64
}

func New(total uint64) *ProgressWriter {
	return &ProgressWriter{
		Total: total,
	}
}

func (wc *ProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	wc.Count += uint64(n)
	return n, nil
}

func (wc *ProgressWriter) GetPercent() float64 {
	if wc.Total <= 0 {
		return 0
	}
	f, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", float64(wc.Count)/float64(wc.Total)), 64)
	return f
}
