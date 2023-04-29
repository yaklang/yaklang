package log

import (
	"gopkg.in/natefinch/lumberjack.v2"
	"os"
	"path"
)

var (
	_config = NewDefaultConfig()
	Level   = os.Getenv("LOG_LEVEL")
)

func init() {
	if Level == "" || (Level != "disable" && Level != "fatal" && Level != "error" && Level != "warn" && Level != "info" && Level != "debug") {
		Level = "info"
	}
}

type LoggerConfig struct {
	Level string
}

type FileConfig struct {
	Dir        string
	MaxSize    int // megabytes
	MaxBackUps int
	MaxAge     int // days
	Compress   bool
}

type Config struct {
	Level      string
	FileConfig FileConfig `json:"file_config" yaml:"file_config"`
	Loggers    map[string]LoggerConfig
}

func (c *Config) Clone() *Config {
	cc := *c
	cc.Loggers = make(map[string]LoggerConfig, len(c.Loggers))
	for k, v := range c.Loggers {
		cc.Loggers[k] = v
	}
	return &cc
}

func NewDefaultConfig() *Config {
	return &Config{
		Level:   Level,
		Loggers: make(map[string]LoggerConfig),
		FileConfig: FileConfig{
			Dir:        "",
			MaxSize:    50,
			MaxBackUps: 5,
			MaxAge:     90,
			Compress:   false,
		},
	}
}

func SetLoggerConfig(l *Logger, c *Config) {
	l.SetLevel(c.Level)
	if c.FileConfig.Dir != "" {
		l.SetOutput(&lumberjack.Logger{
			Filename:   path.Join(c.FileConfig.Dir, l.name) + ".log",
			MaxSize:    c.FileConfig.MaxSize,
			MaxBackups: c.FileConfig.MaxBackUps,
			MaxAge:     c.FileConfig.MaxAge,
			Compress:   c.FileConfig.Compress,
		})
	}
}

func SetConfig(c *Config) {
	lock.Lock()
	defer lock.Unlock()
	_config = c
	for k, v := range loggers {
		config, ok := c.Loggers[k]
		if ok {
			v.SetLevel(config.Level)
		} else {
			v.SetLevel(c.Level)
		}
		if c.FileConfig.Dir != "" {
			v.SetOutput(&lumberjack.Logger{
				Filename:   path.Join(c.FileConfig.Dir, k) + ".log",
				MaxSize:    c.FileConfig.MaxSize,
				MaxBackups: c.FileConfig.MaxBackUps,
				MaxAge:     c.FileConfig.MaxAge,
				Compress:   c.FileConfig.Compress,
			})
		}
	}
}

func ReloadLogLevel(c *Config) {
	lock.Lock()
	defer lock.Unlock()
	for k, v := range loggers {
		config, ok := c.Loggers[k]
		if ok {
			v.SetLevel(config.Level)
		} else {
			v.SetLevel(c.Level)
		}
	}
}

func GetConfig() *Config {
	return _config
}
