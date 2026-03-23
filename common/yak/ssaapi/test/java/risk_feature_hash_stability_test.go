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
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRiskFeatureHash_JavaMethodNameStableAcrossProgramNames(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/com/example/App.java", `package com.example;

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
}`)

	rule := `
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

	programName1 := "java-risk-hash-1-" + uuid.NewString()
	programName2 := "java-risk-hash-2-" + uuid.NewString()

	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName1)
		ssadb.DeleteProgram(ssadb.GetDB(), programName2)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName1, programName2},
		})
	})

	progs1, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(programName1))
	require.NoError(t, err)
	require.NotEmpty(t, progs1)
	result1, err := progs1[0].SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)
	_, err = result1.Save(schema.SFResultKindDebug)
	require.NoError(t, err)

	progs2, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(programName2))
	require.NoError(t, err)
	require.NotEmpty(t, progs2)
	result2, err := progs2[0].SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)
	_, err = result2.Save(schema.SFResultKindDebug)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	_, risks1, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{programName1},
	}, nil)
	require.NoError(t, err)

	_, risks2, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{programName2},
	}, nil)
	require.NoError(t, err)

	require.Len(t, risks1, 2)
	require.Len(t, risks2, 2)

	hashes1 := []string{risks1[0].RiskFeatureHash, risks1[1].RiskFeatureHash}
	hashes2 := []string{risks2[0].RiskFeatureHash, risks2[1].RiskFeatureHash}
	require.ElementsMatch(t, hashes1, hashes2, "Java risk_feature_hash should stay stable across recompiles with different program names")

	functionNames1 := []string{risks1[0].FunctionName, risks1[1].FunctionName}
	functionNames2 := []string{risks2[0].FunctionName, risks2[1].FunctionName}
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, functionNames1)
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, functionNames2)
}

func TestRiskFeatureHash_JavaMethodNameStableAfterFromDatabase(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/main/java/com/example/App.java", `package com.example;

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
}`)

	rule := `
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

	programName1 := "java-db-risk-hash-1-" + uuid.NewString()
	programName2 := "java-db-risk-hash-2-" + uuid.NewString()

	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), programName1)
		ssadb.DeleteProgram(ssadb.GetDB(), programName2)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{programName1, programName2},
		})
	})

	_, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(programName1))
	require.NoError(t, err)
	_, err = ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(programName2))
	require.NoError(t, err)

	prog1, err := ssaapi.FromDatabase(programName1)
	require.NoError(t, err)
	prog2, err := ssaapi.FromDatabase(programName2)
	require.NoError(t, err)

	result1, err := prog1.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)
	_, err = result1.Save(schema.SFResultKindDebug)
	require.NoError(t, err)

	result2, err := prog2.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)
	_, err = result2.Save(schema.SFResultKindDebug)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	_, risks1, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{programName1},
	}, nil)
	require.NoError(t, err)

	_, risks2, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{programName2},
	}, nil)
	require.NoError(t, err)

	require.Len(t, risks1, 2)
	require.Len(t, risks2, 2)

	hashes1 := []string{risks1[0].RiskFeatureHash, risks1[1].RiskFeatureHash}
	hashes2 := []string{risks2[0].RiskFeatureHash, risks2[1].RiskFeatureHash}
	require.ElementsMatch(t, hashes1, hashes2, "Java DB-loaded risk_feature_hash should stay stable across program names")

	functionNames1 := []string{risks1[0].FunctionName, risks1[1].FunctionName}
	functionNames2 := []string{risks2[0].FunctionName, risks2[1].FunctionName}
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, functionNames1)
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, functionNames2)
}

