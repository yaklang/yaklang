package loop_ssa_api_discovery

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHarvestSpringFromJavaFile_ClassAndMethodMapping(t *testing.T) {
	src := `package com.example;
@RequestMapping("/api/admin")
public class LoginAdminController {

    @PostMapping("/login")
    public String login() { return ""; }

    @GetMapping(value = "/status")
    public String st() { return ""; }
}
`
	eps := harvestSpringFromJavaFile([]byte(src), "com.example", "Login.java")
	require.GreaterOrEqual(t, len(eps), 2)
	paths := make(map[string]string)
	for _, e := range eps {
		paths[e.PathPattern] = e.Method
	}
	require.Equal(t, "POST", paths["/api/admin/login"])
	require.Equal(t, "GET", paths["/api/admin/status"])
}

func TestHarvestSpringFromJavaFile_BareGetMapping(t *testing.T) {
	src := `package com.bench.sqli.controller;
@RestController
@RequestMapping("/api/products")
public class ProductController {
    @GetMapping
    public String listProducts() { return ""; }
}
`
	eps := harvestSpringFromJavaFile([]byte(src), "com.bench.sqli.controller", "ProductController.java")
	require.GreaterOrEqual(t, len(eps), 1)
	require.Equal(t, "GET", eps[0].Method)
	require.Equal(t, "/api/products", eps[0].PathPattern)
}

func TestHarvestSpringFromJavaFile_DedupeRepeatedSegment(t *testing.T) {
	src := `package com.example;
@RequestMapping("/dict/save")
public class DictAdminController {
    @PostMapping("save")
    public String save() { return ""; }
}
`
	eps := harvestSpringFromJavaFile([]byte(src), "com.example", "Dict.java")
	require.GreaterOrEqual(t, len(eps), 1)
	require.Equal(t, "/dict/save", eps[0].PathPattern)
}

func TestJoinCatalogPath_SkipsUnknown(t *testing.T) {
	got := joinCatalogPath("unknown", "/admin", "/api/list")
	require.Equal(t, "/admin/api/list", got)
}

func TestCatalogReadyForProbe_RejectsUnknownURLs(t *testing.T) {
	c := &ApiCatalogV1{
		ContextPath:   "unknown",
		AssemblyBasis: "inferred",
		Entries: []ApiCatalogEntry{
			{FullURL: "http://host/unknown/foo"},
			{FullURL: "http://host/unknown/bar"},
		},
	}
	ok, reason := catalogReadyForProbe(c)
	require.False(t, ok)
	require.NotEmpty(t, reason)
}

func TestRouteKeyNormalization(t *testing.T) {
	k1 := routeKey("get", "/foo/")
	k2 := routeKey("GET", "/foo")
	require.Equal(t, k1, k2)
	require.True(t, strings.Contains(k1, "\x00"))
}
