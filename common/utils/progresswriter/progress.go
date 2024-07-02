package progresswriter

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"strconv"
	"time"
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

func (wc *ProgressWriter) ShowProgress(verbose string, ctx context.Context) {
	go func() {
		defer func() {
			log.Infof("progress: %7s (%v/%v)", "down", wc.Total, wc.Total)
		}()
		for {
			select {
			case <-ctx.Done():
				return
			default:
				prefix := ""
				if verbose != "" {
					prefix = verbose + " "
				}
				percentStr := fmt.Sprintf(`%3.2f%%`, wc.GetPercent()*100)
				log.Infof("%vprogress: %7s (%v/%v)", prefix, percentStr, wc.Count, wc.Total)
				time.Sleep(time.Second)
			}
		}
	}()
}
