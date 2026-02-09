package cli

import (
	"testing"
)

func TestApp_IgnoreUnknownFlags_Enabled(t *testing.T) {
	app := NewApp()
	app.IgnoreUnknownFlags = true
	app.Flags = []Flag{
		StringFlag{Name: "known"},
	}
	app.Action = func(c *Context) error {
		if c.String("known") != "yes" {
			t.Errorf("Expected known flag value to be 'yes', got '%s'", c.String("known"))
		}
		return nil
	}

	err := app.Run([]string{"app", "--known", "yes", "--unknown"})
	if err != nil {
		t.Errorf("Expected no error when IgnoreUnknownFlags is true, got %v", err)
	}
}

func TestApp_IgnoreUnknownFlags_Disabled(t *testing.T) {
	app := NewApp()
	app.IgnoreUnknownFlags = false // Default behavior
	app.Flags = []Flag{
		StringFlag{Name: "known"},
	}
	app.Action = func(c *Context) error {
		return nil
	}

	err := app.Run([]string{"app", "--known", "yes", "--unknown"})
	if err == nil {
		t.Error("Expected error when IgnoreUnknownFlags is false, got nil")
	}
}

func TestApp_IgnoreUnknownFlags_SubCommand_Enabled(t *testing.T) {
	app := NewApp()
	app.IgnoreUnknownFlags = true
	app.Commands = []Command{
		{
			Name: "sub",
			Flags: []Flag{
				StringFlag{Name: "known"},
			},
			Action: func(c *Context) error {
				if c.String("known") != "yes" {
					t.Errorf("Expected known flag value to be 'yes', got '%s'", c.String("known"))
				}
				return nil
			},
		},
	}

	err := app.Run([]string{"app", "sub", "--known", "yes", "--unknown"})
	if err != nil {
		t.Errorf("Expected no error when IgnoreUnknownFlags is true in subcommand, got %v", err)
	}
}

func TestApp_IgnoreUnknownFlags_SubCommand_Disabled(t *testing.T) {
	app := NewApp()
	app.IgnoreUnknownFlags = false
	app.Commands = []Command{
		{
			Name: "sub",
			Flags: []Flag{
				StringFlag{Name: "known"},
			},
			Action: func(c *Context) error {
				return nil
			},
		},
	}

	err := app.Run([]string{"app", "sub", "--known", "yes", "--unknown"})
	if err == nil {
		t.Error("Expected error when IgnoreUnknownFlags is false in subcommand, got nil")
	}
}

func TestApp_IgnoreUnknownFlags_ShortOptions_AllDefined(t *testing.T) {
	app := NewApp()
	app.UseShortOptionHandling = true
	app.IgnoreUnknownFlags = true
	app.Flags = []Flag{
		BoolFlag{Name: "all, a"},
		BoolFlag{Name: "brief, b"},
	}
	var aFlag, bFlag bool
	app.Action = func(c *Context) error {
		aFlag = c.Bool("all")
		bFlag = c.Bool("brief")
		return nil
	}

	err := app.Run([]string{"app", "-ab"})
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if !aFlag || !bFlag {
		t.Errorf("Expected both flags to be true, got a=%v, b=%v", aFlag, bFlag)
	}
}

func TestApp_IgnoreUnknownFlags_ShortOptions_SomeUndefined_Enabled(t *testing.T) {
	app := NewApp()
	app.UseShortOptionHandling = true
	app.IgnoreUnknownFlags = true
	app.Flags = []Flag{
		BoolFlag{Name: "all, a"},
		BoolFlag{Name: "brief, b"},
	}
	var aFlag, bFlag bool
	app.Action = func(c *Context) error {
		aFlag = c.Bool("all")
		bFlag = c.Bool("brief")
		return nil
	}

	err := app.Run([]string{"app", "-abc"})
	if err != nil {
		t.Errorf("Expected no error when IgnoreUnknownFlags is true, got %v", err)
	}
	if !aFlag || !bFlag {
		t.Errorf("Expected defined flags to be true, got a=%v, b=%v", aFlag, bFlag)
	}
}

func TestApp_IgnoreUnknownFlags_ShortOptions_SomeUndefined_Disabled(t *testing.T) {
	app := NewApp()
	app.UseShortOptionHandling = true
	app.IgnoreUnknownFlags = false
	app.Flags = []Flag{
		BoolFlag{Name: "all, a"},
		BoolFlag{Name: "brief, b"},
	}
	app.Action = func(c *Context) error {
		return nil
	}

	err := app.Run([]string{"app", "-abc"})
	if err == nil {
		t.Error("Expected error when IgnoreUnknownFlags is false with undefined short option, got nil")
	}
}

func TestApp_IgnoreUnknownFlags_MixedLongAndShort_Enabled(t *testing.T) {
	app := NewApp()
	app.UseShortOptionHandling = true
	app.IgnoreUnknownFlags = true
	app.Flags = []Flag{
		StringFlag{Name: "defined, d"},
		BoolFlag{Name: "all, a"},
	}
	var definedVal string
	var aFlag bool
	app.Action = func(c *Context) error {
		definedVal = c.String("defined")
		aFlag = c.Bool("all")
		return nil
	}

	err := app.Run([]string{"app", "--unknown", "--defined", "value", "-a", "--another-unknown"})
	if err != nil {
		t.Errorf("Expected no error when IgnoreUnknownFlags is true, got %v", err)
	}
	if definedVal != "value" {
		t.Errorf("Expected defined flag value to be 'value', got '%s'", definedVal)
	}
	if !aFlag {
		t.Error("Expected 'all' flag to be true")
	}
}

func TestApp_IgnoreUnknownFlags_MixedLongAndShort_Disabled(t *testing.T) {
	app := NewApp()
	app.UseShortOptionHandling = true
	app.IgnoreUnknownFlags = false
	app.Flags = []Flag{
		StringFlag{Name: "defined, d"},
		BoolFlag{Name: "all, a"},
	}
	app.Action = func(c *Context) error {
		return nil
	}

	err := app.Run([]string{"app", "--unknown", "--defined", "value", "-a"})
	if err == nil {
		t.Error("Expected error when IgnoreUnknownFlags is false with unknown flag, got nil")
	}
}

func TestApp_IgnoreUnknownFlags_OnlyUndefinedShort_Enabled(t *testing.T) {
	app := NewApp()
	app.UseShortOptionHandling = true
	app.IgnoreUnknownFlags = true
	app.Flags = []Flag{
		BoolFlag{Name: "all, a"},
	}
	var aFlag bool
	app.Action = func(c *Context) error {
		aFlag = c.Bool("all")
		return nil
	}

	err := app.Run([]string{"app", "-x"})
	if err != nil {
		t.Errorf("Expected no error when IgnoreUnknownFlags is true, got %v", err)
	}
	if aFlag {
		t.Error("Expected 'all' flag to be false")
	}
}

func TestApp_IgnoreUnknownFlags_OnlyUndefinedShort_Disabled(t *testing.T) {
	app := NewApp()
	app.UseShortOptionHandling = true
	app.IgnoreUnknownFlags = false
	app.Flags = []Flag{
		BoolFlag{Name: "all, a"},
	}
	app.Action = func(c *Context) error {
		return nil
	}

	err := app.Run([]string{"app", "-x"})
	if err == nil {
		t.Error("Expected error when IgnoreUnknownFlags is false with undefined short option, got nil")
	}
}
