package yakgrpc

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/screcorder"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) QueryScreenRecorders(ctx context.Context, req *ypb.QueryScreenRecorderRequest) (*ypb.QueryScreenRecorderResponse, error) {
	p, data, err := yakit.QueryScreenRecorder(consts.GetGormProjectDatabase(), req)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryScreenRecorderResponse{
		Pagination: req.GetPagination(),
		Data: funk.Map(data, func(i *yakit.ScreenRecorder) *ypb.ScreenRecorder {
			return &ypb.ScreenRecorder{
				Id:        int64(i.ID),
				Filename:  i.Filename,
				NoteInfo:  i.NoteInfo,
				Project:   i.Project,
				CreatedAt: i.CreatedAt.Unix(),
				UpdatedAt: i.UpdatedAt.Unix(),
			}
		}).([]*ypb.ScreenRecorder),
		Total: int64(p.TotalRecord),
	}, nil
}

func (s *Server) IsScrecorderReady(ctx context.Context, req *ypb.IsScrecorderReadyRequest) (*ypb.IsScrecorderReadyResponse, error) {
	ok, reason := screcorder.IsAvailable()
	if reason != nil {
		return &ypb.IsScrecorderReadyResponse{
			Ok: ok, Reason: fmt.Sprint(reason),
		}, nil
	}
	return &ypb.IsScrecorderReadyResponse{
		Ok: ok,
	}, nil
}

func (s *Server) InstallScrecorder(req *ypb.InstallScrecorderRequest, stream ypb.Yak_InstallScrecorderServer) error {
	info := func(s string, items ...interface{}) {
		var msg string
		if len(items) > 0 {
			msg = fmt.Sprintf(s, items)
		} else {
			msg = s
		}
		log.Info(msg)
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(msg),
		})
	}

	var targetUrl string
	var filename string
	switch runtime.GOOS {
	case "darwin":
		targetUrl = "https://yaklang.oss-accelerate.aliyuncs.com/ffmpeg/ffmpeg-v6.0-darwin-amd64"
		filename = "ffmpeg"
	case "windows":
		targetUrl = "https://yaklang.oss-accelerate.aliyuncs.com/ffmpeg/ffmpeg-v6.0-windows-amd64.exe"
		filename = "ffmpeg.exe"
	default:
		return utils.Error("unsupported os: " + runtime.GOOS)
	}

	info("获取下载材料大小: Fetching Download Material Basic Info")
	client := utils.NewDefaultHTTPClientWithProxy(req.GetProxy())
	client.Timeout = time.Hour
	rsp, err := client.Head(targetUrl)
	if err != nil {
		return err
	}

	i, err := strconv.Atoi(rsp.Header.Get("Content-Length"))
	if err != nil {
		return utils.Errorf("cannot fetch cl: %v", err)
	}
	info("共需下载大小为：Download %v Total", utils.ByteSize(uint64(i)))

	rsp, err = client.Get(targetUrl)
	if err != nil {
		return utils.Errorf("download ffmpeg failed: %s", err)
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
				info("Progress: %.2f%%", prog.GetPercent()*100)
				if prog.GetPercent() >= 1 {
					return
				}
			}
		}
	}()

	_, err = io.Copy(fp, io.TeeReader(rsp.Body, prog))
	if err != nil {
		fp.Close()
		info("下载文件失败: Download Failed: %s", err)
		return nil
	}
	fp.Close()
	info("下载文件成功：Download Finished")
	return nil
}

func (s *Server) StartScrecorder(req *ypb.StartScrecorderRequest, stream ypb.Yak_StartScrecorderServer) error {
	info := func(s string, items ...interface{}) {
		var msg string
		if len(items) > 0 {
			msg = fmt.Sprintf(s, items)
		} else {
			msg = s
		}
		log.Info(msg)
		stream.Send(&ypb.ExecResult{
			IsMessage: true,
			Message:   []byte(msg),
		})
	}
	_ = info

	proj, err := yakit.GetCurrentProject(consts.GetGormProfileDatabase())
	if err != nil {
		return utils.Errorf("cannot bind screen recorder to proj: %v", err)
	}

	var opts []screcorder.ConfigOpt
	if req.GetFramerate() > 0 {
		opts = append(opts, screcorder.WithFramerate(int(req.GetFramerate())))
	}

	if req.GetCoefficientPTS() > 0 {
		opts = append(opts, screcorder.WithCoefficientPTS(req.GetCoefficientPTS()))
	}

	if req.GetDisableMouse() {
		opts = append(opts, screcorder.WithMouseCapture(false))
	}

	if req.GetResolutionSize() != "" {
		opts = append(opts, screcorder.WithResolutionSize(req.GetResolutionSize()))
	}
	recorder := screcorder.NewRecorder(opts...)
	go func() {
		select {
		case <-stream.Context().Done():
			recorder.Stop()
		}
	}()

	recorder.OnFileAppended(func(r string) {
		record := &yakit.ScreenRecorder{
			Filename: r,
			Project:  proj.ProjectName,
		}
		err := yakit.CreateOrUpdateScreenRecorder(consts.GetGormProjectDatabase(), record.CalcHash(), record)
		if err != nil {
			log.Errorf("save screen recorder failed: %v", err)
		}
	})

	projectPath := filepath.Join(consts.GetDefaultYakitProjectsDir(), "records")
	if utils.GetFirstExistedFile(projectPath) == "" {
		os.MkdirAll(projectPath, 0777)
	}

	var recordName = filepath.Join(projectPath, fmt.Sprintf("screen_records_%v.mp4", utils.DatetimePretty2()))
	err = recorder.Start(recordName)
	if err != nil {
		return utils.Errorf("start to execute screen recorder failed: %s", err)
	}

	select {
	case <-stream.Context().Done():
		recorder.Stop()
	}

	for {
		if !recorder.IsRunning() {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil
}
