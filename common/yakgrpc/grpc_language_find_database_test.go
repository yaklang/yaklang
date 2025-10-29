package yakgrpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func initProgram(t *testing.T, fs filesys_interface.FileSystem, opts ...ssaapi.Option) (string, func()) {
	programID := uuid.NewString()
	opts = append(opts, ssaapi.WithProgramName(programID))
	_, err := ssaapi.ParseProjectWithFS(fs, opts...)
	assert.NoErrorf(t, err, "ParseProject failed: %v", err)
	return programID, func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programID)
	}
}

func check(
	local ypb.YakClient, t *testing.T,
	programName, rawFileName string, selectedRange *memedit.Range,
	wantRanges []*memedit.Range,
) {
	frontEndFileName := fmt.Sprintf("/%s/%s", programName, rawFileName)
	rsp, err := local.YaklangLanguageFind(context.Background(), &ypb.YaklangLanguageSuggestionRequest{
		InspectType: REFERENCES,
		Range:       RangeIfToGrpcRange(selectedRange),
		FileName:    frontEndFileName,
		ProgramName: programName,
	})
	require.NoErrorf(t, err, "YaklangLanguageFind failed: %v", err)
	require.NotNil(t, rsp)
	require.Len(t, rsp.Ranges, len(wantRanges))
	for i, wantRange := range wantRanges {
		assert.Equal(t, wantRange, GrpcRangeToRangeIf(rsp.Ranges[i]))
	}
}

func TestGRPCMUSTPASS_LANGUAGE_Find_WithDB(t *testing.T) {
	local, err := NewLocalClient()
	assert.NoError(t, err)

	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/A.java", `
package find.withDB.A; 
class A {
    public void a() {
        int a = 1; // 1, println1, println2
        println1(a); // println1, 1
        if (c == 1) {
            a = 2; // 2, println2
        }
        println2(a); // println2, 1, 2
    }
}
    `)
	a1 := newRangeFromText("5:13 5:14")
	num1 := newRangeFromText("5:17 5:18")
	println1 := newRangeFromText("6:18 6:19")
	c := newRangeFromText("7:13 7:14")
	a2 := newRangeFromText("8:13 8:14")
	num2 := newRangeFromText("8:17 8:18")
	println2 := newRangeFromText("10:18 10:19")
	// only test reference
	programID, fun := initProgram(t, vf, ssaapi.WithLanguage(ssaconfig.JAVA))
	_ = fun
	defer fun()
	t.Run("find from assign by variable: a1", func(t *testing.T) {
		t.Log("find by variable a1")
		check(local, t, programID,
			"src/main/java/A.java",
			a1,
			[]*memedit.Range{a1, println1, println2},
		)
	})
	t.Run("find from assign by variable: a2", func(t *testing.T) {
		t.Log("find by variable a2")
		check(local, t, programID,
			"src/main/java/A.java",
			a2,
			[]*memedit.Range{a2, println2},
		)
	})

	t.Run("find from assign by value: num1", func(t *testing.T) {
		t.Log("find by value 1")
		check(local, t, programID,
			"src/main/java/A.java",
			num1,
			[]*memedit.Range{num1},
		)
	})
	t.Run("find from assign by value: num2", func(t *testing.T) {
		t.Log("find by value 2")
		check(local, t, programID,
			"src/main/java/A.java",
			num2,
			[]*memedit.Range{num2},
		)
	})

	t.Run("find from user: println1", func(t *testing.T) {
		t.Log("find by println1")
		check(local, t, programID,
			"src/main/java/A.java",
			println1,
			[]*memedit.Range{a1, println1, println2},
		)
	})
	t.Run("find from user: println2", func(t *testing.T) {
		t.Log("find by println2")
		check(local, t, programID,
			"src/main/java/A.java",
			println2,
			[]*memedit.Range{a1, println1, a2, println2},
		)
	})

	t.Run("find nothing else: c", func(t *testing.T) {
		t.Log("find by c")
		check(local, t, programID,
			"src/main/java/A.java",
			c,
			[]*memedit.Range{c},
		)
	})

	t.Run("error: error file", func(t *testing.T) {
		t.Log("find by errRng")
		check(local, t, programID,
			"src/this_file_not_found/b.java",
			a1,
			[]*memedit.Range{a1},
		)
	})

}
