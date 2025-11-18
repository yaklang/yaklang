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
	"CreatePrivilegedDevice":        _createPrivilegedDevice,
	"CreatePrivilegedDeviceWithMTU": _createPrivilegedDeviceWithMTU,
	"NewVMFromDevice":               _newVMFromDevice,
	"NewVMFromDeviceWithContext":    _newVMFromDeviceWithContext,
	"GetSystemRouteManager":         netstackvm.GetSystemRouteManager,
}

// _createPrivilegedDevice creates a privileged TUN device with default MTU (1500)
func _createPrivilegedDevice() (lowtun.Device, error) {
	device, _, err := lowtun.CreatePrivilegedDevice(1500)
	if err != nil {
		return nil, utils.Errorf("failed to create privileged device: %v", err)
	}
	return device, nil
}

// _createPrivilegedDeviceWithMTU creates a privileged TUN device with specified MTU
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

// StartForwarding starts forwarding TCP connections to the provided channel
// The channel should be created in yaklang script using: connChan = make(chan any)
// This channel can then be passed to MITM's extraIncomingConn option
func (vm *NetstackVM) StartForwarding(ch interface{}) error {
	log.Infof("StartForwarding called, vm=%v, listener=%v, ch=%v", vm != nil, vm.listener != nil, ch != nil)
	if vm == nil {
		return utils.Errorf("VM is nil")
	}
	if vm.listener == nil {
		return utils.Errorf("listener not initialized")
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
	if vm == nil {
		return utils.Errorf("VM is nil")
	}
	if vm.listener == nil {
		return utils.Errorf("listener not initialized")
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

// _newVMFromDevice creates a network stack virtual machine from a TUN device
// The VM will hijack TCP connections and make them available via GetTCPConnChan()
func _newVMFromDevice(device lowtun.Device) (*NetstackVM, error) {
	return _newVMFromDeviceWithContext(context.Background(), device)
}

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
