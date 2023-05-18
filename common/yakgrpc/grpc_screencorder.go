package yakgrpc

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/screcorder"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/progresswriter"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

func (s *Server) QueryScreenRecorders(ctx context.Context, req *ypb.QueryScreenRecorderRequest) (*ypb.QueryScreenRecorderResponse, error) {
	p, data, err := yakit.QueryScreenRecorder(consts.GetGormProjectDatabase(), req)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryScreenRecorderResponse{
		Pagination: req.GetPagination(),
		Data: funk.Map(data, func(i *yakit.ScreenRecorder) *ypb.ScreenRecorder {
			before, after := AfterAndBeforeIsExit(int64(i.ID))
			return &ypb.ScreenRecorder{
				Id:        int64(i.ID),
				Filename:  i.Filename,
				NoteInfo:  i.NoteInfo,
				Project:   i.Project,
				CreatedAt: i.CreatedAt.Unix(),
				UpdatedAt: i.UpdatedAt.Unix(),
				VideoName: i.VideoName,
				Cover:     i.Cover,
				Duration:  i.Duration,
				Before: before,
				After: after,
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

	info(0,"获取下载材料大小: Fetching Download Material Basic Info")
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
	info(0,"共需下载大小为：Download %v Total", utils.ByteSize(uint64(i)))
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
		info(0,"下载文件失败: Download Failed: %s", err)
		return nil
	}
	fp.Close()
	info(100, "下载文件成功：Download Finished")
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
		opts = append(opts, screcorder.WithMouseCapture(req.GetDisableMouse()))
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
		duration := screcorder.VideoDuration(r)
		base64Images, _ := screcorder.VideoCoverBase64(r)
		record := &yakit.ScreenRecorder{
			Filename: r,
			Project:  proj.ProjectName,
			Cover: base64Images,
			VideoName:  filepath.Base(r),
			Duration: duration,
		}
		err = yakit.CreateOrUpdateScreenRecorder(consts.GetGormProjectDatabase(), record.CalcHash(), record)
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

func (s *Server) DeleteScreenRecorders(ctx context.Context, req *ypb.QueryScreenRecorderRequest) (*ypb.Empty, error) {
	db := s.GetProjectDatabase()
	db = bizhelper.ExactQueryString(db, "project", req.GetProject())
	db = bizhelper.FuzzSearchEx(db, []string{
		"video_name", "note_info",
	}, req.Keywords, false)
	if len(req.Ids) > 0 {
		db = db.Where("id in (?)", req.Ids)
	}
	data := yakit.BatchScreenRecorder(db, ctx)
	var deleteNum int
	for k := range data {
		file, _ := os.OpenFile(k.Filename, os.O_APPEND, 0777)
		file.Close()
		err := os.RemoveAll(k.Filename)
		if err != nil {
			log.Error("删除本地数据库失败：" + err.Error())
		}
		err = yakit.DeleteScreenRecorder(s.GetProjectDatabase(), int64(k.ID))
		if err != nil {
			deleteNum++
			log.Error("删除录屏失败：" + err.Error())
		}
	}
	if deleteNum > 0 {
		return nil, utils.Error(fmt.Sprintf("%v%v", deleteNum, "条视频数据删除失败"))
	}
	return &ypb.Empty{}, nil
}

func (s *Server) UploadScreenRecorders(ctx context.Context, req *ypb.UploadScreenRecorderRequest) (*ypb.Empty, error) {
	if req.Token == "" {
		return nil, utils.Errorf("empty params")
	}
	db := s.GetProjectDatabase()
	db = bizhelper.ExactQueryString(db, "project", req.GetProject())
	db = bizhelper.FuzzSearchEx(db, []string{
		"video_name", "note_info",
	}, req.Keywords, false)
	if len(req.Ids) > 0 {
		db = db.Where("id in (?)", req.Ids)
	}
	data := yakit.BatchScreenRecorder(db, context.Background())
	var uploadNum int
	for k := range data {
		client := yaklib.NewOnlineClient(consts.GetOnlineBaseUrl())
		file, err := os.Open(k.Filename)
		if err != nil {
			continue
		}
		err = client.UploadScreenRecordersWithToken(ctx, req.Token, *file, k)
		if err != nil {
			uploadNum++
			log.Errorf("UploadScreenRecorders failed: %s", err)
		}
		file.Close()
	}
	if uploadNum > 0 {
		return nil, utils.Error(fmt.Sprintf("%v%v", uploadNum, "条视频数据上传失败"))
	}
	return &ypb.Empty{}, nil
}

func (s *Server) GetOneScreenRecorders(ctx context.Context, req *ypb.GetOneScreenRecorderRequest) (*ypb.ScreenRecorder, error)  {
	data, err := yakit.GetOneScreenRecorder(s.GetProjectDatabase(), req)
	if err != nil {
		return nil, err
	}
	var before, after bool
	before, after = AfterAndBeforeIsExit(int64(data.ID))
	return &ypb.ScreenRecorder{
		Id:        int64(data.ID),
		Filename:  data.Filename,
		NoteInfo:  data.NoteInfo,
		Project:   data.Project,
		CreatedAt: data.CreatedAt.Unix(),
		UpdatedAt: data.UpdatedAt.Unix(),
		VideoName: data.VideoName,
		Cover:     data.Cover,
		Before: before,
		After: after,
	}, nil
}

func AfterAndBeforeIsExit(id int64) (before, after bool)  {
	// 下一条
	beforeData, _ := yakit.IsExitScreenRecorder(consts.GetGormProjectDatabase(), id, "asc")
	if beforeData != nil {
		before = true
	}
	// 上一条
	afterData, _ := yakit.IsExitScreenRecorder(consts.GetGormProjectDatabase(), id, "desc")
	if afterData != nil {
		after = true
	}
	return before, after
}

func (s *Server) UpdateScreenRecorders(ctx context.Context, req *ypb.UpdateScreenRecorderRequest) (*ypb.Empty, error)  {
	if req.GetId() == 0 {
		return nil, utils.Error("request params is nil")
	}
	if req.NoteInfo == "" && req.VideoName == "" {
		return nil, utils.Error("params is nil")
	}
	flow, err := yakit.GetScreenRecorder(consts.GetGormProjectDatabase(), req.Id)
	if err != nil {
		return nil, utils.Error("UpdateScreenRecorders failed ")
	}
	if req.VideoName != "" && req.VideoName != flow.VideoName {
		flow.VideoName = req.VideoName
	}
	if req.NoteInfo != "" && req.NoteInfo != flow.NoteInfo {
		flow.NoteInfo = req.NoteInfo
	}
	if db := consts.GetGormProjectDatabase().Save(flow); db.Error != nil {
		return nil, db.Error
	}
	return &ypb.Empty{}, nil
}