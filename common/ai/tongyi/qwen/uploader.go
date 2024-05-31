package qwen

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	httpclient "github.com/yaklang/yaklang/common/ai/tongyi/httpclient"
)

// upload image to aliyun oss that can be described by LLM

type CertResponse struct {
	RequestID string     `json:"request_id"`
	Data      CertOutput `json:"data"`
}

func (c *CertResponse) JSONString() string {
	if c == nil {
		return ""
	}

	jsonByte, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return string(jsonByte)
	// return c.RequestID
}

type CertOutput struct {
	Policy              string `json:"policy"`
	Signature           string `json:"signature"`
	UploadDir           string `json:"upload_dir"`
	UploadHost          string `json:"upload_host"`
	ExpireInSeconds     int    `json:"expire_in_seconds"`
	MaxFileSizeMB       int    `json:"max_file_size_mb"`
	CapacityLimitMB     int    `json:"capacity_limit_mb"`
	OSSAccessKeyID      string `json:"oss_access_key_id"`
	XOSSObjectACL       string `json:"x_oss_object_acl"`
	XOSSForbidOverwrite string `json:"x_oss_forbid_overwrite"`
}

// url:  https://dashscope.aliyuncs.com/api/v1/uploads
// header:  {'user-agent': 'dashscope/1.14.0; python/3.11.6; platform/macOS-14.2.1-arm64-arm-64bit; processor/arm', 'Authorization': 'Bearer sk-xxxx', 'Accept': 'application/json'}
// params:  {'action': 'getPolicy', 'model': 'qwen-vl-plus'}
// timeout:  300
func getUploadCertificate(ctx context.Context, model, apiKey string) (*CertResponse, error) {
	url := DashScopeBaseURL + "/v1/uploads"
	header := map[string]string{
		"Authorization": "Bearer " + apiKey,
		"Accept":        "application/json",
	}
	params := map[string]string{
		"action": "getPolicy",
		"model":  model,
	}

	headerOpt := httpclient.WithHeader(header)
	timeoutOpt := httpclient.WithTimeout(3 * time.Second)

	cli := httpclient.NewHTTPClient()
	respData := &CertResponse{}
	err := cli.Get(ctx, url, params, respData, headerOpt, timeoutOpt)
	if err != nil {
		// panic(err)
		return nil, &WrapMessageError{Message: "certificate Error", Cause: err}
	}

	return respData, nil
}

type UploadRequest struct {
	File []byte `json:"file"`
}

// TODO:...
// if has upload resource set headers['X-DashScope-OssResourceResolve'] = 'enable'
/*
	elem["image"] = ossURL 把原始的local_path 替换成 ossURL
	"content": [
		{"image": ""},
		{"text": ""},
	]
*/

// uploading local image to aliyun oss, a oss url will be returned.
func UploadLocalFile(ctx context.Context, filePath, model, apiKey string, uploadCacher UploadCacher) (string, error) {
	fileBytes, mimeType, err := loadLocalFileWithMimeType(filePath)
	if err != nil {
		return "", err
	}

	fileName := filepath.Base(filePath)

	if uploadCacher != nil {
		return uploadFileWithCache(ctx, fileBytes, fileName, mimeType, model, apiKey, uploadCacher)
	}

	return uploadFile(ctx, fileBytes, fileName, mimeType, model, apiKey)
}

// download and uploading a online image to aliyun oss, a oss url will be returned.
func UploadFileFromURL(ctx context.Context, fileURL, model, apiKey string, uploadCacher UploadCacher) (string, error) {
	fileBytes, mimeType, err := downloadFileWithMimeType(fileURL)
	if err != nil {
		return "", err
	}
	fileName := filepath.Base(fileURL)

	if uploadCacher != nil {
		return uploadFileWithCache(ctx, fileBytes, fileName, mimeType, model, apiKey, uploadCacher)
	}

	return uploadFile(ctx, fileBytes, fileName, mimeType, model, apiKey)
}

func uploadFileWithCache(ctx context.Context, fileBytes []byte, fileName, mimeType, model, apiKey string, uploadCacher UploadCacher) (string, error) {
	var ossURL string
	var err error

	ossURL = uploadCacher.GetCache(fileBytes)
	if ossURL != "" {
		return ossURL, nil
	}

	ossURL, err = uploadFile(ctx, fileBytes, fileName, mimeType, model, apiKey)
	if err != nil {
		return "", err
	}

	err = uploadCacher.SaveCache(fileBytes, ossURL)
	if err != nil {
		log.Printf("save upload cache error: %v\n", err)
	}

	return ossURL, nil
}

