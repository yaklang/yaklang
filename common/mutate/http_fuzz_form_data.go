package mutate

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"regexp"
	"strings"
	"yaklang/common/log"
	"yaklang/common/utils"
	"yaklang/common/utils/mixer"
	"yaklang/common/yak/yaklib/codec"
)

type multipartData struct {
	Boundary string                 `json:"boundary"`
	Files    map[string][]*formItem `json:"files"`
	Values   map[string][]*formItem `json:"values"`
}

func NewMultiPartData() *multipartData {
	return &multipartData{
		Boundary: "------------------------" + utils.RandStringBytes(40),
		Files:    make(map[string][]*formItem),
		Values:   make(map[string][]*formItem),
	}
}

func (d *multipartData) ReplaceValue(k, v string) {
	d.Values[k] = []*formItem{
		{FieldName: k, FieldValue: v},
	}
}

func (d *multipartData) ReplaceFile(fieldName, fileName string, fileContent []byte) {
	d.Files[fieldName] = []*formItem{
		{FieldName: fieldName, IsFile: true, FileName: fileName, FileContent: fileContent},
	}
}

func (d *multipartData) ReplaceFileName(fieldName, fileName string) {
	raw := d.Files[fieldName]
	if raw == nil {
		d.Files[fieldName] = []*formItem{
			{
				FieldName:   fieldName,
				IsFile:      true,
				FileContent: []byte("..."),
				FileName:    fieldName,
				Header: map[string][]string{
					"Content-Disposition": {
						fmt.Sprintf(`form-data; name="%v"; filename="%v"`,
							codec.EncodeUrlCode(fieldName),
							codec.EncodeUrlCode(fileName),
						),
					},
					"Content-Type": {"application/octet-stream"},
				},
			},
		}
		return
	}
	raw[0].FileName = codec.EncodeUrlCode(fieldName)
}

func (d *multipartData) Write(w *multipart.Writer) error {
	for _, v := range d.Values {
		for _, item := range v {
			err := w.WriteField(item.FieldName, item.FieldValue)
			if err != nil {
				return utils.Errorf("multipart write field[%s:%v] failed: %s", item.FieldName, item.FieldValue, err)
			}
		}
	}

	for _, v := range d.Files {
		for _, item := range v {
			f, err := w.CreatePart(item.Header)
			if err != nil {
				return utils.Errorf("multipart write file[%v:%v] failed: %s", item.FieldName, item.FileName, err)
			}
			_, err = f.Write(item.FileContent)
			if err != nil {
				return utils.Errorf("multipart write file content failed: %s", err)
			}
		}
	}
	return nil
}

type formItem struct {
	Header      textproto.MIMEHeader `json:"header"`
	FieldName   string               `json:"field_name"`
	FieldValue  string               `json:"field_value"`
	IsFile      bool                 `json:"is_file"`
	FileName    string               `json:"file_name"`
	FileContent []byte               `json:"file_content"`
}

var fetchFieldNameRegexp = regexp.MustCompile(`^name\s?=\s?"(.*)"`)
var fetchFileNameRegexp = regexp.MustCompile(`^filename\s?=\s?"(.*)"`)
var fetchBoundaryRegexp = regexp.MustCompile(`boundary\s?=\s?([^;]+)`)

func _fetchBoundaryRegexp(s string) string {
	res := fetchBoundaryRegexp.FindStringSubmatch(s)
	if len(res) > 0 {
		return res[1]
	}
	return ""
}

func _fetchFileName(s string) string {
	res := fetchFileNameRegexp.FindStringSubmatch(s)
	if len(res) > 0 {
		return res[1]
	}
	return ""
}

func _fetchFieldName(s string) string {
	res := fetchFieldNameRegexp.FindStringSubmatch(s)
	if len(res) > 0 {
		return res[1]
	}
	return ""
}

