package subdomain

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
)

type SubdomainResult struct {
	FromTarget    string
	FromDNSServer string
	FromModeRaw   int

	IP     string
	Domain string

	// Tag 用于存储一些其他信息
	// 比如数据源之类的
	Tags []string

	// Aborted 标记此条为“扫描中止哨兵”，表示爆破因 DNS 被劫持/接管而中止。
	// 为 true 时调用方应读取 AbortReason 并向用户报错（如 yakit.Error），
	// 此条不携带真实子域名信息。仅由 OnScanAborted 路径推入结果 channel。
	Aborted bool

	// AbortReason 是中止原因的描述，仅在 Aborted=true 时有意义。
	AbortReason string
}

func (s *SubdomainResult) Hash() string {
	bs := md5.Sum([]byte(fmt.Sprintf("%v:%v:%v", s.IP, s.Domain, s.FromModeRaw)))
	return hex.EncodeToString(bs[:])
}

func (s *SubdomainResult) ToString() string {
	return fmt.Sprintf("%48s IP:[%15s] From:%v", s.Domain, s.IP, s.Tags)
}

func (s *SubdomainResult) Show() {
	println(s.ToString())
}
