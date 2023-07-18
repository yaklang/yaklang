package embed

import "embed"

//go:embed data dataex
var FS embed.FS

func Asset(name string) ([]byte, error) {
	return FS.ReadFile(name)
}

func AssetDir(name string) ([]string, error) {
	dir, err := FS.ReadDir(name)
	if err != nil {
		return nil, err
	}
	entries := make([]string, 0, len(dir))
	for _, v := range dir {
		entries = append(entries, v.Name())
	}
	return entries, nil
}
