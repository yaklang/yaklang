package netx

import "os"

/*
Get returns the current systemwide proxy settings.
*/
func GetSystemProxy() (SystemProxySetting, error) {
	var result = os.Getenv("http_proxy")
	if result == "" {
		return SystemProxySetting{
			Enabled:       false,
			DefaultServer: "",
		}, nil
	}
	return SystemProxySetting{
		Enabled:       true,
		DefaultServer: result,
	}, nil
}

/*
Set updates systemwide proxy settings.
*/
func SetSystemProxy(s SystemProxySetting) error {
	if !s.Enabled {
		os.Setenv("http_proxy", "")
		return nil
	}
	os.Setenv("http_proxy", s.DefaultServer)
	return nil
}
