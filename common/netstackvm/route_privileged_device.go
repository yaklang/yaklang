package netstackvm

import (
	"encoding/json"
	"github.com/yaklang/yaklang/common/lowtun"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
)

type routePrivilegedDevice struct {
	device lowtun.Device
}

func (r *routePrivilegedDevice) modifyRoute(message *netutil.RouteModifyMessage) (*netutil.RouteModifyResult, error) {
	request, err := json.Marshal(message)
	if err != nil {
		return nil, err
	}
	rw, err := lowtun.ConvertTUNDeviceToReadWriter(r.device, 4)
	if err != nil {
		return nil, utils.Errorf("failed to convert tun device: %v", err)
	}

	_, err = rw.Write(request)
	if err != nil {
		return nil, utils.Errorf("failed to write request to route socks: %v", err)
	}
	mtu, err := r.device.MTU()
	if err != nil {
		mtu = 1400
	}

	rawResp := make([]byte, mtu)
	_, err = rw.Read(rawResp)
	if err != nil {
		return nil, utils.Errorf("failed to read resp: %v", err)
	}
	var resp netutil.RouteModifyResult

	err = json.Unmarshal(rawResp, &resp)
	if err != nil {
		return nil, utils.Errorf("failed to unmarshal response data: %v", err)
	}
	return &resp, nil
}

func (r *routePrivilegedDevice) BatchAddSpecificIPRouteToNetInterface(toAdd []string, tunName string) (*netutil.RouteModifyResult, error) {
	message := netutil.RouteModifyMessage{
		IpList:  toAdd,
		Action:  netutil.Action_Add,
		TunName: tunName,
	}
	return r.modifyRoute(&message)
}

func (r *routePrivilegedDevice) BatchDeleteSpecificIPRoute(ipList []string) (*netutil.RouteModifyResult, error) {
	message := netutil.RouteModifyMessage{
		IpList: ipList,
		Action: netutil.Action_Delete,
	}
	return r.modifyRoute(&message)
}
