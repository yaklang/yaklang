package netstack_exports

import (
	"context"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"net"
	"reflect"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/netstackvm"
	"github.com/yaklang/yaklang/common/utils"
)

// Exports provides yaklang bindings for netstack functionality
var Exports = map[string]interface{}{
	"CreatePrivilegedDevice":          _createPrivilegedDevice,
	"CreatePrivilegedDeviceWithMTU":   _createPrivilegedDeviceWithMTU,
	"NewVMFromDevice":                 _newVMFromDevice,
	"NewVMFromDeviceWithContext":      _newVMFromDeviceWithContext,
	"GetSystemRouteManager":           netstackvm.GetSystemRouteManager,
	"GetPrivilegedSystemRouteManager": netstackvm.GetPrivilegedSystemRouteManager,
	"FastKillTCP":                     netstackvm.FastKillTCP,
}

// CreatePrivilegedDevice 创建一个使用默认 MTU(1500) 的特权 TUN 虚拟网卡（导出名为 netstack.CreatePrivilegedDevice）
// 需要管理员/root 权限；返回的设备可交给 netstack.NewVMFromDevice 构建网络栈虚拟机
//
// 返回值:
//   - TUN 设备对象
//   - 错误信息（权限不足或创建失败时返回）
//
// Example:
// ```
// // 真实功能示例：创建 TUN 设备并构建网络栈虚拟机（需要 root 权限，示意性用法）
// device = netstack.CreatePrivilegedDevice()~
// vm = netstack.NewVMFromDevice(device)~
// println("tunnel:", vm.GetTunnelName())
// defer vm.Close()
// ```
func _createPrivilegedDevice() (lowtun.Device, error) {
	device, _, err := lowtun.CreatePrivilegedDevice(1500)
	if err != nil {
		return nil, utils.Errorf("failed to create privileged device: %v", err)
	}
	return device, nil
}

// CreatePrivilegedDeviceWithMTU 创建一个使用指定 MTU 的特权 TUN 虚拟网卡（导出名为 netstack.CreatePrivilegedDeviceWithMTU）
// 需要管理员/root 权限；MTU 取值范围为 1 到 9000
//
// 参数:
//   - mtu: 最大传输单元，常用 1500，巨帧场景可设更大（不超过 9000）
//
// 返回值:
//   - TUN 设备对象
//   - 错误信息（MTU 非法、权限不足或创建失败时返回）
//
// Example:
// ```
// // 真实功能示例：以 1400 的 MTU 创建 TUN 设备（需要 root 权限，示意性用法）
// device = netstack.CreatePrivilegedDeviceWithMTU(1400)~
// vm = netstack.NewVMFromDevice(device)~
// defer vm.Close()
// ```
func _createPrivilegedDeviceWithMTU(mtu int) (lowtun.Device, error) {
	if mtu <= 0 || mtu > 9000 {
		return nil, utils.Errorf("invalid MTU value: %d (must be between 1 and 9000)", mtu)
	}
	device, _, err := lowtun.CreatePrivilegedDevice(mtu)
	if err != nil {
		return nil, utils.Errorf("failed to create privileged device with MTU %d: %v", mtu, err)
	}
	return device, nil
}

// NetstackVM wraps TunVirtualMachine and provides methods for yaklang
type NetstackVM struct {
	tvm      *netstackvm.TunVirtualMachine
	listener *netstackvm.TunSpoofingListener
	ctx      context.Context
	cancel   context.CancelFunc
}

func (vm *NetstackVM) startForwardCheck() error {
	if vm == nil {
		return utils.Errorf("VM is nil")
	}
	if vm.listener == nil {
		return utils.Errorf("listener not initialized")
	}

	return nil
}

// StartForwarding starts forwarding TCP connections to the provided channel
// The channel should be created in yaklang script using: connChan = make(chan any)
// This channel can then be passed to MITM's extraIncomingConn option
func (vm *NetstackVM) StartForwarding(ch interface{}) error {
	if err := vm.startForwardCheck(); err != nil {
		return err
	}
	if ch == nil {
		return utils.Errorf("channel cannot be nil")
	}

	log.Infof("Starting TCP connection forwarding to channel (type: %T)...", ch)

	// Start a goroutine to accept connections and forward them to the channel
	go func() {
		for {
			conn, err := vm.listener.Accept()
			if err != nil {
				if err != net.ErrClosed {
					log.Errorf("error accepting connection: %v", err)
				}
				return
			}

			// Use reflection to send to the channel (works with both chan net.Conn and chan interface{})
			chValue := reflect.ValueOf(ch)
			if chValue.Kind() != reflect.Chan {
				log.Errorf("provided value is not a channel: %T", ch)
				conn.Close()
				return
			}

			connValue := reflect.ValueOf(conn)
			select {
			case <-vm.ctx.Done():
				log.Info("VM context cancelled, stopping connection forwarding")
				conn.Close()
				return
			default:
				// Try to send with a timeout
				sent := false
				select {
				case <-vm.ctx.Done():
					conn.Close()
					return
				default:
					// Non-blocking send
					if chValue.TrySend(connValue) {
						sent = true
						log.Debugf("forwarded connection to channel")
					}
				}

				if !sent {
					log.Warn("connection channel full or closed, dropping connection")
					conn.Close()
				}
			}
		}
	}()

	log.Info("TCP connection forwarding started successfully")
	return nil
}

