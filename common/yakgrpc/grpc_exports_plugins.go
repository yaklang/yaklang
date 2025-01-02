package yakgrpc

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io"
	"os"
	"path/filepath"
)

func (s *Server) ImportYakScriptStream(
	req *ypb.ImportYakScriptStreamRequest,
	stream ypb.Yak_ImportYakScriptStreamServer,
) error {
	var err error

	data := req.GetData()
	if len(data) <= 0 {
		data, err = os.ReadFile(req.GetFilename())
		if err != nil {
			return utils.Wrapf(err, "read file failed: %v", req.GetFilename())
		}
	}

	var zipReader *zip.Reader
	if req.GetPassword() == "" {
		zipReader, err = zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return utils.Wrap(err, "create zip reader failed, do we need password maybe!")
		}
	} else {
		results, err := codec.SM4DecryptCBCWithPKCSPadding(
			codec.PKCS7Padding([]byte(req.GetPassword())),
			data,
			codec.PKCS7Padding([]byte(req.GetPassword())),
		)
		if err != nil {
			return utils.Wrapf(err, "decrypt file failed: %v", req.GetFilename())
		}
		zipReader, err = zip.NewReader(bytes.NewReader(results), int64(len(results)))
		if err != nil {
			return utils.Wrap(err, "create zip reader failed, file is decrypted but broken")
		}
	}

	if zipReader == nil {
		return utils.Errorf("zip reader is nil")
	}

	metaReader, err := zipReader.Open("meta.json")
	if err != nil {
		return utils.Wrap(err, "open meta.json failed")
	}
	var results = make([]map[string]interface{}, 0, 0)
	if err := json.NewDecoder(metaReader).Decode(&results); err != nil {
		return utils.Wrap(err, "decode meta.json failed")
	}
	metaReader.Close()

	client := yaklib.NewVirtualYakitClient(stream.Send)
	_ = client

	for _, r := range results {
		name, ok := r["filename"]
		if !ok {
			continue
		}
		fp, err := zipReader.Open(fmt.Sprint(name))
		if err != nil {
			return utils.Wrapf(err, "open file failed: %v", name)
		}
		raw, _ := io.ReadAll(fp)
		fp.Close()
		var script schema.YakScript
		if err := json.Unmarshal(raw, &script); err != nil {
			return utils.Wrapf(err, "unmarshal yakit script failed: %v", name)
		}
		if script.ScriptName == "" {
			log.Warnf("yakit script name is empty: %v", name)
		}
		err = yakit.CreateOrUpdateYakScriptByName(consts.GetGormProfileDatabase(), script.ScriptName, &script)
		if err != nil {
			log.Warnf("create or update yakit script failed: %v", script.ScriptName)
		}
	}
	return nil
}

func (s *Server) ExportYakScriptStream(
	req *ypb.ExportYakScriptStreamRequest,
	stream ypb.Yak_ExportYakScriptStreamServer,
) error {
	projectsDir := consts.GetDefaultYakitProjectsDir()
	tempFilename := req.GetOutputFilename()
	if utils.StringContainsAnyOfSubString(tempFilename, []string{
		"\\", "|", "/",
	}) {
		return utils.Errorf("output filename contains invalid characters: %v (not contains \\, |, / )", tempFilename)
	}

	db := consts.GetGormProfileDatabase().Model(&schema.YakScript{})
	db = yakit.FilterYakScript(db, req.GetFilter())

	client := yaklib.NewVirtualYakitClient(stream.Send)
	client.YakitSetProgress(0.1)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return err
	}

	if total <= 0 {
		return utils.Error("no yakit script found")
	}

	step := 0.8 / float64(total)
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	var output = make([]map[string]interface{}, 0, 64)
	for script := range yakit.YieldYakScripts(db, stream.Context()) {
		select {
		case <-stream.Context().Done():
			return nil
		default:
		}
		ruid := uuid.New().String()
		filename := ruid + ".json"

		script.ID = 0
		scriptRaw, err := json.Marshal(script)
		if err != nil {
			return utils.Wrapf(err, "marshal yakit script failed: %v", script.ScriptName)
		}

		fileSaver, err := zipWriter.Create(filename)
		if err != nil {
			return err
		}
		_, err = fileSaver.Write(scriptRaw)
		if err != nil {
			log.Warnf("write yakit script failed: %v", script.ScriptName)
			return err
		} else if err := zipWriter.Flush(); err != nil {
			log.Warnf("flush yakit script failed: %v", script.ScriptName)
			return err
		}
		output = append(output, map[string]any{
			"filename":    filename,
			"script_name": script.ScriptName,
		})
		client.YakitSetProgress(step + 0.1)
	}
	err := zipWriter.Flush()
	if err != nil {
		return err
	}
	writer, err := zipWriter.Create("meta.json")
	if err != nil {
		return utils.Wrapf(err, "create yakit plugin meta.json")
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return utils.Wrapf(err, "marshal yakit plugin meta.json")
	}
	_, err = writer.Write(raw)
	if err != nil {
		return utils.Wrapf(err, "write yakit plugin meta.json")
	}
	zipWriter.Close()
	defer func() {
		client.YakitSetProgress(1.0)
	}()
	if req.OutputFilename == "" {
		req.OutputFilename = "yakit_plugins_" + utils.DatetimePretty2() + ".zip"
	}

	if filepath.Ext(req.OutputFilename) != ".zip" { // try fix extension
		req.OutputFilename += ".zip"
	}

	var results []byte = buf.Bytes()
	if req.Password != "" {
		req.OutputFilename += ".enc"
		results, err = codec.SM4EncryptCBCWithPKCSPadding(
			codec.PKCS7Padding([]byte(req.Password)),
			results, codec.PKCS7Padding([]byte(req.Password)),
		)
		if err != nil {
			return err
		}
	}

	finalFilename := filepath.Join(projectsDir, req.OutputFilename)
	fp, err := os.Create(finalFilename)
	if err != nil {
		return err
	}
	defer fp.Close()
	fp.Write(results)

	if req.Password == "" {
		client.YakitFile(finalFilename, "Yakit Plugin Output", "Empty Password")
	} else {
		client.YakitFile(finalFilename, "Yakit Plugin Output", "Encrypted with SM4")
	}

	return nil
}
