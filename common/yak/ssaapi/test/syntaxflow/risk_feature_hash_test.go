package syntaxflow

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRiskFeatureHash_Comprehensive(t *testing.T) {
	testCases := []struct {
		name        string
		code1       string // 第一次扫描的代码
		code2       string // 第二次扫描的代码
		fileName1   string // 第一次扫描的文件名
		fileName2   string // 第二次扫描的文件名
		rule        string // SyntaxFlow 规则
		expectSame  bool   // 是否期望 RiskFeatureHash 相同
		description string // 测试场景描述
	}{
		{
			name: "相同代码相同文件名",
			code1: `
a = source() 
sink(a)
			`,
			code2: `
a = source() 
sink(a)
			`,
			fileName1: "test.yak",
			fileName2: "test.yak",
			rule: `
sink as $sink
$sink #-> as $result
alert $result for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	level:"high"
}
			`,
			expectSame:  true,
			description: "完全相同的代码和文件名应该产生相同的 RiskFeatureHash",
		},
		{
			name: "相同代码不同文件名",
			code1: `
a = source() 
sink(a)
			`,
			code2: `
a = source() 
sink(a)
			`,
			fileName1: "test1.yak",
			fileName2: "test2.yak",
			rule: `
sink as $sink
$sink #-> as $result
alert $result for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	level:"high"
}
			`,
			expectSame:  true,
			description: "相同的代码逻辑在不同文件中应该产生相同的 RiskFeatureHash",
		},
		{
			name: "不同变量名相同逻辑",
			code1: `
a = source() 
sink(a)
			`,
			code2: `
b = source() 
sink(b)
			`,
			fileName1: "test.yak",
			fileName2: "test.yak",
			rule: `
sink as $sink
$sink #-> as $result
alert $result for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	level:"high"
}
			`,
			expectSame:  true,
			description: "不同变量名但相同的代码逻辑应该产生相同的 RiskFeatureHash",
		},
		// TODO：这里漏洞逻辑不同，RiskFeatureHash 也应该不同，但是目前报的都是source，导致结果是一样的
		//		{
		//			name: "不同代码逻辑",
		//			code1: `
		//a = source()
		//sink(a)
		//			`,
		//			code2: `
		//c = source()
		//d = transform(c)
		//sink(d)
		//			`,
		//			fileName1: "test.yak",
		//			fileName2: "test.yak",
		//			rule: `
		//sink as $sink
		//$sink #-> as $result
		//alert $result for {
		//	desc: "Source-Sink vulnerability"
		//	Title:"SQL Injection"
		//	level:"high"
		//}
		//			`,
		//			expectSame:  false,
		//			description: "不同的代码逻辑应该产生不同的 RiskFeatureHash",
		//		},
		{
			name: "相同代码不同函数",
			code1: `
func test1() {
	a = source() 
	sink(a)
}
			`,
			code2: `
func test2() {
	a = source() 
	sink(a)
}
			`,
			fileName1: "test.yak",
			fileName2: "test.yak",
			rule: `
sink as $sink
$sink #-> as $result
alert $result for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	level:"high"
}
			`,
			expectSame:  false,
			description: "相同代码但在不同函数中应该产生不同的 RiskFeatureHash",
		},
		{
			name: "添加注释的相同代码",
			code1: `
a = source() 
sink(a)
			`,
			code2: `
// 这是一个注释
a = source() 
sink(a) // 另一个注释
			`,
			fileName1: "test.yak",
			fileName2: "test.yak",
			rule: `
sink as $sink
$sink #-> as $result
alert $result for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	level:"high"
}
			`,
			expectSame:  true,
			description: "添加注释的相同代码逻辑应该产生相同的 RiskFeatureHash",
		},
		{
			name: "空格和格式差异",
			code1: `
a = source()
sink(a)
			`,
			code2: `
a=source()
sink(  a  )
			`,
			fileName1: "test.yak",
			fileName2: "test.yak",
			rule: `
sink as $sink
$sink #-> as $result
alert $result for {
	desc: "Source-Sink vulnerability"
	Title:"SQL Injection"
	level:"high"
}
			`,
			expectSame:  true,
			description: "空格和格式差异不应该影响 RiskFeatureHash",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			log.Infof("=== 测试场景: %s ===", tc.description)

			// 生成唯一的程序名
			programName1 := "comprehensive_test_1_" + uuid.New().String()
			programName2 := "comprehensive_test_2_" + uuid.New().String()

			// 创建第一个虚拟文件系统
			vf1 := filesys.NewVirtualFs()
			vf1.AddFile(tc.fileName1, tc.code1)

			// 第一次扫描
			programs1, err := ssaapi.ParseProjectWithFS(vf1, ssaapi.WithLanguage(consts.Yak), ssaapi.WithProgramName(programName1))
			require.NoError(t, err)
			require.NotEmpty(t, programs1)
			prog1 := programs1[0]

			t.Cleanup(func() {
				ssadb.DeleteProgram(ssadb.GetDB(), programName1)
				yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
					ProgramName: []string{programName1, programName2},
				})
			})

			result1, err := prog1.SyntaxFlowWithError(tc.rule, ssaapi.QueryWithEnableDebug(true))
			require.NoError(t, err)
			_, err = result1.Save(schema.SFResultKindDebug)
			require.NoError(t, err)

			// 创建第二个虚拟文件系统
			vf2 := filesys.NewVirtualFs()
			vf2.AddFile(tc.fileName2, tc.code2)

			// 第二次扫描
			programs2, err := ssaapi.ParseProjectWithFS(vf2, ssaapi.WithLanguage(consts.Yak), ssaapi.WithProgramName(programName2))
			require.NoError(t, err)
			require.NotEmpty(t, programs2)
			prog2 := programs2[0]

			t.Cleanup(func() {
				ssadb.DeleteProgram(ssadb.GetDB(), programName2)
			})

			result2, err := prog2.SyntaxFlowWithError(tc.rule, ssaapi.QueryWithEnableDebug(true))
			require.NoError(t, err)
			_, err = result2.Save(schema.SFResultKindDebug)
			require.NoError(t, err)

			// 等待一下确保数据库操作完成
			time.Sleep(100 * time.Millisecond)

			// 查询生成的 Risk 记录
			_, risks1, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
				ProgramName: []string{programName1},
			}, nil)
			require.NoError(t, err)

			_, risks2, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
				ProgramName: []string{programName2},
			}, nil)
			require.NoError(t, err)

			// 验证是否生成了 Risk
			require.NotEmpty(t, risks1, "第一次扫描应该生成 Risk 记录")
			require.NotEmpty(t, risks2, "第二次扫描应该生成 Risk 记录")

			risk1 := risks1[0]
			risk2 := risks2[0]

			log.Infof("Risk1: ID=%d, ProgramName=%s, FunctionName=%s, RiskFeatureHash=%s, ",
				risk1.ID, risk1.ProgramName, risk1.FunctionName, risk1.RiskFeatureHash)
			log.Infof("Risk2: ID=%d, ProgramName=%s, FunctionName=%s, RiskFeatureHash=%s,",
				risk2.ID, risk2.ProgramName, risk2.FunctionName, risk2.RiskFeatureHash)

			// 验证基本属性
			require.NotEmpty(t, risk1.RiskFeatureHash, "Risk1 应该有 RiskFeatureHash")
			require.NotEmpty(t, risk2.RiskFeatureHash, "Risk2 应该有 RiskFeatureHash")

			// 验证 RiskFeatureHash 是否符合预期
			if tc.expectSame {
				require.Equal(t, risk1.RiskFeatureHash, risk2.RiskFeatureHash,
					"测试场景 '%s': RiskFeatureHash 应该相同", tc.description)
			} else {
				require.NotEqual(t, risk1.RiskFeatureHash, risk2.RiskFeatureHash,
					"测试场景 '%s': RiskFeatureHash 应该不同", tc.description)
			}

			// 验证 ID 和程序名总是不同的
			require.NotEqual(t, risk1.ID, risk2.ID, "Risk ID 应该不同")
			require.NotEqual(t, risk1.ProgramName, risk2.ProgramName, "程序名应该不同")
		})
	}
}
