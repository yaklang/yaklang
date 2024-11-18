package pcaputil

import (
	"errors"
	"fmt"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/samber/lo"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
	"net"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const WIN_DEV_LOOP = "\\Device\\NPF_Loopback"

func PcapInterfaceEqNetInterface(piface pcap.Interface, iface *net.Interface) bool {
	// 如果 windows \Device\NPF_Loopback 网卡没 IP address，比如 npcap-1.60版本的，安装后默认没 IP address
	// 同时安装完成后需要重启电脑，不然会报错\Device\NPF_Loopback: driver error: not enough memory to allocate the kernel buffer
	// Mock IP addresses
	if piface.Name == WIN_DEV_LOOP && len(piface.Addresses) == 0 {
		piface.Addresses = append(piface.Addresses, pcap.InterfaceAddress{
			IP: net.IPv4(127, 0, 0, 1),
		})
		piface.Addresses = append(piface.Addresses, pcap.InterfaceAddress{
			IP: net.IPv6loopback,
		})
	}

	addrs, err := iface.Addrs()
	if err != nil {
		log.Errorf("fetch iface[%v] addrs failed: %s", iface.Name, err)
		return false
	}

	var pIfaceAddrs []string
	var ifaceAddrs []string

	for _, addr := range piface.Addresses {
		pIfaceAddrs = append(pIfaceAddrs, addr.IP.String())
	}

	for _, addr := range addrs {
		ipValue, _, err := net.ParseCIDR(addr.String())
		if err != nil {
			continue
		}
		ifaceAddrs = append(ifaceAddrs, ipValue.String())
	}

	if pIfaceAddrs == nil || ifaceAddrs == nil {
		log.Debugf("no iIfaceAddrs[pcap:%v] or ifaceAddrs[net:%v]", piface.Name, iface.Name)
		return false
	}

	sort.Strings(pIfaceAddrs)
	sort.Strings(ifaceAddrs)
	return utils.CalcSha1(strings.Join(pIfaceAddrs, "|")) == utils.CalcSha1(strings.Join(ifaceAddrs, "|"))
}

type ConvertIfaceNameError struct {
	name string
}

func (e *ConvertIfaceNameError) Error() string {
	return fmt.Sprintf("convert iface name failed: %s", e.name)
}

func NewConvertIfaceNameError(name string) *ConvertIfaceNameError {
	return &ConvertIfaceNameError{
		name: name,
	}
}

var cachedFindAllDevs = utils.CacheFunc(60, pcap.FindAllDevs)

func IfaceNameToPcapIfaceName(name string) (string, error) {
	devs, err := cachedFindAllDevs()
	if err != nil {
		return "", utils.Errorf("find pcap dev failed: %s", err)
	}

	for _, dev := range devs {
		if dev.Name == name {
			return name, nil
		}
	}

	iface, err := net.InterfaceByName(name)
	if err != nil {
		return "", utils.Errorf("fetch net.Interface failed: %s", err)
	}

	for _, dev := range devs {
		if PcapInterfaceEqNetInterface(dev, iface) {
			return dev.Name, nil
		}
	}
	return "", NewConvertIfaceNameError(name)
}

func PcapIfaceNameToNetInterface(ifaceName string) (*net.Interface, error) {
	devs, err := cachedFindAllDevs()
	if err != nil {
		return nil, utils.Errorf("find pcap dev failed: %s", err)
	}
	for _, dev := range devs {
		if dev.Name == ifaceName {
			// windows 下的 pcap dev name 与 net.Interface.Name 不一致
			if runtime.GOOS == "windows" {
				var ifaceIP string
				if len(dev.Addresses) == 0 {
					if dev.Name == WIN_DEV_LOOP {
						ifaceIP = net.IPv4(127, 0, 0, 1).String()
					}
				} else {
					ifaceIP = dev.Addresses[0].IP.String()
				}
				if ifaceIP == "" {
					return nil, utils.Errorf("no iface ip found: %s", ifaceName)
				}
				iface, err := netutil.FindInterfaceByIP(ifaceIP)
				if err != nil {
					return nil, utils.Errorf("fetch net.Interface failed: %s", err)
				}
				if PcapInterfaceEqNetInterface(dev, &iface) {
					return &iface, nil
				}
			} else {
				iface, err := net.InterfaceByName(dev.Name)
				if err != nil {
					return nil, utils.Errorf("fetch net.Interface failed: %s", err)
				}
				if PcapInterfaceEqNetInterface(dev, iface) {
					return iface, nil
				}
			}
		}
	}
	return nil, utils.Errorf("no iface found: %s", ifaceName)
}

func AllDevices() []*pcap.Interface {
	ifs, err := pcap.FindAllDevs()
	if err != nil {
		log.Errorf("find pcap dev failed: %s", err)
	}
	return lo.Map(ifs, func(item pcap.Interface, index int) *pcap.Interface {
		return &item
	})
}

func GetLoopBackNetInterface() (*net.Interface, error) {
	var localIfaceName string

	for _, d := range AllDevices() { // 尝试获取本地回环网卡
		utils.Debug(func() {
			log.Debugf("\nDEVICE: %v\nDESC: %v\nFLAGS: %v\n", d.Name, d.Description, net.Flags(d.Flags).String())
		})

		// 先获取地址 loopback
		for _, addr := range d.Addresses {
			if addr.IP.IsLoopback() {
				localIfaceName = d.Name
				log.Debugf("fetch loopback by addr: %v", d.Name)
				break
			}
		}
		if localIfaceName != "" {
			break
		}

		// 默认 desc 获取 loopback
		if strings.Contains(strings.ToLower(d.Description), "adapter for loopback traffic capture") {
			log.Infof("found loopback by desc: %v", d.Name)
			localIfaceName = d.Name
			break
		}

		// 获取 flags
		if net.Flags(uint(d.Flags))&net.FlagLoopback == 1 {
			log.Infof("found loopback by flag: %v", d.Name)
			localIfaceName = d.Name
			break
		}
	}
	return PcapIfaceNameToNetInterface(localIfaceName)
}

func GetPcapInterfaceByIndex(i int) (*pcap.Interface, error) {
	devs, err := cachedFindAllDevs()
	if err != nil {
		return nil, utils.Errorf("find pcap dev failed: %s", err)
	}
	if i < 0 || i >= len(devs) {
		return nil, utils.Errorf("index out of range: %d", i)
	}
	return &devs[i], nil
}

func GetPublicInternetPcapHandler() (*pcap.Handle, error) {
	iface, _, _, err := netutil.GetPublicRoute()
	if err != nil {
		return nil, err
	}
	ifaceName, err := IfaceNameToPcapIfaceName(iface.Name)
	if err != nil {
		return nil, err
	}
	return OpenIfaceLive(ifaceName)
}

func OpenFile(filename string) (*pcap.Handle, error) {
	handler, err := pcap.OpenOffline(filename)
	if err != nil {
		return nil, utils.Errorf("pcap.OpenOffline failed: %s", err)
	}
	return handler, nil
}

type OpenIfaceLiveOptions struct {
	SnapLen int32
	Promisc bool
	Timeout time.Duration
}

func DefaultOpenIfaceLiveOptions() []LiveConfig {
	return []LiveConfig{
		WithSnapLen(65535),
		WithPromisc(true),
		WithTimeout(pcap.BlockForever),
	}
}

type LiveConfig func(*OpenIfaceLiveOptions)

func WithSnapLen(snapLen int32) LiveConfig {
	return func(options *OpenIfaceLiveOptions) {
		if snapLen == 0 {
			options.SnapLen = 65535
		}
		options.SnapLen = snapLen
	}
}

func WithPromisc(promisc bool) LiveConfig {
	return func(options *OpenIfaceLiveOptions) {
		options.Promisc = promisc
	}
}

func WithTimeout(timeout time.Duration) LiveConfig {
	return func(options *OpenIfaceLiveOptions) {
		if timeout == 0 {
			options.Timeout = pcap.BlockForever
		}
		options.Timeout = timeout
	}
}

func OpenIfaceLive(iface string, opts ...LiveConfig) (*pcap.Handle, error) {
	options := &OpenIfaceLiveOptions{}
	for _, opt := range opts {
		opt(options)
	}
	handler, err := pcap.OpenLive(iface, options.SnapLen, options.Promisc, options.Timeout)
	if err != nil {
		return nil, utils.Errorf("pcap.OpenLive %s failed: %v", iface, err)
	}
	log.Infof("open iface %s success", iface)
	return handler, nil
}

type PcapHandleWrapper struct {
	handle     *pcap.Handle
	mutex      *sync.RWMutex
	isClose    bool
	isLoopback bool
}

func WrapPcapHandle(handle *pcap.Handle, isloop ...bool) *PcapHandleWrapper {
	isLoopback := false
	if len(isloop) > 0 {
		isLoopback = isloop[0]
	}
	return &PcapHandleWrapper{
		handle:     handle,
		mutex:      new(sync.RWMutex),
		isClose:    false,
		isLoopback: isLoopback,
	}
}

func (w *PcapHandleWrapper) IsLoopback() bool {
	return w.isLoopback
}

func (w *PcapHandleWrapper) WritePacketData(data []byte) error {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	if w.isClose {
		return utils.Errorf("handle is closed")
	}
	return w.handle.WritePacketData(data)
}

func (w *PcapHandleWrapper) ReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	w.mutex.RLock()
	defer w.mutex.RUnlock()
	if w.isClose {
		return nil, gopacket.CaptureInfo{}, utils.Errorf("handle is closed")
	}
	return w.handle.ReadPacketData()
}