func parseRequestToFormData(req *http.Request) *multipartData {
	boundary := _fetchBoundaryRegexp(req.Header.Get("Content-Type"))
	if boundary == "" {
		log.Infof("cannot fetch boundary... maybe not a multipart request")
		return NewMultiPartData()
		//return nil, utils.
	}

	reader, err := req.MultipartReader()
	if err != nil {
		log.Infof("multipart read failed: %s", err)
		return NewMultiPartData()
	}

	f, err := reader.ReadForm(1024 * 1024 * 100)
	if err != nil {
		log.Infof("read multipart form form failed: %s", err)
		return NewMultiPartData()
	}
	mdata := &multipartData{
		Boundary: boundary,
		Files:    make(map[string][]*formItem),
		Values:   make(map[string][]*formItem),
	}
	for fieldName, v := range f.Value {
		for _, value := range v {
			item := &formItem{
				FieldName:  fieldName,
				FieldValue: value,
			}
			mdata.Values[item.FieldName] = append(mdata.Values[item.FieldName], item)
		}
	}

	for _, r := range f.File {
		for _, fileHeader := range r {
			f, err := fileHeader.Open()
			if err != nil {
				log.Errorf("open uploaded file failed: %s", err)
				return mdata
			}
			raw, err := ioutil.ReadAll(f)
			if err != nil {
				log.Errorf("read uploaded file[%s] failed: %s", fileHeader.Filename, err)
				return mdata
			}

			h, err := deepCopyMIMEHeader(fileHeader.Header)
			if err != nil {
				log.Errorf("deep copy mime.header failed: %s", err)
				return mdata
			}
			contentDisposition := h.Get("Content-Disposition")

			var fileName, fieldName string
			for _, par := range strings.Split(contentDisposition, ";") {
				par = strings.TrimSpace(par)
				if fileName == "" {
					fileName = _fetchFileName(par)
				}
				if fieldName == "" {
					fieldName = _fetchFieldName(par)
				}
			}

			if fileName == "" {
				log.Errorf("fileName is empty")
			}

			if fieldName == "" {
				log.Errorf("fieldName is empty")
			}

			item := &formItem{
				Header:      h,
				FieldName:   fieldName,
				IsFile:      true,
				FileName:    fileName,
				FileContent: raw,
			}
			mdata.Files[item.FieldName] = append(mdata.Files[item.FieldName], item)
		}
	}

	return mdata
}