func TestRiskFeatureHash_JavaStableBetweenFullAndIncrementalOverlay(t *testing.T) {
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("src/main/java/com/example/App.java", `package com.example;

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
}`)

	riskyFS := filesys.NewVirtualFs()
	riskyFS.AddFile("src/main/java/com/example/App.java", `package com.example;

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
}`)

	rule := `
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

	baseProgramName := "java-overlay-base-" + uuid.NewString()
	fullProgramName := "java-overlay-full-" + uuid.NewString()
	incrementalProgramName := "java-overlay-incremental-" + uuid.NewString()

	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), fullProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), incrementalProgramName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{baseProgramName, fullProgramName, incrementalProgramName},
		})
	})

	_, err := ssaapi.ParseProjectWithFS(baseFS, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(baseProgramName))
	require.NoError(t, err)

	_, err = ssaapi.ParseProjectWithFS(riskyFS, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(fullProgramName))
	require.NoError(t, err)

	_, err = ssaapi.ParseProjectWithFS(
		riskyFS,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(incrementalProgramName),
		ssaapi.WithBaseProgramName(baseProgramName),
	)
	require.NoError(t, err)

	fullProg, err := ssaapi.FromDatabase(fullProgramName)
	require.NoError(t, err)
	incrementalProg, err := ssaapi.FromDatabase(incrementalProgramName)
	require.NoError(t, err)

	fullResult, err := fullProg.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)
	_, err = fullResult.Save(schema.SFResultKindDebug)
	require.NoError(t, err)

	incrementalResult, err := incrementalProg.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)
	_, err = incrementalResult.Save(schema.SFResultKindDebug)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	_, fullRisks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{fullProgramName},
	}, nil)
	require.NoError(t, err)

	_, incrementalRisks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{incrementalProgramName},
	}, nil)
	require.NoError(t, err)

	require.Len(t, fullRisks, 2)
	require.Len(t, incrementalRisks, 2)

	fullHashes := []string{fullRisks[0].RiskFeatureHash, fullRisks[1].RiskFeatureHash}
	incrementalHashes := []string{incrementalRisks[0].RiskFeatureHash, incrementalRisks[1].RiskFeatureHash}
	require.ElementsMatch(t, fullHashes, incrementalHashes, "full and incremental overlay risk_feature_hash should match")

	fullFunctionNames := []string{fullRisks[0].FunctionName, fullRisks[1].FunctionName}
	incrementalFunctionNames := []string{incrementalRisks[0].FunctionName, incrementalRisks[1].FunctionName}
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, fullFunctionNames)
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, incrementalFunctionNames)
}

func TestRiskFeatureHash_JavaStableBetweenFullAndIncrementalOverlay_ScanSaveKind(t *testing.T) {
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("src/main/java/com/example/App.java", `package com.example;

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
}`)

	riskyFS := filesys.NewVirtualFs()
	riskyFS.AddFile("src/main/java/com/example/App.java", `package com.example;

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
}`)

	rule := `
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

	baseProgramName := "java-overlay-scan-base-" + uuid.NewString()
	fullProgramName := "java-overlay-scan-full-" + uuid.NewString()
	incrementalProgramName := "java-overlay-scan-incremental-" + uuid.NewString()

	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), fullProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), incrementalProgramName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{baseProgramName, fullProgramName, incrementalProgramName},
		})
	})

	_, err := ssaapi.ParseProjectWithFS(baseFS, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(baseProgramName))
	require.NoError(t, err)
	_, err = ssaapi.ParseProjectWithFS(riskyFS, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(fullProgramName))
	require.NoError(t, err)
	_, err = ssaapi.ParseProjectWithFS(
		riskyFS,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(incrementalProgramName),
		ssaapi.WithBaseProgramName(baseProgramName),
	)
	require.NoError(t, err)

	fullProg, err := ssaapi.FromDatabase(fullProgramName)
	require.NoError(t, err)
	incrementalProg, err := ssaapi.FromDatabase(incrementalProgramName)
	require.NoError(t, err)

	_, err = fullProg.SyntaxFlowWithError(rule,
		ssaapi.QueryWithEnableDebug(true),
		ssaapi.QueryWithSave(schema.SFResultKindScan),
		ssaapi.QueryWithTaskID("task-full-"+uuid.NewString()),
	)
	require.NoError(t, err)

	_, err = incrementalProg.SyntaxFlowWithError(rule,
		ssaapi.QueryWithEnableDebug(true),
		ssaapi.QueryWithSave(schema.SFResultKindScan),
		ssaapi.QueryWithTaskID("task-incremental-"+uuid.NewString()),
	)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	_, fullRisks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{fullProgramName},
	}, nil)
	require.NoError(t, err)

	_, incrementalRisks, err := yakit.QuerySSARisk(ssadb.GetDB(), &ypb.SSARisksFilter{
		ProgramName: []string{incrementalProgramName},
	}, nil)
	require.NoError(t, err)

	require.Len(t, fullRisks, 2)
	require.Len(t, incrementalRisks, 2)

	fullHashes := []string{fullRisks[0].RiskFeatureHash, fullRisks[1].RiskFeatureHash}
	incrementalHashes := []string{incrementalRisks[0].RiskFeatureHash, incrementalRisks[1].RiskFeatureHash}
	require.ElementsMatch(t, fullHashes, incrementalHashes, "full and incremental overlay risk_feature_hash should match under scan save kind")

	fullFunctionNames := []string{fullRisks[0].FunctionName, fullRisks[1].FunctionName}
	incrementalFunctionNames := []string{incrementalRisks[0].FunctionName, incrementalRisks[1].FunctionName}
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, fullFunctionNames)
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, incrementalFunctionNames)
}