func (w *PcapHandleWrapper) close() {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	if w.isClose {
		return
	}
	w.handle.Close()
	w.isClose = true
	return
}

func (w *PcapHandleWrapper) Error() (err error) {
	defer func() {
		if panicError := recover(); panicError != nil {
			err = utils.Error("pcap handler get erro panic")
		}
	}()

	if w.isClose {
		return utils.Error("handle is closed")
	}

	err = w.handle.Error()
	if err == nil {
		return nil
	}

	// 错误处理逻辑
	if runtime.GOOS == "windows" {
		errMsg := err.Error()
		if !utf8.ValidString(errMsg) {
			// 尝试转换错误信息编码
			info, convertErr := codec.GB18030ToUtf8([]byte(errMsg))
			if convertErr != nil {
				// 如果转换失败，返回转换错误
				return convertErr
			}
			// 如果转换成功，返回转换后的错误信息
			return utils.Wrapf(errors.New(string(info)), "pcap ifaceDevs")
		}
	}
	return err
}

func (w *PcapHandleWrapper) CompileBPFFilter(expr string) ([]pcap.BPFInstruction, error) {
	return w.handle.CompileBPFFilter(expr)
}

func (w *PcapHandleWrapper) LinkType() layers.LinkType {
	return w.handle.LinkType()
}