func uploadFile(ctx context.Context, fileBytes []byte, fileName, mimeType, model, apiKey string) (string, error) {
	certInfo, err := getUploadCertificate(ctx, model, apiKey)
	if err != nil {
		return "", &WrapMessageError{Message: "upload Cert Error", Cause: err}
	}

	ossKey := certInfo.Data.UploadDir + "/" + fileName

	formData := buildFormData(certInfo.Data, ossKey, mimeType)

	header := getUploadHeaders()
	uploadReq, extraHeaders, err := buildMultiPartRequest(fileName, formData, fileBytes)
	if err != nil {
		return "", err
	}
	for k, v := range extraHeaders {
		header[k] = v
	}

	cli := httpclient.NewHTTPClient()
	headerOpt := httpclient.WithHeader(header)

	err = cli.Post(ctx, certInfo.Data.UploadHost, uploadReq, nil, headerOpt)
	if err != nil {
		return "", &httpclient.HTTPRequestError{Message: "upload image Error", Cause: err}
	}

	ossURL := "oss://" + ossKey

	return ossURL, nil
}

func getUploadHeaders() map[string]string {
	headers := make(map[string]string)
	// headers["User-Agent"] = getUserAgent() // Assuming you have a function getUserAgent()
	headers["Accept"] = "application/json"
	headers["Date"] = time.Now().Format(time.RFC1123) // Go's equivalent to Python's format_date_time(mktime(datetime.now().timetuple()))
	return headers
}

func loadLocalFileWithMimeType(filePath string) ([]byte, string, error) {
	imgBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, "", &WrapMessageError{Message: "read file Error", Cause: err}
	}

	mt, err := mimetype.DetectFile(filePath)
	if err != nil {
		return nil, "", &WrapMessageError{Message: "detectfile mimetype Error", Cause: err}
	}

	return imgBytes, mt.String(), nil
}

func downloadFileWithMimeType(url string) ([]byte, string, error) {
	resp, err := http.Get(url) //nolint:all
	if err != nil {
		return nil, "", &WrapMessageError{Message: "http get image Error", Cause: err}
	}
	defer resp.Body.Close()

	imgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", &WrapMessageError{Message: "read image Error", Cause: err}
	}

	mt, err := mimetype.DetectReader(bytes.NewReader(imgBytes))
	if err != nil {
		return nil, "", &WrapMessageError{Message: "detect image mimetype Error", Cause: err}
	}

	return imgBytes, mt.String(), nil
}

// build request body for upload file.
func buildMultiPartRequest(fileName string, formData map[string]string, fileBytes []byte) (*bytes.Buffer, map[string]string, error) {
	// buffer to save multipart body
	requestBody := new(bytes.Buffer)
	writer := multipart.NewWriter(requestBody)

	// write form data to buffer
	for key, value := range formData {
		_ = writer.WriteField(key, value)
	}

	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return requestBody, nil, &WrapMessageError{Message: "create form file Error", Cause: err}
	}

	// write file bytes to buffer
	_, err = part.Write(fileBytes)
	if err != nil {
		return requestBody, nil, &WrapMessageError{Message: "write Error", Cause: err}
	}

	// Content-Type header content
	contentType := writer.FormDataContentType()
	err = writer.Close()
	if err != nil {
		return requestBody, nil, &WrapMessageError{Message: "close writer Error", Cause: err}
	}

	header := make(map[string]string)
	header["Content-Type"] = contentType

	return requestBody, header, nil
}

func buildFormData(certOutput CertOutput, ossKey, mimeType string) map[string]string {
	formData := make(map[string]string)
	formData["OSSAccessKeyId"] = certOutput.OSSAccessKeyID
	formData["Signature"] = certOutput.Signature
	formData["policy"] = certOutput.Policy
	formData["key"] = ossKey
	formData["x-oss-object-acl"] = certOutput.XOSSObjectACL
	formData["x-oss-forbid-overwrite"] = certOutput.XOSSForbidOverwrite
	formData["success_action_status"] = "200"
	formData["x-oss-content-type"] = mimeType
	return formData
}
