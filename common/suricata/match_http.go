package suricata

type HTTPConfig struct {
	// deprecated
	Uricontent string

	UrilenOp   int
	UrilenNum1 int
	UrilenNum2 int
}

func httpMatcher(c *matchContext) error {
	if c.Rule.ContentRuleConfig == nil {
		return nil
	}
	panic("implement me")
}
