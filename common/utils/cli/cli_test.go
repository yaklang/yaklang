package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"

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

	t.Run("IntSlice", func(t *testing.T) {
		testCliParam(
			t,
			[]string{"--int-slice", "1,2,3"},
			func(test *assert.Assertions, app *CliApp, check func()) {
				is := app.IntSlice("int-slice")
				check()
				test.ElementsMatch(is, []int{1, 2, 3})
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
	t.Run("opt-shortName", func(t *testing.T) {
		test := assert.New(t)
		app := NewCliApp()
		token := utils.RandStringBytes(16)
		app.SetArgs([]string{"-t", token})
		s := app.String("text", app.SetShortName("t"))
		test.Equal(s, token)
	})
	t.Run("opt-easy-shortName", func(t *testing.T) {
		test := assert.New(t)
		app := NewCliApp()
		token := utils.RandStringBytes(16)
		app.SetArgs([]string{"-t", token})
		testcases := []string{"test t", "t test", "test,t", "t,test"}
		for _, testcase := range testcases {
			s := app.String(testcase)
			test.Equal(s, token)
		}
	})
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

	t.Run("opt-env-with-default", func(t *testing.T) {
		test := assert.New(t)
		key := utils.RandStringBytes(10)
		if db := consts.GetGormProfileDatabase().Save(&schema.PluginEnv{Key: key, Value: "abc"}); db.Error != nil {
			t.Fatal(db.Error)
		}
		defer consts.GetGormProfileDatabase().Where("key = ?", key).Unscoped().Delete(&schema.PluginEnv{})
		app := NewCliApp()
		app.SetArgs([]string{})
		s := app.String("s", app.SetPluginEnv(key), app.SetDefault("123"))
		test.Equal("abc", s)
	})

	t.Run("opt-env-with-default2", func(t *testing.T) { // test empty
		test := assert.New(t)
		key := utils.RandStringBytes(10)
		if db := consts.GetGormProfileDatabase().Save(&schema.PluginEnv{Key: key, Value: ""}); db.Error != nil {
			t.Fatal(db.Error)
		}
		defer consts.GetGormProfileDatabase().Where("key = ?", key).Unscoped().Delete(&schema.PluginEnv{})
		app := NewCliApp()
		app.SetArgs([]string{"--s", "abc"})
		s := app.String("s", app.SetPluginEnv(key), app.SetDefault("123"))
		test.Equal("", s)
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

// TestParseIntScientificNotation tests that parseInt can handle scientific notation
// This is important because large numbers from Yak scripts may be formatted as "2e+09"
func TestParseIntScientificNotation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"normal int", "123", 123},
		{"large int", "2000000000", 2000000000},
		{"scientific notation positive", "2e+09", 2000000000},
		{"scientific notation", "2e9", 2000000000},
		{"scientific notation with decimal", "1.5e+09", 1500000000},
		{"negative scientific notation", "-2e+09", -2000000000},
		{"zero", "0", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseInt(tt.input)
			if result != tt.expected {
				t.Errorf("parseInt(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

// TestCliIntWithLargeDefault tests that cli.Int works with large default values
// that may be formatted as scientific notation
func TestCliIntWithLargeDefault(t *testing.T) {
	test := assert.New(t)
	app := NewCliApp()
	app.SetArgs([]string{}) // No args, use default

	// Test with large default value (2GB = 2e+09)
	largeDefault := 1000 * 1000 * 1000 * 2
	result := app.Int("file-size-max", app.SetDefault(largeDefault))

	test.Equal(largeDefault, result, "Large default value should be parsed correctly")
}
