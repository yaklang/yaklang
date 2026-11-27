package extrafp

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// rdp_receive_packet
var os___xrdp_1 = []byte{0x03, 0x00, 0x00, 0x09, 0x02, 0xf0, 0x80, 0x21, 0x80}                                                             // xrdp
var os___xrdp_2 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x01, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00} // xrdp
var os___xrdp_3 = []byte{0x03, 0x00, 0x00, 0x0b, 0x06, 0xd0, 0x00, 0x00, 0x00, 0x00, 0x00}                                                 // xrdp
var os___xrdp_4 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x01, 0x08, 0x00, 0x01, 0x00, 0x00, 0x00} // xrdp

var os______old = []byte{0x03, 0x00, 0x00, 0x0b, 0x06, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00} // Windows 2000 Advanced Server || Windoes XP Professional || Windows Embedded POSReady 2009 || Windows Embedded Standard

var os___2008_1 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x00, 0x08, 0x00, 0x02, 0x00, 0x00, 0x00} // Windows Server 2008 R2 Datacenter
var os___2008_2 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x01, 0x08, 0x00, 0x02, 0x00, 0x00, 0x00} // Windows Server 2008 R2 Datacenter
var os___2008_3 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x09, 0x08, 0x00, 0x02, 0x00, 0x00, 0x00} // Windows Server 2008 R2 Standard

var os___2012_1 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x00, 0x08, 0x00, 0x01, 0x00, 0x00, 0x00} // Windows Server 2012 R2
var os___2012_2 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x07, 0x08, 0x00, 0x02, 0x00, 0x00, 0x00} // Windows Server 2012
var os__2012_r2 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x0f, 0x08, 0x00, 0x02, 0x00, 0x00, 0x00} // Windows Server 2012 R2

var os__Vista_1 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x02, 0x1f, 0x08, 0x00, 0x02, 0x00, 0x00, 0x00} // Vista以后的操作系统

var os_Multiple_1 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x03, 0x00, 0x08, 0x00, 0x02, 0x00, 0x00, 0x00} // Windows 2003 / 2008 / 2012
var os_Multiple_2 = []byte{0x03, 0x00, 0x00, 0x13, 0x0e, 0xd0, 0x00, 0x00, 0x12, 0x34, 0x00, 0x03, 0x00, 0x08, 0x00, 0x03, 0x00, 0x00, 0x00} // Windows 2003 / 2008 / 2012

// Version 探测目标 RDP 服务的操作系统版本信息
// 参数:
//   - addr: 目标地址，格式为 host:port
//   - timeout: 探测的超时时间
//
// 返回值:
//   - 识别到的操作系统版本描述字符串
//   - 命中的版本指纹细节列表
//   - 错误信息，探测失败时返回非空
//
// Example:
// ```
// // 探测 RDP 服务版本，依赖目标服务，此处仅作示意
// version, details, err = rdp.Version("192.168.1.1:3389", 5)
// println(version)
// ```
func RDPVersion(addr string, timeout time.Duration) (string, []string, error) {
	return RDPVersionContext(context.Background(), addr, timeout)
}

// RDPVersionContext bounds supplemental identification by the probe and task deadlines.
func RDPVersionContext(ctx context.Context, addr string, timeout time.Duration) (_ string, _ []string, finalResult error) {
	defer func() {
		if err := recover(); err != nil {
			log.Errorf("extrafp to rdp failed: %v", err)
		}
	}()
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := netx.DialX(addr, netx.DialX_WithTimeout(timeout), netx.DialX_WithContext(ctx), netx.DialX_WithProxy())
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, utils.Errorf("rdp to addr failed: %s", err)
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	var b, _ = codec.DecodeHex("030000130ee000000000000100080003000000")
	if _, err := conn.Write(b); err != nil {
		if ctx.Err() != nil {
			return "", nil, ctx.Err()
		}
		return "", nil, err
	}
	buf := utils.StableReader(io.LimitReader(conn, 4096), timeout, 4096)
	if ctx.Err() != nil {
		return "", nil, ctx.Err()
	}

	// RDP
	if bytes.HasPrefix(buf, []byte{0x03, 0x00, 0x00}) {
		if bytes.HasPrefix(buf, os______old) {
			return "Windows Server 2003 or before", []string{
				"cpe:2.3:o:microsoft:windows:2003:*",
				"cpe:2.3:a:microsoft:remote_desktop",
			}, nil
		} else if bytes.HasPrefix(buf, os___xrdp_1) ||
			bytes.HasPrefix(buf, os___xrdp_2) ||
			bytes.HasPrefix(buf, os___xrdp_3) ||
			bytes.HasPrefix(buf, os___xrdp_4) {
			// xrdp
			return "xrdp", []string{
				"cpe:2.3:a:neutrinolabs:xrdp:*",
			}, nil
		} else if bytes.HasPrefix(buf, os___2008_1) ||
			bytes.HasPrefix(buf, os___2008_2) ||
			bytes.HasPrefix(buf, os___2008_3) {
			return "Windows Server 2008 [R2] [Standard/Enterprise/Datacenter]", []string{
				"cpe:2.3:o:microsoft:windows:server_2008:r2",
				"cpe:2.3:a:microsoft:remote_desktop",
			}, nil
		} else if bytes.HasPrefix(buf, os___2012_1) ||
			bytes.HasPrefix(buf, os___2012_2) ||
			bytes.HasPrefix(buf, os__2012_r2) {
			return "Windows Server 2012 [R2]", []string{
				"cpe:2.3:o:microsoft:windows:server_2012:r2",
				"cpe:2.3:a:microsoft:remote_desktop",
			}, nil
		} else if bytes.HasPrefix(buf, os__Vista_1) {
			return "Windows Vista or later", []string{"cpe:2.3:o:microsoft:windows:vista", "cpe:2.3:a:microsoft:remote_desktop"}, nil
		} else if bytes.HasPrefix(buf, os_Multiple_1) ||
			bytes.HasPrefix(buf, os_Multiple_2) {
			return "Windows [7/8/10/Server] 2003/2008/2012 [R2] [Standard/Enterprise] [x64] Edition", []string{
				"cpe:2.3:o:microsoft:windows:*",
				"cpe:2.3:a:microsoft:remote_desktop",
			}, nil
		} else {
			return strconv.Quote(string(buf)), nil, nil
		}
	}
	return "", nil, utils.Errorf("cannot find fp for rdp")
}
