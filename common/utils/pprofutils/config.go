package pprofutils

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type Config struct {
	cpuProfileFile       string
	memProfileFile       string
	ctx                  context.Context
	onCPUProfileFinished func(string, error)
	onCPUProfileStarted  func(string)
	onMemProfileStarted  func(string)
	onMemProfileFinished func(string, error)
}

func NewConfig() *Config {
	return &Config{
		cpuProfileFile: "",
	}
}

type Option func(*Config)

// cpuProfilePath 指定 CPU profile 的输出文件路径（导出名为 pprof.cpuProfilePath）
// 作为 pprof.StartCPUProfile / pprof.StartCPUAndMemoryProfile 的可选项使用；不指定时使用默认临时路径
//
// 参数:
//   - file: CPU profile 输出文件路径
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "cpu_path_demo.prof")
// pprof.StartCPUProfile(pprof.cpuProfilePath(prof), pprof.timeout(1))
// println(file.IsExisted(prof))   // OUT: true
// assert file.IsExisted(prof), "cpuProfilePath should control where the CPU profile is written"
// file.Remove(prof)
// ```
func WithCPUProfileFile(file string) Option {
	return func(c *Config) {
		c.cpuProfileFile = file
	}
}

// memProfilePath 指定内存 profile 的输出文件路径（导出名为 pprof.memProfilePath）
// 作为 pprof.StartMemoryProfile / pprof.StartCPUAndMemoryProfile 的可选项使用；不指定时使用默认临时路径
//
// 参数:
//   - file: 内存 profile 输出文件路径
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "mem_path_demo.prof")
// pprof.StartMemoryProfile(pprof.memProfilePath(prof), pprof.timeout(1))
// println(file.IsExisted(prof))   // OUT: true
// assert file.IsExisted(prof), "memProfilePath should control where the memory profile is written"
// file.Remove(prof)
// ```
func WithMemProfileFile(file string) Option {
	return func(c *Config) {
		c.memProfileFile = file
	}
}

// ctx 指定控制采样时长的上下文（导出名为 pprof.ctx）
// 采样会一直进行直到上下文结束；若上下文未设置截止时间，则回退为默认 15 秒
// 作为 pprof 采样函数的可选项使用
//
// 参数:
//   - ctx: 控制采样生命周期的上下文
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "cpu_ctx_demo.prof")
// ctx = context.WithTimeout(context.Background(), time.ParseDuration("1s")~)[0]
// pprof.StartCPUProfile(pprof.cpuProfilePath(prof), pprof.ctx(ctx))
// println(file.IsExisted(prof))   // OUT: true
// assert file.IsExisted(prof), "ctx should bound the CPU profiling duration"
// file.Remove(prof)
// ```
func WithContext(ctx context.Context) Option {
	return func(c *Config) {
		c.ctx = ctx
	}
}

// timeout 指定采样持续的秒数（导出名为 pprof.timeout）
// 内部会据此创建一个带截止时间的上下文；传入非正数时回退为默认 15 秒
// 作为 pprof 采样函数的可选项使用
//
// 参数:
//   - i: 采样秒数（支持小数）
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "mem_timeout_demo.prof")
// pprof.StartMemoryProfile(pprof.memProfilePath(prof), pprof.timeout(1))
// println(file.IsExisted(prof))   // OUT: true
// assert file.IsExisted(prof), "timeout should bound the profiling duration"
// file.Remove(prof)
// ```
func WithTimeout(i float64) Option {
	return func(c *Config) {
		if i <= 0 {
			i = 15
		}
		c.ctx = utils.TimeoutContextSeconds(i)
	}
}

