// Package config
// https://github.com/unknwon/goconfig
package config

// deepCopy will copy a new map with different address
func deepCopy(d map[string]string) map[string]string {
	rs := make(map[string]string)
	for k, v := range d {
		rs[k] = v
	}

	return rs
}
