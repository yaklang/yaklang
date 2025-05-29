package mcp

import (
	"maps"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/mcp"
	"github.com/yaklang/yaklang/common/mcp/mcp-go/server"
)

type ResourceWithHandler struct {
	resource         *mcp.Resource
	resourceTemplate *mcp.ResourceTemplate
	handler          ResourceHandlerWrapperFunc
}

var (
	globalResources    = make(map[string]*ResourceWithHandler, 0)
	globalResourceSets = make(map[string]*ResourceSet, 0)
)

type ResourceSet struct {
	Resources map[string]*ResourceWithHandler
}

type ResourceSetOption func(*ResourceSet)
type ResourceHandlerWrapperFunc func(*MCPServer) server.ResourceHandlerFunc

func WithResource(resource *mcp.Resource, handler ResourceHandlerWrapperFunc) ResourceSetOption {
	return func(b *ResourceSet) {
		b.Resources[resource.Name] = &ResourceWithHandler{
			resource: resource,
			handler:  handler,
		}
	}
}

func WithResourceTemplate(resource *mcp.ResourceTemplate, handler ResourceHandlerWrapperFunc) ResourceSetOption {
	return func(b *ResourceSet) {
		b.Resources[resource.Name] = &ResourceWithHandler{
			resourceTemplate: resource,
			handler:          handler,
		}
	}
}

func AddGlobalResourceSet(setName string, opts ...ResourceSetOption) {
	b := &ResourceSet{
		Resources: make(map[string]*ResourceWithHandler),
	}
	for _, opt := range opts {
		opt(b)
	}

	globalResourceSets[setName] = b
	maps.Copy(globalResources, b.Resources)
}

func GlobalResourceSetList() []string {
	return lo.Keys(globalResourceSets)
}
