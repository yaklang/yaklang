package yakgrpc

import (
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func (s *Server) DownloadWithStream(proxy string, fileGetter func() (urlStr string, name string, err error), stream DownloadStream) error {
	if fileGetter == nil {
		return utils.Error("fileGetter is nil")
	}
	info := func(progress float64, s string, items ...interface{}) {
		var msg string
		if len(items) > 0 {
			msg = fmt.Sprintf(s, items)
		} else {
			msg = s
		}
		log.Info(msg)
		progressInfo, _ := strconv.ParseFloat(fmt.Sprintf("%.2f", progress), 64)
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(msg),
			Progress:  float32(progressInfo),
		})
	}

	var targetUrl, filename, err = fileGetter()
	if err != nil {
		return utils.Errorf("cannot get file: %v", err)
	}

	info(0, "获取下载材料大小: Fetching Download Material Basic Info")
	client := utils.NewDefaultHTTPClientWithProxy(proxy)
	client.Timeout = time.Hour
	rsp, err := client.Head(targetUrl)
	if err != nil {
		return err
	}

	i, err := strconv.Atoi(rsp.Header.Get("Content-Length"))
	if err != nil {
		return utils.Errorf("cannot fetch cl: %v", err)
	}
	info(0, "共需下载大小为：Download %v Total", utils.ByteSize(uint64(i)))
	rsp, err = client.Get(targetUrl)
	if err != nil {
		return utils.Errorf("download material failed: %s", err)
	}

	dirPath := filepath.Join(
		consts.GetDefaultYakitProjectsDir(),
		"libs",
	)
	err = os.MkdirAll(dirPath, 0755)
	if err != nil {
		return err
	}
	fPath := filepath.Join(dirPath, filename)
	os.RemoveAll(fPath)
	fp, err := os.OpenFile(fPath, os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		return err
	}
	prog := progresswriter.New(uint64(i))
	go func() {
		for {
			time.Sleep(time.Second)
			select {
			case <-stream.Context().Done():
				return
			default:
				info(prog.GetPercent()*100, "")
				if prog.GetPercent() >= 1 {
					return
				}
			}
		}
	}()

	_, err = io.Copy(fp, io.TeeReader(rsp.Body, prog))
	if err != nil {
		fp.Close()
		info(0, "下载文件失败: Download Failed: %s", err)
		return nil
	}
	fp.Close()
	info(100, "下载文件成功：Download Finished")
	return nil
}
