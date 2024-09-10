package yakcmds_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/cmd/yakcmds"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

func addCommands(app *cli.App, cmds ...*cli.Command) {
	for _, i := range cmds {
		app.Commands = append(app.Commands, *i)
	}
}

func TestRecompileWarn(t *testing.T) {
	tmpDir := t.TempDir()
	log.Infof("tmpDir: %s", tmpDir)

	// create a test yak file
	file, err := os.Create(tmpDir + "/test.yak")
	require.NoError(t, err)
	file.WriteString(`
	a = 1
	`)
	defer file.Close()

	programName := uuid.NewString()

	// compile the yak file
	app := cli.NewApp()
	addCommands(app, yakcmds.SSACompilerCommands...)
	// compile with program name
	err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
	require.NoError(t, err)
	defer ssadb.DeleteProgram(ssadb.GetDB(), programName)

	// recompile
	err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
	require.Error(t, err)
	require.Contains(t, err.Error(), "please use `re-compile` flag to re-compile or change program name")
}

func TestEvaluateVerifyResult(t *testing.T) {
	t.Run("test empty alert symbol", func(t *testing.T) {
		tmpDir := t.TempDir()
		log.Infof("tmpDir: %s", tmpDir)
		javaFile, err := os.Create(tmpDir + "/test.java")
		require.NoError(t, err)
		javaFile.WriteString(`
	package test;
class Test {
	public static void main(String[] args) {
		System.out.println("Hello World");
}
}
	`)
		defer javaFile.Close()

		sfFile, err := os.Create(tmpDir + "/test.sf")
		require.NoError(t, err)
		sfFile.WriteString(`
	println() as $result;
		`)
		defer sfFile.Close()

		programName := uuid.NewString()
		app := cli.NewApp()
		addCommands(app, yakcmds.SSACompilerCommands...)
		err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
		require.NoError(t, err)
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
		err = app.Run([]string{"yak", "sf", tmpDir, "-p", programName})
		require.NotNil(t, err)
		log.Infof("err: \n%s", err)
		require.Contains(t, err.Error(), "alert symbol table is empty")
	})

	t.Run("test check no-exist result", func(t *testing.T) {
		tmpDir := t.TempDir()
		log.Infof("tmpDir: %s", tmpDir)
		javaFile, err := os.Create(tmpDir + "/test.java")
		require.NoError(t, err)
		javaFile.WriteString(`
	package test;
class Test {
	public static void main(String[] args) {
		System.out.println("Hello World");
}
}
	`)
		defer javaFile.Close()
		sfFile, err := os.Create(tmpDir + "/test.sf")
		require.NoError(t, err)
		sfFile.WriteString(`
		println() as $result;
		check $noExist;
		alert $result;
		`)
		defer sfFile.Close()

		programName := uuid.NewString()
		app := cli.NewApp()
		addCommands(app, yakcmds.SSACompilerCommands...)
		err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
		require.NoError(t, err)
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
		err = app.Run([]string{"yak", "sf", tmpDir, "-p", programName})
		require.NotNil(t, err)
		log.Infof("err:\n%s", err)
		require.Contains(t, err.Error(), "$noExist is not found")
	})

	t.Run("test lib not exporting output in `alert`", func(t *testing.T) {
		tmpDir := t.TempDir()
		log.Infof("tmpDir: %s", tmpDir)
		javaFile, err := os.Create(tmpDir + "/test.java")
		require.NoError(t, err)
		javaFile.WriteString(`
	package test;
class Test {
	public static void main(String[] args) {
		System.out.println("Hello World");
}
}
	`)
		defer javaFile.Close()
		sfFile, err := os.Create(tmpDir + "/test.sf")
		require.NoError(t, err)
		sfFile.WriteString(`
		desc(lib: "abc");
abc() as $output;
alert $output;
		`)
		defer sfFile.Close()

		programName := uuid.NewString()
		app := cli.NewApp()
		addCommands(app, yakcmds.SSACompilerCommands...)
		err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
		require.NoError(t, err)
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
		err = app.Run([]string{"yak", "sf", tmpDir, "-p", programName})
		require.NotNil(t, err)
		log.Infof("err:\n%s", err)
		require.Contains(t, err.Error(), "alert symbol table is empty")
		require.Contains(t, err.Error(), "exporting output in `alert`")
	})

	t.Run("test lib not exporting output in `alert`", func(t *testing.T) {
		tmpDir := t.TempDir()
		log.Infof("tmpDir: %s", tmpDir)
		javaFile, err := os.Create(tmpDir + "/test.java")
		require.NoError(t, err)
		javaFile.WriteString(`
	package test;
class Test {
	public static void main(String[] args) {
		System.out.println("Hello World");
}
}
	`)
		defer javaFile.Close()
		sfFile, err := os.Create(tmpDir + "/test.sf")
		require.NoError(t, err)
		sfFile.WriteString(`
		desc(
			alert_min: '2000'
		);
		println(* as $param)
		alert $param;
		`)
		defer sfFile.Close()

		programName := uuid.NewString()
		app := cli.NewApp()
		addCommands(app, yakcmds.SSACompilerCommands...)
		err = app.Run([]string{"yak", "ssa-compile", "-t", tmpDir, "-p", programName})
		require.NoError(t, err)
		defer ssadb.DeleteProgram(ssadb.GetDB(), programName)
		err = app.Run([]string{"yak", "sf", tmpDir, "-p", programName})
		require.NotNil(t, err)
		log.Infof("err:\n%s", err)
		require.Contains(t, err.Error(), "alert symbol table is less than alert_min config: 2000")
	})
}
