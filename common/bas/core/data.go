// Package core
// @Author bcy2007  2023/9/18 10:50
package core

import (
	"errors"
	"github.com/yaklang/yaklang/common/utils"

	//"packet/utils"
	"strconv"
	"strings"
)

const (
	EthernetLength     = 14
	NetworkLayerCheckA = 13
	NetworkLayerCheckB = 14
	IPv4CheckA         = 0x08
	IPv4CheckB         = 0x00
	IPv4SrcStartPos    = 13
	IPv4DstStartPos    = 17
	IPv4CheckSumA      = 11
	IPv4CheckSumB      = 12
	IPv6CheckA         = 0x86
	IPv6CheckB         = 0xdd

	IPv4LengthCheck         = 1
	IPv4TransportLayerCheck = 10
	UdpCheck                = 0x11
	TcpCheck                = 0x06
	IcmpCheck               = 0x01

	TcpLengthCheck = 13

	UdpLength = 8

	unknown = "unknown"
)

func CalculateIPv4Length(flag byte) int {
	byteLength := flag & 0x0f
	return int(byteLength) * 4
}

func CalculateTCPLength(flag byte) int {
	byteLength := flag & 0xf0
	return int(byteLength) / 16 * 4
}

func networkLayerCheck(a, b byte) string {
	if a == IPv4CheckA && b == IPv4CheckB {
		return "IPv4"
	} else {
		return unknown
	}
}

func transportLayerCheck(flag byte) string {
	if flag == UdpCheck {
		return "UDP"
	} else if flag == TcpCheck {
		return "TCP"
	} else {
		return unknown
	}
}

func networkLayerDo(layerName string, data []byte, beforeLength int) (int, string) {
	if layerName == "IPv4" {
		return IPv4Do(data, beforeLength)
	}
	return 0, unknown
}

func IPv4Do(data []byte, beforeLength int) (int, string) {
	dataLength := len(data)
	if dataLength < beforeLength+IPv4TransportLayerCheck {
		return 0, unknown
	}
	lengthFlag := data[beforeLength+IPv4LengthCheck-1]
	length := CalculateIPv4Length(lengthFlag)
	transportLayerFlag := data[beforeLength+IPv4TransportLayerCheck-1]
	transportLayer := transportLayerCheck(transportLayerFlag)
	return length, transportLayer
}

func transportLayerDo(layerName string, data []byte, beforeLength int) int {
	if layerName == "TCP" {
		return TCPDo(data, beforeLength)
	} else if layerName == "UDP" {
		return UDPDo(data, beforeLength)
	} else {
		return 0
	}
}

func TCPDo(data []byte, beforeLength int) int {
	dataLength := len(data)
	if dataLength < beforeLength+TcpLengthCheck {
		return 0
	}
	data[beforeLength+3-1] = 0xf7
	data[beforeLength+4-1] = 0x3f
	lengthFlag := data[beforeLength+TcpLengthCheck-1]
	length := CalculateTCPLength(lengthFlag)
	return length
}

func UDPDo(_ []byte, _ int) int {
	return UdpLength
}

func PacketDataAnalysis(traffic []byte) ([]byte, error) {
	dataLength := len(traffic)
	if dataLength < NetworkLayerCheckB {
		return nil, errors.New("traffic length error")
	}
	networkLayer := networkLayerCheck(traffic[NetworkLayerCheckA-1], traffic[NetworkLayerCheckB-1])
	if networkLayer == unknown {
		return nil, errors.New("network layer unknown")

	}
	networkLength, transportLayer := networkLayerDo(networkLayer, traffic, EthernetLength)
	if transportLayer == unknown {
		return nil, errors.New("transport layer unknown")

	}
	transportLength := transportLayerDo(transportLayer, traffic, EthernetLength+networkLength)
	if transportLength == 0 {
		return nil, errors.New("transport layer length unknown")

	}
	if dataLength < EthernetLength+networkLength+transportLength {
		return nil, errors.New("traffic length error")
	}
	result := traffic[EthernetLength+networkLength+transportLength:]
	return result, nil
}

func ParseIPAddressToByte(ipaddress string) ([]byte, error) {
	result := make([]byte, 0)
	items := strings.Split(ipaddress, ".")
	if len(items) != 4 {
		return result, utils.Error("length error")
	}
	for _, item := range items {
		num, err := strconv.Atoi(item)
		if err != nil {
			return result, utils.Errorf("a to i error: %v", err)
		}
		result = append(result, byte(num))
	}
	return result, nil
}
