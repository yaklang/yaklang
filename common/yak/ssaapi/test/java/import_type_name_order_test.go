package java

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestJavaExplicitImportTypeNameDoesNotBorrowImporterPackage(t *testing.T) {
	for _, workers := range []int{1, 2, 4} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("workers%d/reverse%v", workers, reverse), func(t *testing.T) {
				vf := filesys.NewVirtualFs()
				owner, importer := "00_owner.java", "99_importer.java"
				if reverse {
					owner, importer = importer, owner
				}
				vf.AddFile(owner, `package org.target; public class Owned { public int field; public int method() { return 7; } }`)
				vf.AddFile(importer, `package org.importer;
import org.target.Owned;
class Consumer {
 void consume(int number, Owned reference, Missing unknown) {
  Object casted = (Owned)number;
  var typed = reference;
  var localFallback = unknown;
 }
}`)
				programs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithMemory(), ssaconfig.WithCompileConcurrency(workers))
				require.NoError(t, err)
				result, err := programs.SyntaxFlowWithError(`
casted<typeName>?{have:'Owned'} as $castNames
typed<typeName>?{have:'Owned'} as $declaredNames
typed<fullTypeName> as $fullNames
localFallback<fullTypeName> as $fallbackNames`)
				require.NoError(t, err)
				for _, binding := range []string{"castNames", "declaredNames"} {
					values := result.GetValues(binding)
					require.Len(t, values, 2, "%s: %s", binding, values.String())
					require.NotContains(t, values.String(), "org.importer.Owned")
					require.Contains(t, values.String(), "org.target.Owned")
				}
				require.NotContains(t, result.GetValues("fullNames").String(), "org.importer.Owned")
				require.Contains(t, result.GetValues("fallbackNames").String(), "org.importer.Missing", "unimported unknown types retain their local fallback")
			})
		}
	}
}
