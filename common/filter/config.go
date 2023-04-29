package filter

type Config struct {
	TTL           int64 `json:"ttl" yaml:"ttl"`                       // 去重时间窗，秒为单位
	CaseSensitive bool  `json:"case_sensitive" yaml:"case_sensitive"` // 是否对 URL 中的大小写敏感
	Hash          bool  `json:"hash" yaml:"hash"`                     // 是否将 URL hash 作为去重因素
	Query         bool  `json:"query" yaml:"query"`                   // 是否将 query 作为去重因素, 若为 true， 那么 ?a=b 和  ?a=c 将被视为两个链接
	Credential    bool  `json:"credential" yaml:"credential"`         // 是否将认证信息作为去重因素，若为 true, 登录前后的同一页面可能会被扫描两次
	Rewrite       bool  `json:"rewrite" yaml:"rewrite"`               // 是否启用 rewrite 识别
}

func (c *Config) Clone() *Config {
	newC := *c
	return &newC
}

func NewDefaultConfig() *Config {
	return &Config{Credential: true}
}
