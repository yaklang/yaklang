package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

func testCliParam(t *testing.T, args []string, callback func(*assert.Assertions, *CliApp, func())) {
	t.Helper()
	test := assert.New(t)
	app := NewCliApp()
	app.SetArgs(args)
	callback(test, app, func() {
		t.Helper()
		if app.paramInvalid.IsSet() {
			app.CliCheckFactory(func() {})()
			t.FailNow()
		}
	})
}

func TestCliParam(t *testing.T) {
	t.Run("normal", func(t *testing.T) {
		testCliParam(
			t,
			[]string{"--bool", "--int", "1", "--float", "2.9"},
			func(test *assert.Assertions, app *CliApp, check func()) {
				b := app.Bool("bool")
				i := app.Int("int")
				f := app.Float("float")
				check()
				test.True(b)
				test.Equal(i, 1)
				test.Equal(f, 2.9)
			})
	})

	t.Run("YakitPlugin", func(t *testing.T) {
		filename, err := utils.SaveTempFile("plugin1|plugin2", "test")
		defer os.Remove(filename)
		if err != nil {
			t.Fatal(err)
		}
		testCliParam(
			t,
			[]string{"--yakit-plugin-file", filename},
			func(test *assert.Assertions, app *CliApp, check func()) {
				plugins := app.YakitPlugin()
				check()
				test.ElementsMatch(plugins, []string{"plugin1", "plugin2"})
			})
	})

	t.Run("StringSlice", func(t *testing.T) {
		testCliParam(
			t,
			[]string{"--string-slice", "a,b,c"},
			func(test *assert.Assertions, app *CliApp, check func()) {
				ss := app.StringSlice("string-slice")
				check()
				test.ElementsMatch(ss, []string{"a", "b", "c"})
			},
		)
	})

	t.Run("Urls", func(t *testing.T) {
		testCliParam(
			t,
			[]string{"--urls", "yaklang.com:443,google.com:443,https://example.com"},
			func(test *assert.Assertions, app *CliApp, check func()) {
				urls := app.Urls("urls")
				check()
				test.ElementsMatch(urls, []string{"https://example.com", "https://google.com", "https://www.example.com", "https://www.google.com", "https://www.yaklang.com", "https://yaklang.com"})
			},
		)
	})

	t.Run("Hosts", func(t *testing.T) {
		testCliParam(
			t,
			[]string{"--hosts", "127.0.0.1,192.168.1.1-3"},
			func(test *assert.Assertions, app *CliApp, check func()) {
				hosts := app.Hosts("hosts")
				check()
				test.ElementsMatch(hosts, []string{"127.0.0.1", "192.168.1.1", "192.168.1.2", "192.168.1.3"})
			},
		)
	})

	t.Run("File", func(t *testing.T) {
		filename, err := utils.SaveTempFile("content", "test")
		defer os.Remove(filename)
		if err != nil {
			t.Fatal(err)
		}
		testCliParam(
			t,
			[]string{"--file", filename},
			func(test *assert.Assertions, app *CliApp, check func()) {
				file := app.File("file")
				check()
				test.Equal(string(file), "content")
			},
		)
	})

	t.Run("FileOrContent-File", func(t *testing.T) {
		filename, err := utils.SaveTempFile("content", "test")
		defer os.Remove(filename)
		if err != nil {
			t.Fatal(err)
		}
		testCliParam(
			t,
			[]string{"--file", filename},
			func(test *assert.Assertions, app *CliApp, check func()) {
				file := app.FileOrContent("file")
				check()
				test.Equal(string(file), "content")
			},
		)
	})

	t.Run("FileOrContent-Content", func(t *testing.T) {
		testCliParam(
			t,
			[]string{"--file", "content"},
			func(test *assert.Assertions, app *CliApp, check func()) {
				file := app.FileOrContent("file")
				check()
				test.Equal(string(file), "content")
			},
		)
	})

	t.Run("LineDict", func(t *testing.T) {
		filename, err := utils.SaveTempFile("line1\nline2\nline3", "test")
		defer os.Remove(filename)
		if err != nil {
			t.Fatal(err)
		}
		testCliParam(
			t,
			[]string{"--dict", filename},
			func(test *assert.Assertions, app *CliApp, check func()) {
				dict := app.LineDict("dict")
				check()
				test.ElementsMatch(dict, []string{"line1", "line2", "line3"})
			},
		)
	})
}

func TestCliParamOpt(t *testing.T) {
	t.Run("opt-default", func(t *testing.T) {
		test := assert.New(t)
		app := NewCliApp()
		app.SetArgs([]string{})
		s := app.String("s", app.SetDefault("default"))
		test.Equal(s, "default")
	})

	t.Run("opt-required", func(t *testing.T) {
		test := assert.New(t)
		app := NewCliApp()
		app.SetArgs([]string{})
		app.String("s", app.SetRequired(true))
		test.True(app.paramInvalid.IsSet())
	})

	t.Run("opt-default-with-required", func(t *testing.T) {
		test := assert.New(t)
		app := NewCliApp()
		app.SetArgs([]string{})
		app.String("s", app.SetRequired(true), app.SetDefault("123"))
		test.False(app.paramInvalid.IsSet())
	})
}

func TestCliConfig(t *testing.T) {
	t.Run("name-with-doc", func(t *testing.T) {
		test := assert.New(t)
		app := NewCliApp()
		app.SetArgs([]string{})
		app.SetCliName("cli-name")
		app.SetDoc("document")

		var builder strings.Builder
		app.Help(&builder)
		test.Contains(builder.String(), "cli-name")
		test.Contains(builder.String(), "document")
	})

	t.Run("param-help", func(t *testing.T) {
		test := assert.New(t)
		app := NewCliApp()
		app.SetArgs([]string{})
		app.String("s", app.SetHelp("document"))

		var builder strings.Builder
		app.Help(&builder)
		test.Contains(builder.String(), "document")
	})
}

func TestCliFileAllowNoFile(t *testing.T) {
	t.Run("AllowNoFile", func(t *testing.T) {
		app := NewCliApp()
		app.SetArgs([]string{})
		app.File("target-file")
		if app.paramInvalid.IsSet() {
			app.CliCheckFactory(func() {})()
			t.Fatal("allow no pass cli.File, but param invalid")
		}
	})
}