func TestRiskFeatureHash_JavaOverlayDirectQueryKeepsSameFileMatches(t *testing.T) {
	baseFS := filesys.NewVirtualFs()
	baseFS.AddFile("src/main/java/com/example/App.java", `package com.example;

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
}`)

	riskyFS := filesys.NewVirtualFs()
	riskyFS.AddFile("src/main/java/com/example/App.java", `package com.example;

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
}`)

	rule := `
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

	baseProgramName := "java-overlay-direct-base-" + uuid.NewString()
	incrementalProgramName := "java-overlay-direct-incremental-" + uuid.NewString()

	t.Cleanup(func() {
		ssadb.DeleteProgram(ssadb.GetDB(), baseProgramName)
		ssadb.DeleteProgram(ssadb.GetDB(), incrementalProgramName)
		yakit.DeleteSSARisks(ssadb.GetDB(), &ypb.SSARisksFilter{
			ProgramName: []string{baseProgramName, incrementalProgramName},
		})
	})

	_, err := ssaapi.ParseProjectWithFS(baseFS, ssaapi.WithLanguage(ssaconfig.JAVA), ssaapi.WithProgramName(baseProgramName))
	require.NoError(t, err)
	_, err = ssaapi.ParseProjectWithFS(
		riskyFS,
		ssaapi.WithLanguage(ssaconfig.JAVA),
		ssaapi.WithProgramName(incrementalProgramName),
		ssaapi.WithBaseProgramName(baseProgramName),
	)
	require.NoError(t, err)

	incrementalProg, err := ssaapi.FromDatabase(incrementalProgramName)
	require.NoError(t, err)
	overlay := incrementalProg.GetOverlay()
	require.NotNil(t, overlay)

	result, err := overlay.SyntaxFlowWithError(rule, ssaapi.QueryWithEnableDebug(true))
	require.NoError(t, err)

	sinkValues := result.GetAlertValue("sink")
	require.Len(t, sinkValues, 2, "overlay query should keep both risky matches from the same file")

	err = result.CreateRisk()
	require.NoError(t, err)
	require.Equal(t, 2, result.RiskCount())

	functionNames := make([]string, 0, 2)
	for risk := range result.YieldRisk() {
		functionNames = append(functionNames, risk.FunctionName)
	}
	require.ElementsMatch(t, []string{"legacyDigest", "weakDigest"}, functionNames)
}
