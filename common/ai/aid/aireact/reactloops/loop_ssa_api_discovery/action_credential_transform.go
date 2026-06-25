package loop_ssa_api_discovery

import (
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/utils"
)

func buildDiscoveryTransformCredential() reactloops.ReActLoopOption {
	return reactloops.WithRegisterLoopAction(
		"transform_credential",
		"Hash/encode plaintext for login payloads (md5/sha256/sha512/base64/url/hmac). Pure Go; use after reading login JS/controller.",
		[]aitool.ToolOption{
			aitool.WithStringParam("algorithm",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("md5|sha1|sha256|sha512|base64|base64url|url|hex|hmac-md5|hmac-sha1|hmac-sha256|hmac-sha512"),
			),
			aitool.WithStringParam("input",
				aitool.WithParam_Required(true),
				aitool.WithParam_Description("plaintext password or value to transform"),
			),
			aitool.WithStringParam("salt", aitool.WithParam_Description("optional salt applied before hash")),
			aitool.WithStringParam("salt-position", aitool.WithParam_Description("none|prefix|suffix")),
			aitool.WithStringParam("key", aitool.WithParam_Description("HMAC key when algorithm is hmac-*")),
			aitool.WithBoolParam("uppercase", aitool.WithParam_Description("uppercase hex/hash output")),
			aitool.WithStringParam("output-format", aitool.WithParam_Description("hex|base64|lower|upper")),
		},
		func(l *reactloops.ReActLoop, action *aicommon.Action) error {
			if action.GetString("algorithm") == "" || action.GetString("input") == "" {
				return utils.Error("algorithm and input are required")
			}
			return nil
		},
		func(loop *reactloops.ReActLoop, action *aicommon.Action, op *reactloops.LoopActionHandlerOperator) {
			content, err := credentialTransformJSON(
				action.GetString("algorithm"),
				action.GetString("input"),
				action.GetString("salt"),
				action.GetString("salt-position"),
				action.GetString("key"),
				action.GetString("output-format"),
				action.GetBool("uppercase", false),
			)
			if err != nil {
				op.Feedback(err.Error())
				op.Continue()
				return
			}
			if res, terr := transformCredentialGoParams(
				action.GetString("algorithm"),
				action.GetString("input"),
				action.GetString("salt"),
				action.GetString("salt-position"),
				action.GetString("key"),
				action.GetString("output-format"),
				action.GetBool("uppercase", false),
			); terr == nil {
				recordCredentialTransform(loop, res)
			}
			op.Feedback(utils.ShrinkString(content, 8000))
			op.Continue()
		},
	)
}
