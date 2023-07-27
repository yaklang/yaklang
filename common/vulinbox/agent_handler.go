package vulinbox

import (
	"encoding/base64"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/vulinboxagentproto"
	"net"
	"time"
)

func handlePing(_ []byte) (any, error) {
	return nil, nil
}

func handleUDP(data []byte) (any, error) {
	udp := utils.MustUnmarshalJson[vulinboxagentproto.UDPAction](data)
	if udp == nil {
		return nil, nil
	}
	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(udp.Target))
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	bytes, err := base64.StdEncoding.DecodeString(udp.Content)
	if err != nil {
		return nil, err
	}

	if _, err = conn.Write(bytes); err != nil {
		return nil, err
	}

	if udp.WaitTimeout == 0 {
		return nil, nil
	}

	if err := conn.SetDeadline(time.Now().Add(udp.WaitTimeout)); err != nil {
		return nil, err
	}

	buf := make([]byte, 1024)
	_, _, err = conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}
	return buf, nil
}
