// Package recv
// @Author bcy2007  2023/9/22 11:44
package recv

type PacketMessage struct {
	IPAddress string   `json:"ip"`
	MD5       []string `json:"md5,omitempty"`
}
