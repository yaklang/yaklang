package browser

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func testExtensionBridgeCapabilityCatalog(
	t *testing.T,
	methods ...string,
) *ExtensionBridgeCapabilityCatalog {
	t.Helper()
	descriptors := make([]ExtensionBridgeCapabilityDescriptor, 0, len(methods))
	for _, method := range methods {
		descriptors = append(descriptors, ExtensionBridgeCapabilityDescriptor{
			Method: method, Domain: "page", Access: "read", Summary: "Test capability",
			Scopes: []string{"browser.tabs.read"}, TargetMode: "document", DefaultTimeoutMS: 20_000,
			ParamsSchema: json.RawMessage(`{
				"$schema":"http://json-schema.org/draft-07/schema#",
				"type":"object",
				"additionalProperties":false
			}`),
		})
	}
	catalog := &ExtensionBridgeCapabilityCatalog{
		Version:       extensionBridgeCapabilityCatalogVersion,
		SchemaDialect: extensionBridgeCapabilitySchemaDialect,
		Capabilities:  descriptors,
	}
	hash, err := extensionBridgeCapabilityCatalogHash(catalog)
	require.NoError(t, err)
	catalog.Hash = hash
	return catalog
}

func TestExtensionBridgeCapabilityCatalogValidation(t *testing.T) {
	catalog := testExtensionBridgeCapabilityCatalog(t, "browser.context", "browser.tabs")
	require.NoError(t, validateExtensionBridgeCapabilityCatalog(
		catalog,
		[]string{"browser.context", "browser.tabs"},
	))

	tampered := cloneExtensionBridgeCapabilityCatalog(catalog)
	tampered.Capabilities[0].Summary = "Tampered"
	require.ErrorContains(t, validateExtensionBridgeCapabilityCatalog(
		tampered,
		[]string{"browser.context", "browser.tabs"},
	), "hash mismatch")

	require.ErrorContains(t, validateExtensionBridgeCapabilityCatalog(
		catalog,
		[]string{"browser.context"},
	), "list and catalog differ")

	descriptor, ok := catalog.Capability("browser.context")
	require.True(t, ok)
	require.Equal(t, "browser.context", descriptor.Method)
	require.NoError(t, catalog.ValidateCapabilityParams("browser.context", map[string]interface{}{}))
	require.ErrorContains(t, catalog.ValidateCapabilityParams(
		"browser.context",
		map[string]interface{}{"unexpected": true},
	), "do not match schema")
	require.ErrorContains(t, catalog.ValidateCapabilityParams(
		"browser.missing",
		map[string]interface{}{},
	), "not declared")
}

func TestExtensionBridgeCapabilityParamsNormalizeGoStructsBeforeValidation(t *testing.T) {
	type transformPacket struct {
		URL        string `json:"url"`
		StatusCode int    `json:"statusCode,omitempty"`
	}
	type transformCall struct {
		ProfileID string          `json:"profileId"`
		Direction string          `json:"direction"`
		Packet    transformPacket `json:"packet"`
		Ignored   string          `json:"-"`
	}

	catalog := testExtensionBridgeCapabilityCatalog(t, "browser.transform.execute")
	catalog.Capabilities[0].ParamsSchema = json.RawMessage(`{
		"$schema":"http://json-schema.org/draft-07/schema#",
		"type":"object",
		"required":["profileId","direction","packet"],
		"properties":{
			"profileId":{"type":"string","minLength":1},
			"direction":{"enum":["request","response"]},
			"packet":{
				"type":"object",
				"required":["url"],
				"properties":{
					"url":{"type":"string","minLength":1},
					"statusCode":{"type":"integer"}
				},
				"additionalProperties":false
			}
		},
		"additionalProperties":false
	}`)
	hash, err := extensionBridgeCapabilityCatalogHash(catalog)
	require.NoError(t, err)
	catalog.Hash = hash
	require.NoError(t, validateExtensionBridgeCapabilityCatalog(
		catalog,
		[]string{"browser.transform.execute"},
	))

	require.NoError(t, catalog.ValidateCapabilityParams(
		"browser.transform.execute",
		transformCall{
			ProfileID: "profile-1",
			Direction: "request",
			Packet:    transformPacket{URL: "https://example.test/api", StatusCode: 200},
			Ignored:   "must not enter JSON validation",
		},
	))
	require.ErrorContains(t, catalog.ValidateCapabilityParams(
		"browser.transform.execute",
		transformCall{
			ProfileID: "profile-1",
			Direction: "invalid",
			Packet:    transformPacket{URL: "https://example.test/api"},
		},
	), "do not match schema")
}

func TestExtensionBridgeCapabilityCatalogAcceptsIsolationDomain(t *testing.T) {
	catalog := testExtensionBridgeCapabilityCatalog(t, "browser.isolation.inspect")
	catalog.Capabilities[0].Domain = "isolation"
	catalog.Capabilities[0].Scopes = []string{"browser.isolation.read"}
	catalog.Capabilities[0].TargetMode = "none"
	hash, err := extensionBridgeCapabilityCatalogHash(catalog)
	require.NoError(t, err)
	catalog.Hash = hash

	require.NoError(t, validateExtensionBridgeCapabilityCatalog(
		catalog,
		[]string{"browser.isolation.inspect"},
	))
}

func TestExtensionBridgeCapabilityCatalogAcceptsAuthorizationDomain(t *testing.T) {
	catalog := testExtensionBridgeCapabilityCatalog(t, "browser.authorization.context.capture")
	catalog.Capabilities[0].Domain = "authorization"
	catalog.Capabilities[0].Access = "sensitive-read"
	catalog.Capabilities[0].Scopes = []string{
		"browser.isolation.read",
		"browser.cookies.read",
		"browser.storage.read",
	}
	hash, err := extensionBridgeCapabilityCatalogHash(catalog)
	require.NoError(t, err)
	catalog.Hash = hash

	require.NoError(t, validateExtensionBridgeCapabilityCatalog(
		catalog,
		[]string{"browser.authorization.context.capture"},
	))
}

func TestExtensionBridgeCapabilityCatalogRejectsInvalidSchema(t *testing.T) {
	catalog := testExtensionBridgeCapabilityCatalog(t, "browser.context")
	catalog.Capabilities[0].ParamsSchema = json.RawMessage(`{"type":"not-a-json-schema-type"}`)
	hash, err := extensionBridgeCapabilityCatalogHash(catalog)
	require.NoError(t, err)
	catalog.Hash = hash
	require.ErrorContains(t, validateExtensionBridgeCapabilityCatalog(
		catalog,
		[]string{"browser.context"},
	), "compile browser capability")
}

func TestExtensionBridgeCapabilityCatalogHashMatchesBrowserVector(t *testing.T) {
	catalog := &ExtensionBridgeCapabilityCatalog{
		Version:       1,
		SchemaDialect: "http://json-schema.org/draft-07/schema#",
		Capabilities: []ExtensionBridgeCapabilityDescriptor{{
			Method: "browser.tabs", Domain: "page", Access: "read", Summary: "List tabs",
			Scopes: []string{"browser.tabs.read"}, TargetMode: "none", DefaultTimeoutMS: 20_000,
			ParamsSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}},
	}
	hash, err := extensionBridgeCapabilityCatalogHash(catalog)
	require.NoError(t, err)
	require.Equal(t, "82bc1210338e773137b95359ca0b39c0443bae3d9fdae33f97942cf82119006f", hash)
}