func (vm *NetstackVM) StartForwardingSafeChannel(ch *chanx.UnlimitedChan[net.Conn]) error {
	log.Infof("StartForwarding called, vm=%v, listener=%v, ch=%v", vm != nil, vm.listener != nil, ch != nil)
	if err := vm.startForwardCheck(); err != nil {
		return err
	}
	if ch == nil {
		return utils.Errorf("channel cannot be nil")
	}

	log.Infof("Starting TCP connection forwarding to channel (type: %T)...", ch)

	// Start a goroutine to accept connections and forward them to the channel
	go func() {
		for {
			conn, err := vm.listener.Accept()
			if err != nil {
				if err != net.ErrClosed {
					log.Errorf("error accepting connection: %v", err)
				}
				return
			}

			select {
			case <-vm.ctx.Done():
				log.Info("VM context cancelled, stopping connection forwarding")
				conn.Close()
				return
			default:
				// Try to send with a timeout
				select {
				case <-vm.ctx.Done():
					conn.Close()
					return
				default:
					// Non-blocking send
					ch.SafeFeed(conn)
				}
			}
		}
	}()

	log.Info("TCP connection forwarding started successfully")
	return nil
}

func (vm *NetstackVM) StartForwardingCallbackMode(handle func(conn net.Conn) error) error {
	if err := vm.startForwardCheck(); err != nil {
		return err
	}
	log.Infof("Starting TCP connection forwarding to callback function...")

	for {
		conn, err := vm.listener.Accept()
		if err != nil {
			if err != net.ErrClosed {
				log.Errorf("error accepting connection: %v", err)
			}
			return err
		}

		select {
		case <-vm.ctx.Done():
			log.Info("VM context cancelled, stopping connection forwarding")
			conn.Close()
			return vm.ctx.Err()
		default:
			// Try to send with a timeout
			select {
			case <-vm.ctx.Done():
				conn.Close()
				return vm.ctx.Err()
			default:
				// Non-blocking send
				if err := handle(conn); err != nil {
					log.Errorf("error in callback function: %v", err)
					conn.Close()
				}
			}
		}
	}
}

// Close closes the VM and all associated resources
func (vm *NetstackVM) Close() error {
	if vm.cancel != nil {
		vm.cancel()
	}
	if vm.listener != nil {
		vm.listener.Close()
	}
	if vm.tvm != nil {
		return vm.tvm.Close()
	}
	return nil
}

// GetTunnelName returns the name of the TUN device (e.g., "utun3")
func (vm *NetstackVM) GetTunnelName() string {
	if vm.tvm != nil {
		return vm.tvm.GetTunnelName()
	}
	return ""
}

// NewVMFromDevice 基于一个 TUN 设备创建网络栈虚拟机（导出名为 netstack.NewVMFromDevice）
// 该虚拟机会劫持流经 TUN 设备的 TCP 连接，可通过 StartForwarding 把连接转发到通道，进而交给 tcpmitm 处理
//
// 参数:
//   - device: 由 netstack.CreatePrivilegedDevice 等创建的 TUN 设备
//
// 返回值:
//   - 网络栈虚拟机对象（使用完毕需调用 Close 释放）
//   - 错误信息（设备为空或初始化失败时返回）
//
// Example:
// ```
// // 真实功能示例：劫持 TCP 连接并转发到通道供后续处理（需要 root 权限，示意性用法）
// device = netstack.CreatePrivilegedDevice()~
// vm = netstack.NewVMFromDevice(device)~
// defer vm.Close()
// connChan = make(chan any, 16)
// vm.StartForwarding(connChan)~
// ```
func _newVMFromDevice(device lowtun.Device) (*NetstackVM, error) {
	return _newVMFromDeviceWithContext(context.Background(), device)
}

// NewVMFromDeviceWithContext 基于 TUN 设备与自定义上下文创建网络栈虚拟机（导出名为 netstack.NewVMFromDeviceWithContext）
// 与 netstack.NewVMFromDevice 类似，但可通过上下文统一控制虚拟机生命周期（上下文取消即停止）
//
// 参数:
//   - ctx: 控制虚拟机生命周期的上下文
//   - device: 由 netstack.CreatePrivilegedDevice 等创建的 TUN 设备
//
// 返回值:
//   - 网络栈虚拟机对象（使用完毕需调用 Close 释放）
//   - 错误信息（设备为空或初始化失败时返回）
//
// Example:
// ```
// // 真实功能示例：用带超时的上下文限制虚拟机运行时长（需要 root 权限，示意性用法）
// ctx = context.WithTimeout(context.Background(), 60 * time.Second)
// device = netstack.CreatePrivilegedDevice()~
// vm = netstack.NewVMFromDeviceWithContext(ctx, device)~
// defer vm.Close()
// ```
func _newVMFromDeviceWithContext(ctx context.Context, device lowtun.Device) (*NetstackVM, error) {
	if device == nil {
		return nil, utils.Errorf("device cannot be nil")
	}

	ctx, cancel := context.WithCancel(ctx)

	// Create TUN virtual machine from device
	tvm, err := netstackvm.NewTunVirtualMachineFromDevice(ctx, device)
	if err != nil {
		cancel()
		return nil, utils.Errorf("failed to create TUN virtual machine: %v", err)
	}

	log.Infof("created TUN virtual machine, tunnel name: %s", tvm.GetTunnelName())

	// Get listener for TCP connections
	listener := tvm.GetListener()
	if listener == nil {
		cancel()
		tvm.Close()
		return nil, utils.Errorf("failed to get listener from TUN virtual machine")
	}

	log.Info("TUN virtual machine listener created successfully")

	vm := &NetstackVM{
		tvm:      tvm,
		listener: listener,
		ctx:      ctx,
		cancel:   cancel,
	}

	return vm, nil
}
