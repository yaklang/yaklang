package fuzztagx

type BuildInTagFun func(s string) []string

var BuildInTag *map[string]BuildInTagFun
