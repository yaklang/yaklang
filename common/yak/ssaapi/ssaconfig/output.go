package ssaconfig

// OutputConfig CLI输出配置
type OutputConfig struct {
	OutputFile   string `json:"output_file"`   // 输出文件路径
	OutputFormat string `json:"output_format"` // 输出格式: sarif, irify, irify-full, irify-react-report

	// 输出选项
	WithFileContent  bool `json:"with_file_content"`  // 是否包含文件内容
	WithDataflowPath bool `json:"with_dataflow_path"` // 是否包含数据流路径
}

// --- 输出配置 Get 方法 ---

func (c *Config) GetOutputFile() string {
	if c == nil || c.Output == nil {
		return ""
	}
	return c.Output.OutputFile
}

func (c *Config) GetOutputFormat() string {
	if c == nil || c.Output == nil {
		return ""
	}
	return c.Output.OutputFormat
}

func (c *Config) GetOutputWithFileContent() bool {
	if c == nil || c.Output == nil {
		return false
	}
	return c.Output.WithFileContent
}

func (c *Config) GetOutputWithDataflowPath() bool {
	if c == nil || c.Output == nil {
		return false
	}
	return c.Output.WithDataflowPath
}

// --- 输出配置 Set 方法 ---

func (c *Config) SetOutputFile(outputFile string) {
	if c == nil {
		return
	}
	if c.Output == nil {
		c.Output = defaultOutputConfig()
	}
	c.Output.OutputFile = outputFile
}

func (c *Config) SetOutputFormat(outputFormat string) {
	if c == nil {
		return
	}
	if c.Output == nil {
		c.Output = defaultOutputConfig()
	}
	c.Output.OutputFormat = outputFormat
}

func (c *Config) SetOutputWithFileContent(withFileContent bool) {
	if c == nil {
		return
	}
	if c.Output == nil {
		c.Output = defaultOutputConfig()
	}
	c.Output.WithFileContent = withFileContent
}

func (c *Config) SetOutputWithDataflowPath(withDataflowPath bool) {
	if c == nil {
		return
	}
	if c.Output == nil {
		c.Output = defaultOutputConfig()
	}
	c.Output.WithDataflowPath = withDataflowPath
}

// --- 输出配置 Options ---

// WithOutputFile 设置输出文件路径
func WithOutputFile(outputFile string) Option {
	return func(c *Config) error {
		if err := c.ensureOutput("Output File"); err != nil {
			return err
		}
		c.Output.OutputFile = outputFile
		return nil
	}
}

// WithOutputFormat 设置输出格式
func WithOutputFormat(outputFormat string) Option {
	return func(c *Config) error {
		if err := c.ensureOutput("Output Format"); err != nil {
			return err
		}
		c.Output.OutputFormat = outputFormat
		return nil
	}
}

// WithOutputFileContent 设置是否包含文件内容
func WithOutputFileContent(withFileContent bool) Option {
	return func(c *Config) error {
		if err := c.ensureOutput("Output File Content"); err != nil {
			return err
		}
		c.Output.WithFileContent = withFileContent
		return nil
	}
}

// WithOutputDataflowPath 设置是否包含数据流路径
func WithOutputDataflowPath(withDataflowPath bool) Option {
	return func(c *Config) error {
		if err := c.ensureOutput("Output Dataflow Path"); err != nil {
			return err
		}
		c.Output.WithDataflowPath = withDataflowPath
		return nil
	}
}
