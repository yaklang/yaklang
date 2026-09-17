package yakgrpc

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// ExportHotPatchTemplateStream exports hot patch templates (filtered by req.Filter)
// into an encrypted (optional) zip archive and streams progress back to the client.
//
// The archive layout mirrors the yak plugin export:
//   - each template is stored as "<uuid>.json" (full schema.HotPatchTemplate)
//   - a "meta.json" index lists every file together with name & type
//
// When req.Password is set the whole zip blob is encrypted with SM4-CBC and the
// output filename gains a ".enc" suffix.
func (s *Server) ExportHotPatchTemplateStream(
	req *ypb.ExportHotPatchTemplateStreamRequest,
	stream ypb.Yak_ExportHotPatchTemplateStreamServer,
) error {
	outputDir := req.GetOutputPluginDir()
	if outputDir == "" {
		outputDir = consts.GetDefaultYakitProjectsDir()
	}
	tempFilename := req.GetOutputFilename()
	if utils.StringContainsAnyOfSubString(tempFilename, []string{
		"\\", "|", "/",
	}) {
		return utils.Errorf("output filename contains invalid characters: %v (not contains \\, |, / )", tempFilename)
	}

	db := s.GetProfileDatabase().Model(&schema.HotPatchTemplate{})
	db = yakit.FilterHotPatchTemplate(db, req.GetFilter())

	client := yaklib.NewVirtualYakitClient(stream.Send)
	client.YakitSetProgress(0.1)

	var total int64
	if err := db.Count(&total).Error; err != nil {
		return err
	}
	if total <= 0 {
		return utils.Error("no hot patch template found")
	}

	step := 0.8 / float64(total)
	var buf bytes.Buffer
	zipWriter := zip.NewWriter(&buf)
	var output = make([]map[string]interface{}, 0, 64)
	var exported int64

	for template := range yakit.YieldHotPatchTemplates(stream.Context(), db) {
		select {
		case <-stream.Context().Done():
			return nil
		default:
		}
		ruid := uuid.New().String()
		filename := ruid + ".json"

		// keep all info: name / content / type / tags (+ zero out the PK)
		template.ID = 0
		templateRaw, err := json.Marshal(template)
		if err != nil {
			return utils.Wrapf(err, "marshal hot patch template failed: %v", template.Name)
		}

		fileSaver, err := zipWriter.Create(filename)
		if err != nil {
			return err
		}
		if _, err = fileSaver.Write(templateRaw); err != nil {
			log.Warnf("write hot patch template failed: %v", template.Name)
			return err
		} else if err := zipWriter.Flush(); err != nil {
			log.Warnf("flush hot patch template failed: %v", template.Name)
			return err
		}
		output = append(output, map[string]any{
			"filename": filename,
			"name":      template.Name,
			"type":      template.Type,
		})
		exported++
		client.YakitSetProgress(0.1 + step*float64(exported))
	}

	if err := zipWriter.Flush(); err != nil {
		return err
	}
	writer, err := zipWriter.Create("meta.json")
	if err != nil {
		return utils.Wrapf(err, "create hot patch template meta.json")
	}
	raw, err := json.Marshal(output)
	if err != nil {
		return utils.Wrapf(err, "marshal hot patch template meta.json")
	}
	if _, err = writer.Write(raw); err != nil {
		return utils.Wrapf(err, "write hot patch template meta.json")
	}
	zipWriter.Close()

	defer func() {
		client.YakitSetProgress(1.0)
	}()

	if req.OutputFilename == "" {
		req.OutputFilename = "hotpatch_templates_" + utils.DatetimePretty2() + ".zip"
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

	finalFilename := filepath.Join(outputDir, req.OutputFilename)
	fp, err := os.Create(finalFilename)
	if err != nil {
		return err
	}
	defer fp.Close()
	fp.Write(results)

	if req.Password == "" {
		client.YakitFile(finalFilename, "HotPatch Template Output", "Empty Password")
	} else {
		client.YakitFile(finalFilename, "HotPatch Template Output", "Encrypted with SM4")
	}

	return nil
}

// ImportHotPatchTemplateStream imports hot patch templates from an (optionally
// encrypted) zip archive produced by ExportHotPatchTemplateStream. Each template
// is upserted by (name, type) so existing entries are updated in place.
func (s *Server) ImportHotPatchTemplateStream(
	req *ypb.ImportHotPatchTemplateStreamRequest,
	stream ypb.Yak_ImportHotPatchTemplateStreamServer,
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
	var meta = make([]map[string]interface{}, 0, 0)
	if err := json.NewDecoder(metaReader).Decode(&meta); err != nil {
		return utils.Wrap(err, "decode meta.json failed")
	}
	metaReader.Close()

	client := yaklib.NewVirtualYakitClient(stream.Send)
	_ = client

	total := len(meta)
	if total <= 0 {
		return utils.Error("no hot patch template in archive")
	}
	step := 0.9 / float64(total)

	for i, r := range meta {
		select {
		case <-stream.Context().Done():
			return nil
		default:
		}
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

		var template schema.HotPatchTemplate
		if err := json.Unmarshal(raw, &template); err != nil {
			return utils.Wrapf(err, "unmarshal hot patch template failed: %v", name)
		}
		if template.Name == "" {
			log.Warnf("hot patch template name is empty: %v", name)
		}
		err = yakit.CreateOrUpdateHotPatchTemplate(
			s.GetProfileDatabase(),
			template.Name, template.Type, template.Content, []string(template.Tags),
		)
		if err != nil {
			log.Warnf("create or update hot patch template failed: %v", template.Name)
		}
		client.YakitSetProgress(0.1 + step*float64(i+1))
	}

	defer func() {
		client.YakitSetProgress(1.0)
	}()
	return nil
}
