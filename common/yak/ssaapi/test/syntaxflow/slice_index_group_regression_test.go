package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestCondition_SliceIndexShouldApplyPerCallGroup(t *testing.T) {
	vfs := filesys.NewVirtualFs()
	vfs.AddFile("unsafe.java", `
package com.example;

import java.io.File;
import java.security.KeyStore;

class Demo {
    public void test() {
        KeyStore keyStore = KeyStore.getInstance("JKS");
        keyStore.getInstance(new File("path/to/keystore"), null);
    }
}
`)

	rule := `
.getInstance?{<typeName>?{have:'java.security'}}(*<slice(index=2)> as $password);
$password?{opcode:const}?{have:'nil'} as $risk;
`

	ssatest.CheckWithFS(vfs, t, func(programs ssaapi.Programs) error {
		result, err := programs.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug())
		if err != nil {
			return err
		}
		password := result.GetValues("password")
		risk := result.GetValues("risk")
		require.Len(t, password, 1, "slice(index=2) should run per-call, not on flattened global args")
		require.Len(t, risk, 1)
		require.Equal(t, "nil", password[0].String())
		require.Equal(t, "nil", risk[0].String())
		return nil
	}, ssaapi.WithLanguage(ssaconfig.JAVA))
}
