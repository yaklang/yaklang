package openapi3

import (
	"github.com/yaklang/yaklang/common/utils"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIssue495(t *testing.T) {
	{
		spec := []byte(`
openapi: 3.0.1
info:
  version: v1
  title: Products api
components:
  schemas:
    someSchema:
      type: object
    schemaArray:
      type: array
      minItems: 1
      items:
        $ref: '#'
paths:
  /categories:
    get:
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                properties:
                  allOf:
                    $ref: '#/components/schemas/schemaArray'
`[1:])

		sl := NewLoader()

		doc, err := sl.LoadFromData(spec)
		require.NoError(t, err)

		err = doc.Validate(sl.Context)
		require.EqualError(t, err, `invalid components: schema "schemaArray": found unresolved ref: "#"`)
	}

	spec := []byte(`
openapi: 3.0.1
info:
  version: v1
  title: Products api
components:
  schemas:
    someSchema:
      type: object
    schemaArray:
      type: array
      minItems: 1
      items:
        $ref: '#/components/schemas/someSchema'
paths:
  /categories:
    get:
      responses:
        '200':
          description: ''
          content:
            application/json:
              schema:
                properties:
                  allOf:
                    $ref: '#/components/schemas/schemaArray'
`[1:])

	sl := NewLoader()

	doc, err := sl.LoadFromData(spec)
	require.NoError(t, err)

	err = doc.Validate(sl.Context)
	require.NoError(t, err)

	require.Equal(t, &Schema{Type: "object"}, doc.Components.Schemas["schemaArray"].Value.Items.Value)
}

func TestIssue495WithDraft04(t *testing.T) {
	host, port := utils.DebugMockHTTPHandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Write([]byte(`{
    "id": "http://json-schema.org/draft-04/schema#",
    "$schema": "http://json-schema.org/draft-04/schema#",
    "description": "Core schema meta-schema",
    "definitions": {
        "schemaArray": {
            "type": "array",
            "minItems": 1,
            "items": { "$ref": "#" }
        },
        "positiveInteger": {
            "type": "integer",
            "minimum": 0
        },
        "positiveIntegerDefault0": {
            "allOf": [ { "$ref": "#/definitions/positiveInteger" }, { "default": 0 } ]
        },
        "simpleTypes": {
            "enum": [ "array", "boolean", "integer", "null", "number", "object", "string" ]
        },
        "stringArray": {
            "type": "array",
            "items": { "type": "string" },
            "minItems": 1,
            "uniqueItems": true
        }
    },
    "type": "object",
    "properties": {
        "id": {
            "type": "string"
        },
        "$schema": {
            "type": "string"
        },
        "title": {
            "type": "string"
        },
        "description": {
            "type": "string"
        },
        "default": {},
        "multipleOf": {
            "type": "number",
            "minimum": 0,
            "exclusiveMinimum": true
        },
        "maximum": {
            "type": "number"
        },
        "exclusiveMaximum": {
            "type": "boolean",
            "default": false
        },
        "minimum": {
            "type": "number"
        },
        "exclusiveMinimum": {
            "type": "boolean",
            "default": false
        },
        "maxLength": { "$ref": "#/definitions/positiveInteger" },
        "minLength": { "$ref": "#/definitions/positiveIntegerDefault0" },
        "pattern": {
            "type": "string",
            "format": "regex"
        },
        "additionalItems": {
            "anyOf": [
                { "type": "boolean" },
                { "$ref": "#" }
            ],
            "default": {}
        },
        "items": {
            "anyOf": [
                { "$ref": "#" },
                { "$ref": "#/definitions/schemaArray" }
            ],
            "default": {}
        },
        "maxItems": { "$ref": "#/definitions/positiveInteger" },
        "minItems": { "$ref": "#/definitions/positiveIntegerDefault0" },
        "uniqueItems": {
            "type": "boolean",
            "default": false
        },
        "maxProperties": { "$ref": "#/definitions/positiveInteger" },
        "minProperties": { "$ref": "#/definitions/positiveIntegerDefault0" },
        "required": { "$ref": "#/definitions/stringArray" },
        "additionalProperties": {
            "anyOf": [
                { "type": "boolean" },
                { "$ref": "#" }
            ],
            "default": {}
        },
        "definitions": {
            "type": "object",
            "additionalProperties": { "$ref": "#" },
            "default": {}
        },
        "properties": {
            "type": "object",
            "additionalProperties": { "$ref": "#" },
            "default": {}
        },
        "patternProperties": {
            "type": "object",
            "additionalProperties": { "$ref": "#" },
            "default": {}
        },
        "dependencies": {
            "type": "object",
            "additionalProperties": {
                "anyOf": [
                    { "$ref": "#" },
                    { "$ref": "#/definitions/stringArray" }
                ]
            }
        },
        "enum": {
            "type": "array",
            "minItems": 1,
            "uniqueItems": true
        },
        "type": {
            "anyOf": [
                { "$ref": "#/definitions/simpleTypes" },
                {
                    "type": "array",
                    "items": { "$ref": "#/definitions/simpleTypes" },
                    "minItems": 1,
                    "uniqueItems": true
                }
            ]
        },
        "format": { "type": "string" },
        "allOf": { "$ref": "#/definitions/schemaArray" },
        "anyOf": { "$ref": "#/definitions/schemaArray" },
        "oneOf": { "$ref": "#/definitions/schemaArray" },
        "not": { "$ref": "#" }
    },
    "dependencies": {
        "exclusiveMaximum": [ "maximum" ],
        "exclusiveMinimum": [ "minimum" ]
    },
    "default": {}
}`))
	})
	addr := utils.HostPort(host, port)

	spec := []byte(`openapi: 3.0.1
servers:
- url: http://localhost:5000
info:
  version: v1
  title: Products api
  contact:
    name: me
    email: me@github.com
  description: This is a sample
paths:
  /categories:
    get:
      summary: Provides the available categories for the store
      operationId: list-categories
      responses:
        '200':
          description: this is a desc
          content:
            application/json:
              schema:
                $ref: http://` + addr + `/draft-04/schema
`)

	sl := NewLoader()
	sl.IsExternalRefsAllowed = true

	doc, err := sl.LoadFromData(spec)
	require.NoError(t, err)

	err = doc.Validate(sl.Context)
	require.ErrorContains(t, err, `found unresolved ref: "#"`)
}

func TestIssue495WithDraft04Bis(t *testing.T) {
	spec := []byte(`
openapi: 3.0.1
servers:
- url: http://localhost:5000
info:
  version: v1
  title: Products api
  contact:
    name: me
    email: me@github.com
  description: This is a sample
paths:
  /categories:
    get:
      summary: Provides the available categories for the store
      operationId: list-categories
      responses:
        '200':
          description: this is a desc
          content:
            application/json:
              schema:
                $ref: testdata/draft04.yml
`[1:])

	sl := NewLoader()
	sl.IsExternalRefsAllowed = true

	doc, err := sl.LoadFromData(spec)
	require.NoError(t, err)

	err = doc.Validate(sl.Context)
	require.ErrorContains(t, err, `found unresolved ref: "#"`)
}
