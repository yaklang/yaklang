package main

import "net/http"

// Fixture: Gin/Echo style verb registration + net/http

type fakeRouter struct{}

func (fakeRouter) GET(path string, _ any) {}
func (fakeRouter) POST(path string, _ any) {}

func stubHTTP() {
	var r fakeRouter
	r.GET("/api/ping", nil)
	r.POST("/api/items", nil)
	http.HandleFunc("/health", func(http.ResponseWriter, *http.Request) {})
}
