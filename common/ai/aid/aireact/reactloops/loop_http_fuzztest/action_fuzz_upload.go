package loop_http_fuzztest

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

var fuzzUploadAction = func(r aicommon.AIInvokeRuntime) reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopActionWithStreamField(
		"fuzz_upload",
		"Fuzz multipart file upload requests. Use this for file upload endpoints to test filename bypass, multipart field tampering, and small file content probes. Prefer this over fuzz_body raw or generate_and_send_packet for multipart/form-data uploads.",
		[]aitool.ToolOption{
			aitool.WithStringParam("upload_type", aitool.WithParam_Description("Upload fuzz type: file_name (test filenames), file_content (replace file body with small template/profile), file_content_type (not yet supported, use patch_http_request), multipart_field (fuzz non-file form fields)."), aitool.WithParam_Required(true)),
			aitool.WithStringParam("field_name", aitool.WithParam_Description("Multipart field name to fuzz."), aitool.WithParam_Required(true)),
			aitool.WithStringArrayParam("file_names", aitool.WithParam_Description("Filenames to test. Required for file_name and file_content. Supports fuzztag for batch generation.")),
			aitool.WithStringArrayParam("field_values", aitool.WithParam_Description("Values to test for multipart_field type. Supports fuzztag.")),
			aitool.WithStringParam("content_template", aitool.WithParam_Description("Small inline file content template for file_content. Keep it small; do not embed large files.")),
			aitool.WithStringParam("content_profile", aitool.WithParam_Description("Built-in small content profile for file_content: empty, text, svg_xss, php_probe, polyglot_jpeg_php.")),
			aitool.WithStringParam("file_resource_id", aitool.WithParam_Description("Reuse an externalized upload file resource ID from upload_request_summary instead of generating new content.")),
			aitool.WithStringArrayParam("content_type_values", aitool.WithParam_Description("Content-Type values for file_content_type (phase 1: prefer patch_http_request).")),
			aitool.WithStringParam("reason", aitool.WithParam_Description("请用中文说明测试目的、怀疑漏洞和安全边界。不要上传 webshell 或破坏性 payload。")),
		},
		[]*reactloops.LoopStreamField{
			{FieldName: "reason", AINodeId: "thought", IsSystem: true},
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			uploadType := strings.TrimSpace(action.GetString("upload_type"))
			fieldName := strings.TrimSpace(action.GetString("field_name"))
			if uploadType == "" {
				return fmt.Errorf("upload_type parameter is required")
			}
			if fieldName == "" {
				return fmt.Errorf("field_name parameter is required")
			}
			switch uploadType {
			case "file_name", "file_content", "multipart_field", "file_content_type":
			default:
				return fmt.Errorf("upload_type must be one of: file_name, file_content, multipart_field, file_content_type")
			}
			switch uploadType {
			case "file_name", "file_content":
				if len(action.GetStringSlice("file_names")) == 0 {
					return fmt.Errorf("file_names is required for upload_type=%s", uploadType)
				}
				if uploadType == "file_content" &&
					strings.TrimSpace(action.GetString("content_template")) == "" &&
					strings.TrimSpace(action.GetString("content_profile")) == "" &&
					strings.TrimSpace(action.GetString("file_resource_id")) == "" {
					return fmt.Errorf("file_content requires content_template, content_profile, or file_resource_id")
				}
			case "multipart_field":
				if len(action.GetStringSlice("field_values")) == 0 {
					return fmt.Errorf("field_values is required for upload_type=multipart_field")
				}
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, operator *reactloops.LoopActionHandlerOperator) {
			uploadType := strings.TrimSpace(action.GetString("upload_type"))
			fieldName := strings.TrimSpace(action.GetString("field_name"))
			fileNames := action.GetStringSlice("file_names")
			fieldValues := action.GetStringSlice("field_values")
			contentTemplate := action.GetString("content_template")
			contentProfile := action.GetString("content_profile")
			fileResourceID := action.GetString("file_resource_id")
			reason := action.GetString("reason")

			log.Infof("fuzz_upload action: type=%s field=%s reason=%s", uploadType, fieldName, reason)

			fuzzReq, err := getFuzzRequest(loop)
			if err != nil {
				operator.Fail(err)
				return
			}
			if !lowhttp.IsMultipartFormDataRequest(fuzzReq.GetBytes()) {
				operator.Fail(fmt.Errorf("current request is not multipart/form-data; use fuzz_body or patch_http_request instead"))
				return
			}

			var fuzzResult mutate.FuzzHTTPRequestIf
			switch uploadType {
			case "file_name":
				fuzzResult = fuzzReq.FuzzUploadFileName(fieldName, fileNames)
			case "multipart_field":
				fuzzResult = fuzzReq.FuzzUploadKVPair(fieldName, fieldValues)
			case "file_content":
				contentBytes, contentErr := resolveUploadContentBytes(contentTemplate, contentProfile, fileResourceID, loop)
				if contentErr != nil {
					operator.Fail(contentErr)
					return
				}
				if len(contentBytes) > loopHTTPUploadPartExternalizeThreshold {
					operator.Fail(fmt.Errorf("file_content template/profile exceeds %d bytes; use file_resource_id to reuse externalized files", loopHTTPUploadPartExternalizeThreshold))
					return
				}
				fuzzResult = fuzzReq.FuzzUploadFile(fieldName, fileNames, contentBytes)
			case "file_content_type":
				operator.Fail(fmt.Errorf("file_content_type is not supported in fuzz_upload yet; use patch_http_request to adjust Content-Type"))
				return
			default:
				operator.Fail(fmt.Errorf("unknown upload_type: %s", uploadType))
				return
			}

			paramSummary := fmt.Sprintf("upload_type=%s; field_name=%s; file_names=%v; field_values=%v; content_profile=%s; file_resource_id=%s; reason=%s",
				uploadType, fieldName, fileNames, fieldValues, contentProfile, fileResourceID, reason)
			diffResult, verifyResult, err := executeFuzzAndCompare(loop, fuzzResult, "fuzz_upload", paramSummary, action)
			if err != nil {
				operator.Fail(err)
				return
			}

			r.AddToTimeline("fuzz_upload", fmt.Sprintf("Tested upload (%s) field %s\n%s", uploadType, fieldName, buildFuzzTimelineSummary(diffResult)))
			applyFuzzVerificationOutcome(loop, operator, diffResult, verifyResult)
		},
	)
}
