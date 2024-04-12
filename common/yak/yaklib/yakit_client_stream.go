package yaklib

import (
	"bufio"
	"bytes"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"time"
)

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

	err := c.YakitLog("stream", string(utils.Jsonify(map[string]any{
		"type":       "stream",
		"action":     "start",
		"streamType": streamType,
		"streamId":   streamId,
		"extra":      params,
	})))
	if err != nil {
		log.Warnf("stream log failed: %s", err)
		return
	}

	go func() {
		// read with buf until EOF, set 0.2s as interval
		defer func() {
			if err := recover(); err != nil {
				log.Errorf("stream panic: %v", err)
			}
			err := c.YakitLog("stream", string(utils.Jsonify(map[string]any{
				"type":     "stream",
				"action":   "stop",
				"streamId": streamId,
			})))
			if err != nil {
				log.Warnf("stream log failed: %s", err)
				return
			}
		}()
		bstream := bufio.NewScanner(stream)
		bstream.Split(bufio.ScanRunes)
		var buf = bytes.NewBufferString("")
		lastTimeMS := time.Now().UnixMilli()
		for bstream.Scan() {
			buf.WriteString(bstream.Text())

			if time.Now().UnixMilli()-lastTimeMS > 200 {
				err := c.YakitLog("stream", string(utils.Jsonify(map[string]any{
					"type":     "stream",
					"streamId": streamId,
					"action":   "data",
					"data":     buf.String(),
				})))
				if err != nil {
					log.Warnf("stream send failed: %s", err)
					continue
				}
				buf.Reset()
				lastTimeMS = time.Now().UnixMilli()
				continue
			}
		}
		if buf.Len() > 0 {
			err := c.YakitLog("stream", string(utils.Jsonify(map[string]any{
				"action":   "data",
				"data":     buf.String(),
				"streamId": streamId,
				"type":     "stream",
			})))
			if err != nil {
				log.Warnf("stream send failed: %s", err)
				return
			}
		}
	}()
}
