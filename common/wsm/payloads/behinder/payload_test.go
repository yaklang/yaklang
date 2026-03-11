package behinder

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/javaclassparser"
	"github.com/yaklang/yaklang/common/wsm/payloads"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestGetRawClassSupportsWrappedAndBareKeys(t *testing.T) {
	t.Parallel()

	hexPayload := payloads.HexPayload[ypb.ShellScript_JSP.String()][payloads.EchoGo]
	testCases := []struct {
		name string
		key  string
	}{
		{name: "bare key", key: "customEncoderFromClass"},
		{name: "wrapped key", key: "{{customEncoderFromClass}}"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			raw, err := GetRawClass(hexPayload, map[string]string{
				tc.key: "TEST-ENCODER",
			})
			require.NoError(t, err)

			clsObj, err := javaclassparser.Parse(raw)
			require.NoError(t, err)
			require.NotNil(t, clsObj.FindConstStringFromPool("TEST-ENCODER"))
		})
	}
}

func TestGetRawClassRepairsBrokenSessionBootstrap(t *testing.T) {
	t.Parallel()

	testCases := []payloads.Payload{
		payloads.BasicInfoGo,
		payloads.CmdGo,
		payloads.Payload("DatabaseGo"),
		payloads.FileOperationGo,
	}

	for _, payloadType := range testCases {
		payloadType := payloadType
		t.Run(payloadType.String(), func(t *testing.T) {
			t.Parallel()

			raw, err := GetRawClass(
				payloads.HexPayload[ypb.ShellScript_JSP.String()][payloadType],
				nil,
			)
			require.NoError(t, err)

			clsObj, err := javaclassparser.Parse(raw)
			require.NoError(t, err)

			code := fillContextCode(t, clsObj)
			require.Equal(t, -1, findSessionBootstrapGuardIndex(code, opcodeIfNull))
			require.NotEqual(t, -1, findSessionBootstrapGuardIndex(code, opcodeIfNonNull))
		})
	}
}

func fillContextCode(t *testing.T, clsObj *javaclassparser.ClassObject) []byte {
	t.Helper()

	for _, method := range clsObj.Methods {
		if classMemberName(clsObj, method.NameIndex) != "fillContext" {
			continue
		}
		for _, attr := range method.Attributes {
			codeAttr, ok := attr.(*javaclassparser.CodeAttribute)
			if ok {
				return codeAttr.Code
			}
		}
	}

	t.Fatalf("fillContext method not found")
	return nil
}
