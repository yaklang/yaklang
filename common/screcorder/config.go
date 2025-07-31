package screcorder

import (
	"context"
	"fmt"
	"runtime"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
)

type Config struct {
	Framerate           int // 1-3 is safe
	Height              int
	Width               int
	CoefficientPTSFloat float64 // 0.333*PTS
	MouseCapture        bool
	MouseClickCapture   bool

	ctx    context.Context
	cancel context.CancelFunc
}

func WithContext(ctx context.Context, cancel context.CancelFunc) ConfigOpt {
	return func(config *Config) {
		config.ctx = ctx
		config.cancel = cancel
	}
}

func (c *Config) ToParams(input, output string) ([]string, error) {
	var params []string
	switch runtime.GOOS {
	case "darwin":
		params = append(params, "-f", "avfoundation")
		if c.MouseCapture {
			params = append(params, "-capture_cursor", "1")
			if c.MouseClickCapture {
				params = append(params, "-capture_mouse_clicks", "1")
			}
		}
		// -video_size 1680x1050
		params = append(params, "-video_size", fmt.Sprintf("%vx%v", c.Width, c.Height))
	case "windows":
		params = append(params, "-f", "gdigrab")
		if c.MouseCapture {
			params = append(params, "-draw_mouse", "1")
		}
		// Set video size for Windows if specified
		if c.Width > 0 && c.Height > 0 {
			params = append(params, "-video_size", fmt.Sprintf("%vx%v", c.Width, c.Height))
		}
	default:
		return nil, utils.Errorf("unsupported os: %v", runtime.GOOS)
	}

	// framerate
	params = append(params, "-r", fmt.Sprintf("%0.1f", float64(c.Framerate)))

	// INPUT
	params = append(params, "-i", input)

	// 抽样 PTS
	if c.CoefficientPTSFloat > 0 {
		params = append(params, "-vf", fmt.Sprintf("setpts=%0.2f*PTS", float64(c.CoefficientPTSFloat)))
	} else {
		params = append(params, "-vf", "setpts=1*PTS")
	}

	// -c:v libx264 -pix_fmt yuv420p
	params = append(params, "-c:v", "libx264", "-pix_fmt", "yuv420p")

	if utils.GetFirstExistedFile(output) != "" {
		return nil, utils.Errorf("get existed output: %v failed", output)
	}

	params = append(params, output)
	return params, nil
}

func NewDefaultConfig() *Config {
	ctx, cancel := context.WithCancel(context.Background())
	return &Config{
		Framerate:           2,
		Height:              1080,
		Width:               1920,
		CoefficientPTSFloat: 0.5,
		MouseCapture:        true,
		MouseClickCapture:   true,
		ctx:                 ctx,
		cancel:              cancel,
	}
}

type ConfigOpt func(config *Config)

func WithFramerate(i int) ConfigOpt {
	return func(config *Config) {
		if i <= 1 {

		}
		config.Framerate = i
	}
}

func WithMouseCapture(i bool) ConfigOpt {
	return func(config *Config) {
		config.MouseCapture = i
		config.MouseClickCapture = i
	}
}

func WithCoefficientPTS(i float64) ConfigOpt {
	return func(config *Config) {
		if i <= 0 {
			return
		}
		config.CoefficientPTSFloat = i
	}
}

func WithResolutionSize(i string) ConfigOpt {
	return func(config *Config) {
		i = strings.ToLower(i)
		var height, width int
		if strings.Contains(i, "x") {
			results := strings.SplitN(i, "x", 2)
			if len(results) == 2 {
				width, _ = strconv.Atoi(results[0])
				height, _ = strconv.Atoi(results[1])
			}
		}
		if height > 0 && width > 0 {
			config.Height = height
			config.Width = width
		}
	}
}