// callback 设置采样完成后的统一回调（导出名为 pprof.callback）
// 同时作用于 CPU 与内存 profile 完成事件，回调参数为生成的 profile 文件路径
// 作为 pprof 采样函数的可选项使用
//
// 参数:
//   - h: 回调函数 func(profilePath)
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "mem_cb_demo.prof")
// done = ""
// pprof.StartMemoryProfile(pprof.memProfilePath(prof), pprof.timeout(1), pprof.callback(func(p) { done = p }))
// println(done == prof)   // OUT: true
// assert done == prof, "callback should receive the generated profile path"
// file.Remove(prof)
// ```
func WithFinished(h func(string)) Option {
	return func(config *Config) {
		config.onMemProfileFinished = func(s string, err error) {
			if err != nil {
				log.Errorf("memory profile finished: %s, error: %v", s, err)
			}
			if s != "" {
				h(s)
			}
		}
		config.onCPUProfileFinished = func(s string, err error) {
			if err != nil {
				log.Errorf("cpu profile finished: %s, error: %v", s, err)
			}
			if s != "" {
				h(s)
			}
		}
	}
}

// onCPUProfileFinished 设置 CPU 采样结束时的回调（导出名为 pprof.onCPUProfileFinished）
// 回调参数为生成的 CPU profile 文件路径与可能的错误；作为 pprof.StartCPUProfile 的可选项使用
//
// 参数:
//   - fn: 回调函数 func(profilePath, err)
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "cpu_fin_demo.prof")
// fin = ""
// pprof.StartCPUProfile(pprof.cpuProfilePath(prof), pprof.timeout(1), pprof.onCPUProfileFinished(func(p, err) { fin = p }))
// println(fin == prof)   // OUT: true
// assert fin == prof, "onCPUProfileFinished should report the finished CPU profile path"
// file.Remove(prof)
// ```
func WithOnCPUProfileFinished(fn func(string, error)) Option {
	return func(c *Config) {
		c.onCPUProfileFinished = fn
	}
}

// onCPUProfileStarted 设置 CPU 采样开始时的回调（导出名为 pprof.onCPUProfileStarted）
// 回调参数为 CPU profile 文件路径；作为 pprof.StartCPUProfile 的可选项使用
//
// 参数:
//   - fn: 回调函数 func(profilePath)
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "cpu_start_demo.prof")
// started = ""
// pprof.StartCPUProfile(pprof.cpuProfilePath(prof), pprof.timeout(1), pprof.onCPUProfileStarted(func(p) { started = p }))
// println(started == prof)   // OUT: true
// assert started == prof, "onCPUProfileStarted should report the CPU profile path"
// file.Remove(prof)
// ```
func WithOnCPUProfileStarted(fn func(string)) Option {
	return func(c *Config) {
		c.onCPUProfileStarted = fn
	}
}

// onMemProfileStarted 设置内存采样开始时的回调（导出名为 pprof.onMemProfileStarted）
// 回调参数为内存 profile 文件路径；作为 pprof.StartMemoryProfile 的可选项使用
//
// 参数:
//   - fn: 回调函数 func(profilePath)
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "mem_start_demo.prof")
// started = ""
// pprof.StartMemoryProfile(pprof.memProfilePath(prof), pprof.timeout(1), pprof.onMemProfileStarted(func(p) { started = p }))
// println(started == prof)   // OUT: true
// assert started == prof, "onMemProfileStarted should report the memory profile path"
// file.Remove(prof)
// ```
func WithOnMemProfileStarted(fn func(string)) Option {
	return func(c *Config) {
		c.onMemProfileStarted = fn
	}
}

// onMemProfileFinished 设置内存采样结束时的回调（导出名为 pprof.onMemProfileFinished）
// 回调参数为生成的内存 profile 文件路径与可能的错误；作为 pprof.StartMemoryProfile 的可选项使用
//
// 参数:
//   - fn: 回调函数 func(profilePath, err)
//
// 返回值:
//   - 可传入 pprof 采样函数的选项
//
// Example:
// ```
// prof = file.Join(os.TempDir(), "mem_fin_demo.prof")
// fin = ""
// pprof.StartMemoryProfile(pprof.memProfilePath(prof), pprof.timeout(1), pprof.onMemProfileFinished(func(p, err) { fin = p }))
// println(fin == prof)   // OUT: true
// assert fin == prof, "onMemProfileFinished should report the finished memory profile path"
// file.Remove(prof)
// ```
func WithOnMemProfileFinished(fn func(string, error)) Option {
	return func(c *Config) {
		c.onMemProfileFinished = fn
	}
}
