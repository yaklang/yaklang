package yaklib

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

type ScreenRecordersUploadToOnlineRequest struct {
	//Filename 	string	 `json:"filename"`
	NoteInfo                 string  `json:"note_info"`
	Project                  string  `json:"project"`
	Hash                     string  `json:"hash"`
	VideoName                string  `json:"video_name"`
	Cover                    string  `json:"cover"`
	Token                    string  `json:"token"`
	VideoFile                os.File `json:"video_file"`
	ScreenRecordersCreatedAt int64   `json:"screen_recorders_created_at"`
}

func (s *OnlineClient) UploadScreenRecordersWithToken(ctx context.Context, token string, file os.File, screenRecorders *yakit.ScreenRecorder) error {
	err := s.UploadScreenRecordersToOnline(ctx,
		token,
		file,
		screenRecorders.NoteInfo,
		screenRecorders.Project,
		screenRecorders.Hash,
		screenRecorders.VideoName,
		screenRecorders.Cover,
		screenRecorders.CreatedAt.Unix(),
		screenRecorders.Filename,
	)
	if err != nil {
		log.Errorf("upload ScreenRecorders to online failed: %s", err.Error())
		return utils.Errorf("upload ScreenRecorders to online failed: %s", err.Error())
	}

	return nil
}

func (s *OnlineClient) UploadScreenRecordersToOnline(ctx context.Context,
	token string, file os.File, noteInfo string, project string, hash string, videoName string, cover string, screenRecordersCreatedAt int64, filePath string) error {
	urlIns, err := url.Parse(s.genUrl("/api/upload/screen/recorders"))
	if err != nil {
		return utils.Errorf("parse url-instance failed: %s", err)
	}
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)

	files, err := os.Open(filePath)
	if err != nil {
		return utils.Errorf("Error opening file: %v", err)
	}
	defer file.Close()

	// 创建一个文件字段
	fileInfo, err := writer.CreateFormFile("video_file", filepath.Base(filePath))
	if err != nil {
		return utils.Errorf("Error creating form file: %v", err)
	}

	// 将文件内容复制到文件字段中
	_, err = io.Copy(fileInfo, files)
	if err != nil {
		return utils.Errorf("Error copying file data: %v", err)
	}
	valueStr := strconv.FormatInt(screenRecordersCreatedAt, 10)
	writer.WriteField("screen_recorders_created_at", valueStr)
	writer.WriteField("token", token)
	writer.WriteField("note_info", noteInfo)
	writer.WriteField("project", project)
	writer.WriteField("hash", hash)
	writer.WriteField("video_name", videoName)
	writer.WriteField("cover", cover)

	writer.Close()

	rsp, err := s.client.Post(urlIns.String(), writer.FormDataContentType(), body)
	if err != nil {
		return utils.Errorf("HTTP Post %v failed: %v params:%s", urlIns.String(), err, writer.FormDataContentType())
	}
	rawResponse, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		return utils.Errorf("read body failed: %s", err)
	}
	var responseData map[string]interface{}
	err = json.Unmarshal(rawResponse, &responseData)
	if err != nil {
		return utils.Errorf("unmarshal upload ScreenRecorder to online response failed: %s", err)
	}
	if !utils.MapGetBool(responseData, "ok") {
		return utils.Errorf("upload ScreenRecorder to online failed: %s", utils.MapGetString(responseData, "reason"))
	}
	return nil
}