func (w *PcapHandleWrapper) ListDataLinks() ([]pcap.Datalink, error) {
	return w.handle.ListDataLinks()
}

func (w *PcapHandleWrapper) NewBPF(expr string) (*pcap.BPF, error) {
	return w.handle.NewBPF(expr)
}

func (w *PcapHandleWrapper) NewBPFInstructionFilter(bpfInstructions []pcap.BPFInstruction) (*pcap.BPF, error) {
	return w.handle.NewBPFInstructionFilter(bpfInstructions)
}

func (w *PcapHandleWrapper) Resolution() gopacket.TimestampResolution {
	return w.handle.Resolution()
}

func (w *PcapHandleWrapper) SetBPFFilter(expr string) error {
	return w.handle.SetBPFFilter(expr)
}

func (w *PcapHandleWrapper) SetBPFInstructionFilter(bpfInstructions []pcap.BPFInstruction) error {
	return w.handle.SetBPFInstructionFilter(bpfInstructions)
}

func (w *PcapHandleWrapper) SetDirection(direction pcap.Direction) error {
	return w.handle.SetDirection(direction)
}

func (w *PcapHandleWrapper) SetLinkType(linkType layers.LinkType) error {
	return w.handle.SetLinkType(linkType)
}

func (w *PcapHandleWrapper) Stats() (*pcap.Stats, error) {
	return w.handle.Stats()
}

func (w *PcapHandleWrapper) SnaLen() int {
	return w.handle.SnapLen()
}

func (w *PcapHandleWrapper) ZeroCopyReadPacketData() ([]byte, gopacket.CaptureInfo, error) {
	return w.handle.ZeroCopyReadPacketData()
}
