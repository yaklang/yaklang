package systemproxy

import "os"

/*
Get returns the current systemwide proxy settings.
*/
func Get() (Settings, error) {
	var result = os.Getenv("http_proxy")
	if result == "" {
		return Settings{
			Enabled:       false,
			DefaultServer: "",
		}, nil
	}
	return Settings{
		Enabled:       true,
		DefaultServer: result,
	}, nil
}

/*
Set updates systemwide proxy settings.
*/
func Set(s Settings) error {
	if !s.Enabled {
		os.Setenv("http_proxy", "")
		return nil
	}
	os.Setenv("http_proxy", s.DefaultServer)
	return nil
}
