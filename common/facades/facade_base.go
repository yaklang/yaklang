package facades

import (
	"fmt"
	uuid "github.com/satori/go.uuid"
	"yaklang.io/yaklang/common/utils"
)

type FacadeConnectionHandler func(conn *utils.BufferedPeekableConn) error

type Notification struct {
	// dns
	// http/s
	// rmi
	Type         string `json:"type"`
	RemoteAddr   string `json:"remote_addr"`
	Raw          []byte `json:"raw"`
	Token        string `json:"token"`
	Uuid         string `json:"uuid"`
	ResponseInfo string `json:"response_info"`
	ConnectHash  string `json:"connect_hash"`
}

func NewNotification(t string, remoteAddr string, raw []byte, token string) *Notification {
	return &Notification{
		Type:       t,
		RemoteAddr: remoteAddr,
		Raw:        raw,
		Token:      token,
		Uuid:       uuid.NewV4().String(),
	}
}

func (n *Notification) String() string {
	base := fmt.Sprintf("[%6s] from: %v", n.Type, n.RemoteAddr)
	if n.Token != "" {
		base = fmt.Sprintf("%v token: %s", base, n.Token)
	}
	return base
}
