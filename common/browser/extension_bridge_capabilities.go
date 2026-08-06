package browser

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	extensionBridgeCapabilityCatalogVersion  = 1
	extensionBridgeCapabilitySchemaDialect   = "http://json-schema.org/draft-07/schema#"
	extensionBridgeMaxCapabilityCatalogBytes = 1 << 20
	extensionBridgeMaxCapabilitySchemaBytes  = 256 << 10
	extensionBridgeMaxCapabilities           = 128
)

var extensionBridgeCapabilityMethodPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,159}$`)

type ExtensionBridgeCapabilityScopeCondition struct {
	Scope string `json:"scope"`
	When  string `json:"when"`
}

type ExtensionBridgeCapabilityDescriptor struct {
	Method            string                                    `json:"method"`
	Domain            string                                    `json:"domain"`
	Access            string                                    `json:"access"`
	Summary           string                                    `json:"summary"`
	Scopes            []string                                  `json:"scopes"`
	ConditionalScopes []ExtensionBridgeCapabilityScopeCondition `json:"conditionalScopes,omitempty"`
	TargetMode        string                                    `json:"targetMode"`
	DefaultTimeoutMS  int                                       `json:"defaultTimeoutMs"`
	ParamsSchema      json.RawMessage                           `json:"paramsSchema"`
}

type ExtensionBridgeCapabilityCatalog struct {
	Version       int                                   `json:"version"`
	SchemaDialect string                                `json:"schemaDialect"`
	Hash          string                                `json:"hash"`
	Capabilities  []ExtensionBridgeCapabilityDescriptor `json:"capabilities"`
	compiled      map[string]*jsonschema.Schema
}

func cloneExtensionBridgeCapabilityCatalog(
	catalog *ExtensionBridgeCapabilityCatalog,
) *ExtensionBridgeCapabilityCatalog {
	if catalog == nil {
		return nil
	}
	clone := *catalog
	clone.Capabilities = make([]ExtensionBridgeCapabilityDescriptor, len(catalog.Capabilities))
	for index, descriptor := range catalog.Capabilities {
		clone.Capabilities[index] = descriptor
		clone.Capabilities[index].Scopes = append([]string(nil), descriptor.Scopes...)
		clone.Capabilities[index].ConditionalScopes = append(
			[]ExtensionBridgeCapabilityScopeCondition(nil),
			descriptor.ConditionalScopes...,
		)
		clone.Capabilities[index].ParamsSchema = append(json.RawMessage(nil), descriptor.ParamsSchema...)
	}
	return &clone
}

func normalizeExtensionBridgeJSONValue(value interface{}) (interface{}, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var normalized interface{}
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func canonicalExtensionBridgeJSON(value interface{}) ([]byte, error) {
	normalized, err := normalizeExtensionBridgeJSONValue(value)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(normalized); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buffer.Bytes()), nil
}

func extensionBridgeCapabilityCatalogHash(
	catalog *ExtensionBridgeCapabilityCatalog,
) (string, error) {
	payload := struct {
		Version       int                                   `json:"version"`
		SchemaDialect string                                `json:"schemaDialect"`
		Capabilities  []ExtensionBridgeCapabilityDescriptor `json:"capabilities"`
	}{
		Version:       catalog.Version,
		SchemaDialect: catalog.SchemaDialect,
		Capabilities:  catalog.Capabilities,
	}
	canonical, err := canonicalExtensionBridgeJSON(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func compileExtensionBridgeCapabilitySchema(
	method string,
	raw json.RawMessage,
) (*jsonschema.Schema, error) {
	if len(raw) == 0 || len(raw) > extensionBridgeMaxCapabilitySchemaBytes || !json.Valid(raw) {
		return nil, fmt.Errorf("browser capability %s has an invalid parameter schema", method)
	}
	var schema interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return nil, fmt.Errorf("decode browser capability %s parameter schema: %w", method, err)
	}
	if _, ok := schema.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("browser capability %s parameter schema must be an object", method)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft7)
	resource := "urn:yak-browser-capability:" + method
	if err := compiler.AddResource(resource, schema); err != nil {
		return nil, fmt.Errorf("register browser capability %s parameter schema: %w", method, err)
	}
	compiled, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("compile browser capability %s parameter schema: %w", method, err)
	}
	return compiled, nil
}

// Capability returns the descriptor advertised by the connected extension.
func (c *ExtensionBridgeCapabilityCatalog) Capability(
	method string,
) (ExtensionBridgeCapabilityDescriptor, bool) {
	if c == nil {
		return ExtensionBridgeCapabilityDescriptor{}, false
	}
	method = strings.TrimSpace(method)
	for _, descriptor := range c.Capabilities {
		if descriptor.Method == method {
			return descriptor, true
		}
	}
	return ExtensionBridgeCapabilityDescriptor{}, false
}

// ValidateCapabilityParams enforces the exact signed parameter schema before a
// capability is dispatched to the extension.
func (c *ExtensionBridgeCapabilityCatalog) ValidateCapabilityParams(
	method string,
	params interface{},
) error {
	descriptor, ok := c.Capability(method)
	if !ok {
		return fmt.Errorf("browser capability %q is not declared by the connected extension schema", method)
	}
	compiled := c.compiled[descriptor.Method]
	if compiled == nil {
		var err error
		compiled, err = compileExtensionBridgeCapabilitySchema(descriptor.Method, descriptor.ParamsSchema)
		if err != nil {
			return err
		}
	}
	normalized, err := normalizeExtensionBridgeJSONValue(params)
	if err != nil {
		return fmt.Errorf(
			"normalize browser capability %s parameters as JSON: %w",
			descriptor.Method,
			err,
		)
	}
	if err := compiled.Validate(normalized); err != nil {
		return fmt.Errorf(
			"browser capability %s parameters do not match schema %s: %w",
			descriptor.Method,
			c.Hash,
			err,
		)
	}
	return nil
}

func validateExtensionBridgeCapabilityCatalog(
	catalog *ExtensionBridgeCapabilityCatalog,
	advertised []string,
) error {
	if catalog == nil {
		return errors.New("browser extension capability catalog is required")
	}
	encoded, err := json.Marshal(catalog)
	if err != nil || len(encoded) > extensionBridgeMaxCapabilityCatalogBytes {
		return errors.New("browser extension capability catalog exceeds the size limit")
	}
	if catalog.Version != extensionBridgeCapabilityCatalogVersion {
		return fmt.Errorf("unsupported browser extension capability catalog version: %d", catalog.Version)
	}
	if catalog.SchemaDialect != extensionBridgeCapabilitySchemaDialect {
		return fmt.Errorf("unsupported browser extension capability schema dialect: %s", catalog.SchemaDialect)
	}
	if len(catalog.Capabilities) == 0 || len(catalog.Capabilities) > extensionBridgeMaxCapabilities {
		return errors.New("browser extension capability catalog has an invalid capability count")
	}
	expectedHash, err := extensionBridgeCapabilityCatalogHash(catalog)
	if err != nil {
		return fmt.Errorf("hash browser extension capability catalog: %w", err)
	}
	if len(catalog.Hash) != sha256.Size*2 || catalog.Hash != expectedHash {
		return errors.New("browser extension capability catalog hash mismatch")
	}

	advertisedSet := make(map[string]struct{}, len(advertised))
	for _, method := range advertised {
		method = strings.TrimSpace(method)
		if !extensionBridgeCapabilityMethodPattern.MatchString(method) {
			return fmt.Errorf("browser extension advertises an invalid capability: %q", method)
		}
		if _, duplicate := advertisedSet[method]; duplicate {
			return fmt.Errorf("browser extension advertises duplicate capability: %s", method)
		}
		advertisedSet[method] = struct{}{}
	}
	if len(advertisedSet) != len(catalog.Capabilities) {
		return errors.New("browser extension capability list and catalog differ")
	}

	seen := make(map[string]struct{}, len(catalog.Capabilities))
	allowedDomains := map[string]bool{
		"system": true, "page": true, "isolation": true, "authorization": true, "handoff": true, "network": true,
		"recording": true, "callable": true, "debugger": true, "transform": true, "proxy": true,
	}
	allowedAccess := map[string]bool{
		"read": true, "sensitive-read": true, "write": true,
		"control": true, "execute": true, "dangerous": true,
	}
	allowedTargets := map[string]bool{"none": true, "tab": true, "document": true, "profile": true}
	compiled := make(map[string]*jsonschema.Schema, len(catalog.Capabilities))
	for _, descriptor := range catalog.Capabilities {
		if !extensionBridgeCapabilityMethodPattern.MatchString(descriptor.Method) {
			return fmt.Errorf("browser capability method is invalid: %q", descriptor.Method)
		}
		if _, duplicate := seen[descriptor.Method]; duplicate {
			return fmt.Errorf("browser capability descriptor is duplicated: %s", descriptor.Method)
		}
		seen[descriptor.Method] = struct{}{}
		if _, ok := advertisedSet[descriptor.Method]; !ok {
			return fmt.Errorf("browser capability descriptor was not advertised: %s", descriptor.Method)
		}
		if !allowedDomains[descriptor.Domain] || !allowedAccess[descriptor.Access] || !allowedTargets[descriptor.TargetMode] {
			return fmt.Errorf("browser capability %s has invalid metadata", descriptor.Method)
		}
		if summary := strings.TrimSpace(descriptor.Summary); summary == "" || len(summary) > 500 {
			return fmt.Errorf("browser capability %s has an invalid summary", descriptor.Method)
		}
		if descriptor.DefaultTimeoutMS < 250 || descriptor.DefaultTimeoutMS > 120_000 {
			return fmt.Errorf("browser capability %s has an invalid timeout", descriptor.Method)
		}
		if len(descriptor.Scopes) > 16 || len(descriptor.ConditionalScopes) > 16 {
			return fmt.Errorf("browser capability %s declares too many scopes", descriptor.Method)
		}
		for _, scope := range descriptor.Scopes {
			if !extensionBridgeCapabilityMethodPattern.MatchString(scope) {
				return fmt.Errorf("browser capability %s has an invalid scope", descriptor.Method)
			}
		}
		for _, condition := range descriptor.ConditionalScopes {
			if !extensionBridgeCapabilityMethodPattern.MatchString(condition.Scope) ||
				strings.TrimSpace(condition.When) == "" || len(condition.When) > 240 {
				return fmt.Errorf("browser capability %s has an invalid conditional scope", descriptor.Method)
			}
		}
		schema, err := compileExtensionBridgeCapabilitySchema(descriptor.Method, descriptor.ParamsSchema)
		if err != nil {
			return err
		}
		compiled[descriptor.Method] = schema
	}
	if len(seen) != len(advertisedSet) {
		missing := make([]string, 0)
		for method := range advertisedSet {
			if _, ok := seen[method]; !ok {
				missing = append(missing, method)
			}
		}
		sort.Strings(missing)
		return fmt.Errorf("browser capability catalog is missing: %s", strings.Join(missing, ", "))
	}
	catalog.compiled = compiled
	return nil
}
