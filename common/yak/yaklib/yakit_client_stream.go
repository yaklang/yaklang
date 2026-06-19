package yaklib

import (
	"bufio"
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"sync"
	"time"
)

// Stream 向 Yakit 持续输出一个流（如实时日志、命令输出，导出名为 yakit.Stream）
// 参数:
//   - streamType: 流类型标识
//   - streamId: 流 ID
//   - stream: 数据源 reader
//   - extra: 可选的额外信息
//
// 返回值:
//   - 无
//
// Example:
// ```
// // reader 为任意 io.Reader（示意性示例）
// yakit.Stream("exec", "task-1", reader)
// ```
func (c *YakitClient) Stream(streamType string, streamId string, stream io.Reader, extra ...any) {
	defer func() {
		if err := recover(); err != nil {
			log.Warnf("stream panic: %v", err)
		}
	}()
	if stream == nil || c == nil {
		// protect me!
		return
	}

	var params = make(map[string]any)
	for _, i := range extra {
		for k, v := range utils.InterfaceToGeneralMap(i) {
			params[k] = v
		}
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("stream panic: %v", err)
		}
		err := c.YakitLog("stream", string(utils.Jsonify(map[string]any{
			"type":       "stream",
			"action":     "stop",
			"streamType": streamType,
			"streamId":   streamId,
			"extra":      params,
		})))
		if err != nil {
			log.Warnf("stream log failed: %s", err)
			//return
		}
	}()
	bstream := bufio.NewScanner(stream)
	bstream.Split(bufio.ScanRunes)
	lastTimeMS := time.Now().UnixMilli()
	bufChannel := make(chan string, 0)
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := c.YakitLog("stream", string(utils.Jsonify(map[string]any{
			"type":       "stream",
			"action":     "start",
			"streamType": streamType,
			"streamId":   streamId,
			"extra":      params,
		})))
		if err != nil {
			log.Warnf("stream log failed: %s", err)
			//return
		}
		var buf = bytes.NewBufferString("")
		defer func() {
			if buf.Len() > 0 {
				err := c.YakitLog("stream", string(utils.Jsonify(map[string]any{
					"action":     "data",
					"data":       buf.String(),
					"streamId":   streamId,
					"type":       "stream",
					"streamType": streamType,
					"extra":      params,
				})))
				if err != nil {
					log.Warnf("stream send failed: %s", err)
					//return
				}
				buf.Reset()
			}
			wg.Done()
		}()
		warningOnce := new(sync.Once)
		for {
			select {
			case msg, ok := <-bufChannel:
				if !ok {
					return
				}
				buf.WriteString(msg)
			default:
				if buf.Len() > 0 && time.Now().UnixMilli()-lastTimeMS > 200 {
					err := c.YakitLog("stream", string(utils.Jsonify(map[string]any{
						"action":     "data",
						"data":       buf.String(),
						"streamId":   streamId,
						"type":       "stream",
						"streamType": streamType,
						"extra":      params,
					})))
					if err != nil {
						warningOnce.Do(func() {
							log.Warnf("stream send failed: %s", err)
						})
						//return
					}
					buf.Reset()
					lastTimeMS = time.Now().UnixMilli()
				} else {
					time.Sleep(10 * time.Millisecond)
				}
			}
		}
	}()
	for bstream.Scan() {
		bufChannel <- bstream.Text()
	}
	close(bufChannel)
	wg.Wait()
}