func (f *FuzzHTTPRequest) fuzzFormEncoded(k, v interface{}) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	data := parseRequestToFormData(req)

	keys := InterfaceToFuzzResults(k)
	values := InterfaceToFuzzResults(v)
	if keys == nil || values == nil {
		return nil, utils.Errorf("keys or Values is empty...")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	var reqs []*http.Request
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		_, _ = key, value

		mdata, err := deepCopyMultipartData(data)
		if err != nil {
			return nil, utils.Errorf("multipart data deep copy failed: %s", err)
		}
		mdata.ReplaceValue(key, value)
		var buffer bytes.Buffer
		w := multipart.NewWriter(&buffer)
		_ = w.SetBoundary(data.Boundary)
		err = mdata.Write(w)
		if err != nil {
			return nil, err
		}
		_ = w.Close()

		_req, err := rebuildHTTPRequest(req, int64(len(buffer.Bytes())))
		if err != nil {
			return nil, utils.Errorf("fuzz rebuild http request failed: %s", err)
		}
		_req.Body = ioutil.NopCloser(&buffer)
		_req.Header.Set("Content-Type", w.FormDataContentType())
		if _req != nil {
			reqs = append(reqs, _req)
		}

		err = m.Next()
		if err != nil {
			break
		}
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzMultipartKeyValue(k, v interface{}) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	data := parseRequestToFormData(req)

	keys := InterfaceToFuzzResults(k)
	values := InterfaceToFuzzResults(v)
	if keys == nil || values == nil {
		return nil, utils.Errorf("keys or Values is empty...")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	var reqs []*http.Request
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		_, _ = key, value

		mdata, err := deepCopyMultipartData(data)
		if err != nil {
			return nil, utils.Errorf("multipart data deep copy failed: %s", err)
		}
		mdata.ReplaceValue(key, value)
		var buffer bytes.Buffer
		w := multipart.NewWriter(&buffer)
		_ = w.SetBoundary(data.Boundary)
		err = mdata.Write(w)
		if err != nil {
			return nil, err
		}
		_ = w.Close()

		_req, err := rebuildHTTPRequest(req, int64(buffer.Len()))
		if err != nil {
			return nil, utils.Errorf("fuzz rebuild http request failed: %s", err)
		}
		_req.Body = ioutil.NopCloser(&buffer)
		_req.Header.Set("Content-Type", w.FormDataContentType())
		if _req != nil {
			reqs = append(reqs, _req)
		}

		err = m.Next()
		if err != nil {
			break
		}
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzUploadedFileName(fieldName, fileName interface{}) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	data := parseRequestToFormData(req)

	keys := InterfaceToFuzzResults(fieldName)
	values := InterfaceToFuzzResults(fileName)
	if keys == nil || values == nil {
		return nil, utils.Errorf("keys or Values is empty...")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	var reqs []*http.Request
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		_, _ = key, value

		mdata, err := deepCopyMultipartData(data)
		if err != nil {
			return nil, utils.Errorf("multipart data deep copy failed: %s", err)
		}
		mdata.ReplaceFileName(key, value)
		var buffer bytes.Buffer
		w := multipart.NewWriter(&buffer)
		_ = w.SetBoundary(data.Boundary)
		err = mdata.Write(w)
		if err != nil {
			return nil, err
		}
		_ = w.Close()

		_req, err := rebuildHTTPRequest(req, int64(buffer.Len()))
		if err != nil {
			return nil, utils.Errorf("fuzz rebuild http request failed: %s", err)
		}
		_req.Body = ioutil.NopCloser(&buffer)
		_req.Header.Set("Content-Type", w.FormDataContentType())
		if _req != nil {
			reqs = append(reqs, _req)
		}

		err = m.Next()
		if err != nil {
			break
		}
	}
	return reqs, nil
}

func (f *FuzzHTTPRequest) fuzzUploadedFile(fieldName interface{}, fileNames interface{}, content []byte) ([]*http.Request, error) {
	req, err := f.GetOriginHTTPRequest()
	if err != nil {
		return nil, err
	}

	data := parseRequestToFormData(req)

	keys := InterfaceToFuzzResults(fieldName)
	values := InterfaceToFuzzResults(fileNames)
	if keys == nil || values == nil {
		return nil, utils.Errorf("keys or Values is empty...")
	}

	m, err := mixer.NewMixer(keys, values)
	if err != nil {
		return nil, err
	}

	var reqs []*http.Request
	for {
		pair := m.Value()
		key, value := pair[0], pair[1]
		_, _ = key, value

		mdata, err := deepCopyMultipartData(data)
		if err != nil {
			return nil, utils.Errorf("multipart data deep copy failed: %s", err)
		}
		mdata.ReplaceFile(key, value, content)
		var buffer bytes.Buffer
		w := multipart.NewWriter(&buffer)
		_ = w.SetBoundary(data.Boundary)
		err = mdata.Write(w)
		if err != nil {
			return nil, err
		}
		_ = w.Close()

		_req, err := rebuildHTTPRequest(req, int64(buffer.Len()))
		if err != nil {
			return nil, utils.Errorf("fuzz rebuild http request failed: %s", err)
		}
		_req.Body = ioutil.NopCloser(&buffer)
		_req.Header.Set("Content-Type", w.FormDataContentType())
		if _req != nil {
			reqs = append(reqs, _req)
		}

		err = m.Next()
		if err != nil {
			break
		}
	}
	return reqs, nil
}
