package pcaputil

import (
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"
)

func (p *TrafficPool) observeCapture(size int) {
	p.capturedPackets.Add(1)
	p.capturedBytes.Add(uint64(size))
}

func (c *CaptureConfig) requiresExclusiveHandle() bool {
	return c.reassemblyOptions.Workers > 1 || c.binParser != nil || c.recorder != nil || c.captureBuffer > 0
}

// WithCaptureBufferSize requests a native buffer per live device before opening
// it. Zero keeps the backend default; otherwise use 64 KiB through 256 MiB.
// A larger buffer absorbs bursts, but cannot fix sustained analysis overload.
func WithCaptureBufferSize(size int) CaptureOption {
	return func(c *CaptureConfig) error {
		if size != 0 && (size < 64<<10 || size > 256<<20) {
			return fmt.Errorf("capture buffer must be zero or 64 KiB through 256 MiB")
		}
		if size > 0 && c.EnableCache {
			return fmt.Errorf("capture buffer requires an exclusive capture handle")
		}
		c.captureBuffer = size
		return nil
	}
}

// WithCaptureWriter records packets before analysis without constructing public
// packet objects. It writes classic nanosecond pcap with one link type. Multiple
// devices with different link types must use separate captures. Writes provide
// backpressure; errors stop capture instead of silently discarding recordings.
// The caller owns the writer and must flush/close it after Start/ReplayPcap.
func WithCaptureWriter(w io.Writer) CaptureOption {
	return func(c *CaptureConfig) error {
		if c.outputFile != "" {
			return fmt.Errorf("choose an output file or a capture writer")
		}
		if w == nil {
			return fmt.Errorf("capture writer is nil")
		}
		if c.EnableCache {
			return fmt.Errorf("capture writer requires an exclusive capture handle")
		}
		c.recorder = &captureWriter{output: w}
		return nil
	}
}

// pcap_outputFile 将捕获到的原始包保存为新的 pcap 文件，配合 StartSniff 或 OpenPcapFile 使用。
// 文件在捕获/回放开始时创建，结束或出错时由 pcapx 关闭；已存在的文件不会被覆盖。
// 保存发生在协议分析前，回调中的显示筛选不影响保存内容，BPF 输入过滤仍生效。
// 输出为单链路类型的纳秒精度 pcap；不同链路类型应分开捕获。写入失败会返回错误。
//
// 参数:
//   - filename: 尚不存在的非空输出文件路径，父目录必须存在。不能与 pcap_captureWriter 同时使用。
//
// 返回值:
//   - 抓包配置选项。
//
// Example:
// ```
// pcapx.OpenPcapFile("input.pcapng", pcapx.pcap_outputFile("output.pcap"))~
// ```
func WithOutputFile(filename string) CaptureOption {
	return func(c *CaptureConfig) error {
		if filename == "" {
			return fmt.Errorf("capture output filename is empty")
		}
		if c.recorder != nil {
			return fmt.Errorf("choose an output file or a capture writer")
		}
		c.outputFile = filename
		return nil
	}
}

func (c *CaptureConfig) openCaptureOutput() (func() error, error) {
	if c.outputFile == "" {
		return func() error { return nil }, nil
	}
	f, err := os.OpenFile(c.outputFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	c.recorder = &captureWriter{output: f}
	return f.Close, nil
}

type captureWriter struct {
	mu     sync.Mutex
	output io.Writer
	writer *pcapgo.Writer
	link   layers.LinkType
	err    error
}

func (r *captureWriter) initLocked(link layers.LinkType) error {
	if r.err != nil {
		return r.err
	}
	if r.writer == nil {
		r.writer, r.link = pcapgo.NewWriterNanos(r.output), link
		r.err = r.writer.WriteFileHeader(16<<20, link)
	} else if link != r.link {
		r.err = fmt.Errorf("capture writer: link type changed from %v to %v; record interfaces separately", r.link, link)
	}
	return r.err
}

func (r *captureWriter) init(link layers.LinkType) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initLocked(link)
}

func (r *captureWriter) write(raw []byte, ci gopacket.CaptureInfo, link layers.LinkType) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.initLocked(link); err != nil {
		return err
	}
	r.err = r.writer.WritePacket(ci, raw)
	return r.err
}
