package java

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

const riskHashTestRule = `
	.getInstance?{<typeName>?{have:'java.security'}}(*<slice(index=1)> as $algorithm);
	$algorithm #{
		until:` + "`*?{ opcode:const && have:/MD2|MD4|MD5|SHA(-)?1|SHA(-)?0|RIPEMD160|^SHA$/}`" + `,
		exclude:` + "`*?{any:'SHA256','SHA384','SHA512'}`" + `
	}-> as $sink;
	alert $sink for {
		title: "Check Java java.security use of broken or risky hash algorithm",
		level: "low",
		risk: "不安全加密算法"
	}
`

const safeJavaApp = `package com.example;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

public class App {
    public String digest(String input) throws Exception {
        MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
        return bytesToHex(sha256.digest(input.getBytes(StandardCharsets.UTF_8)));
    }

    private String bytesToHex(byte[] data) {
        StringBuilder builder = new StringBuilder();
        for (byte b : data) {
            builder.append(String.format("%02x", b));
        }
        return builder.toString();
    }
}`

const riskyJavaRemovedDigest = `package com.example;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

public class RemovedDigest {
    public String removedDigest(String input) throws Exception {
        MessageDigest md5 = MessageDigest.getInstance("MD5");
        return bytesToHex(md5.digest(input.getBytes(StandardCharsets.UTF_8)));
    }

    private String bytesToHex(byte[] data) {
        StringBuilder builder = new StringBuilder();
        for (byte b : data) {
            builder.append(String.format("%02x", b));
        }
        return builder.toString();
    }
}`

const riskyJavaApp = `package com.example;

import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;

public class App {
    public String weakDigest(String input) throws Exception {
        MessageDigest md5 = MessageDigest.getInstance("MD5");
        return bytesToHex(md5.digest(input.getBytes(StandardCharsets.UTF_8)));
    }

    public String legacyDigest(String input) throws Exception {
        MessageDigest sha1 = MessageDigest.getInstance("SHA-1");
        return bytesToHex(sha1.digest(input.getBytes(StandardCharsets.UTF_8)));
    }

    private String bytesToHex(byte[] data) {
        StringBuilder builder = new StringBuilder();
        for (byte b : data) {
            builder.append(String.format("%02x", b));
        }
        return builder.toString();
    }
}`

func TestRiskFeatureHash_JavaMethodNameStableAcrossProgramNames(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/com/example/App.java", riskyJavaApp)

	programName1 := "java-risk-hash-1-" + uuid.NewString()
	programName2 := "java-risk-hash-2-" + uuid.NewString()

	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName1)
		ssadb.DeleteProgram(ssadb.GetDB(), programName2)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName1, programName2},
		})
	})

	collectRisks := func(programName string) (hashes, names []string) {
		progs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(programName))
		require.NoError(t, err)
		require.NotEmpty(t, progs)
		result, err := progs[0].SyntaxFlowWithError(riskHashTestRule, ssaapi.QueryWithEnableDebug(true))
		require.NoError(t, err)
		_, err = result.Save(schema.SFResultKindDebug)
		require.NoError(t, err)

		time.Sleep(100 * time.Millisecond)
		_, risks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName},
		}, nil)
		require.NoError(t, err)
		require.Len(t, risks, 2)
		for _, r := range risks {
			hashes = append(hashes, r.RiskFeatureHash)
			names = append(names, r.FunctionName)
		}
		return
	}

	hashes1, names1 := collectRisks(programName1)
	hashes2, names2 := collectRisks(programName2)
	require.ElementsMatch(t, hashes1, hashes2, "risk_feature_hash should be stable across program names")
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, names1)
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, names2)
}

func TestRiskFeatureHash_JavaIncrementalOverlay(t *testing.T) {
	var compileHashes []string

	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"src/main/java/com/example/App.java": safeJavaApp,
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"src/main/java/com/example/App.java": riskyJavaApp,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				result, err := overlay.SyntaxFlowWithError(riskHashTestRule, ssaapi.QueryWithEnableDebug(true))
				require.NoError(t, err)

				sinkValues := result.GetAlertValue("sink")
				require.Len(t, sinkValues, 2, "overlay should find 2 risky hash usages")

				err = result.CreateRisk()
				require.NoError(t, err)
				require.Equal(t, 2, result.RiskCount())

				var hashes, functionNames []string
				for risk := range result.YieldRisk() {
					hashes = append(hashes, risk.RiskFeatureHash)
					functionNames = append(functionNames, risk.FunctionName)
				}
				require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, functionNames)

				if stage == ssatest.IncrementalCheckStageCompile {
					compileHashes = hashes
				} else {
					require.ElementsMatch(t, compileHashes, hashes,
						"risk feature hashes should be stable between compile and DB reload")
				}
			},
		},
	)
}

func TestRiskFeatureHash_JavaIncrementalOverlayScanKind(t *testing.T) {
	ssatest.CheckIncrementalProgram(t,
		ssatest.IncrementalStep{
			Files: map[string]string{
				"src/main/java/com/example/App.java": safeJavaApp,
			},
		},
		ssatest.IncrementalStep{
			Files: map[string]string{
				"src/main/java/com/example/App.java": riskyJavaApp,
			},
			Check: func(overlay *ssaapi.ProgramOverLay, stage ssatest.IncrementalCheckStage) {
				result, err := overlay.SyntaxFlowWithError(riskHashTestRule, ssaapi.QueryWithEnableDebug(true))
				require.NoError(t, err)

				sinkValues := result.GetAlertValue("sink")
				require.Len(t, sinkValues, 2)

				err = result.CreateRisk()
				require.NoError(t, err)
				require.Equal(t, 2, result.RiskCount())

				functionNames := make([]string, 0, 2)
				for risk := range result.YieldRisk() {
					functionNames = append(functionNames, risk.FunctionName)
				}
				require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, functionNames)
			},
		},
	)
}

func TestRiskFeatureHash_JavaDeleteOnlyIncrementalScanStreamParts(t *testing.T) {
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("src/main/java/com/example/App.java", riskyJavaApp)
	baseFS.AddFile("src/main/java/com/example/RemovedDigest.java", riskyJavaRemovedDigest)

	deleteOnlyFS := filesys.NewVirtualFs()
	deleteOnlyFS.AddFile("src/main/java/com/example/App.java", riskyJavaApp)

	baseProgramName := "java-delete-only-base-" + uuid.NewString()
	incrementalProgramName := "java-delete-only-incremental-" + uuid.NewString()

	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), incrementalProgramName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{baseProgramName, incrementalProgramName},
		})
	})

	_, err := ssaapi.ParseProjectWithFS(baseFS, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(baseProgramName))
	require.NoError(t, err)
	_, err = ssaapi.ParseProjectWithIncrementalCompile(
		deleteOnlyFS, baseProgramName, incrementalProgramName, ssaconfig.JAVA,
	)
	require.NoError(t, err)

	incrementalProg, err := ssaapi.FromDatabase(incrementalProgramName)
	require.NoError(t, err)
	overlay := incrementalProg.GetOverlay()
	require.NotNil(t, overlay)

	result, err := overlay.SyntaxFlowWithError(riskHashTestRule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)

	sinkValues := result.GetAlertValue("sink")
	require.Len(t, sinkValues, 2, "delete-only incremental should only see App.java risks, not RemovedDigest.java")

	err = result.CreateRisk()
	require.NoError(t, err)
	require.Equal(t, 2, result.RiskCount())

	functionNames := make([]string, 0, 2)
	for risk := range result.YieldRisk() {
		functionNames = append(functionNames, risk.FunctionName)
	}
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, functionNames)
}
