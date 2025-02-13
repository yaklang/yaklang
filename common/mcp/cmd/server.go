package main

import "github.com/yaklang/yaklang/common/mcp"

func main() {
	s := mcp.NewMCPServer()
	if err := s.ServeStdio(); err != nil {
		panic(err)
	}
}
