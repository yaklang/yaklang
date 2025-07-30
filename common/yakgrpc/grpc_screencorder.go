package yakgrpc

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/yaklang/yaklang/common/schema"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/go-funk"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/screcorder"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/bizhelper"
	"github.com/yaklang/yaklang/common/utils/ffmpegutils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/grpc"
)

func (s *Server) QueryScreenRecorders(ctx context.Context, req *ypb.QueryScreenRecorderRequest) (*ypb.QueryScreenRecorderResponse, error) {
	p, data, err := yakit.QueryScreenRecorder(consts.GetGormProjectDatabase(), req)
	if err != nil {
		return nil, err
	}
	return &ypb.QueryScreenRecorderResponse{
		Pagination: req.GetPagination(),
		Data: funk.Map(data, func(i *schema.ScreenRecorder) *ypb.ScreenRecorder {
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
				Before:    before,
				After:     after,
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

type DownloadStream interface {
	Send(result *ypb.ExecResult) error
	grpc.ServerStream
}

func (s *Server) InstallScrecorder(req *ypb.InstallScrecorderRequest, stream ypb.Yak_InstallScrecorderServer) error {
	return s.DownloadWithStream(req.GetProxy(), func() (urlStr string, name string, err error) {
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
			return "", "", utils.Error("unsupported os: " + runtime.GOOS)
		}
		return targetUrl, filename, nil
	}, stream)
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

	proj, err := yakit.GetCurrentProject(consts.GetGormProfileDatabase(), yakit.TypeProject)
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

	cfg := screcorder.NewDefaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}
	// TODO: support other platforms
	devices := screcorder.GetDarwinAvailableAVFoundationScreenDevices()
	if len(devices) == 0 {
		return utils.Errorf("no screen device found")
	}
	dev := devices[0]

	recorder, err := screcorder.NewScreenRecorder(cfg, dev)
	if err != nil {
		return err
	}
	go func() {
		select {
		case <-stream.Context().Done():
			recorder.Stop()
		}
	}()

	projectPath := filepath.Join(consts.GetDefaultYakitProjectsDir(), "records")
	if utils.GetFirstExistedFile(projectPath) == "" {
		os.MkdirAll(projectPath, 0777)
	}

	var recordName = filepath.Join(projectPath, fmt.Sprintf("screen_records_%v.mp4", utils.DatetimePretty2()))
	err = recorder.Start(context.Background())
	if err != nil {
		return utils.Errorf("start to execute screen recorder failed: %s", err)
	}

	select {
	case <-stream.Context().Done():
		recorder.Stop()
	}

	for {
		if !recorder.IsRecording() {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	// move file
	tmpFilename := recorder.Filename()
	defer os.Remove(tmpFilename)

	err = utils.CopyFile(tmpFilename, recordName)
	if err != nil {
		return err
	}

	duration, err := ffmpegutils.GetVideoDuration(recordName)
	if err != nil {
		log.Warnf("get video duration failed: %v", err)
	}
	frameData, err := ffmpegutils.ExtractSpecificFrame(recordName, 1)
	if err != nil {
		log.Errorf("convert video to base64 failed: %v, use default(empty)", err)
	}
	var base64Images string
	if frameData != nil {
		base64Images = base64.StdEncoding.EncodeToString(frameData)
	}
	record := &schema.ScreenRecorder{
		Filename:  recordName,
		Project:   proj.ProjectName,
		Cover:     base64Images,
		VideoName: filepath.Base(recordName),
		Duration:  fmt.Sprintf("%d", duration.Milliseconds()),
	}
	err = yakit.CreateOrUpdateScreenRecorder(consts.GetGormProjectDatabase(), record.CalcHash(), record)
	if err != nil {
		log.Errorf("save screen recorder failed: %v", err)
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

func (s *Server) GetOneScreenRecorders(ctx context.Context, req *ypb.GetOneScreenRecorderRequest) (*ypb.ScreenRecorder, error) {
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
		Before:    before,
		After:     after,
	}, nil
}

func AfterAndBeforeIsExit(id int64) (before, after bool) {
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

func (s *Server) UpdateScreenRecorders(ctx context.Context, req *ypb.UpdateScreenRecorderRequest) (*ypb.Empty, error) {
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
