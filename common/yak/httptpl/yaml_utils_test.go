package httptpl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestYamlNodeUtils(t *testing.T) {
	testYaml := `id: CVE-2023-24278

info:
  testInt: 123
  testFloat: 123.456
  name: Squidex <7.4.0 - Cross-Site Scripting
  author: r3Y3r53
  severity: medium
  description: |
    Squidex before 7.4.0 contains a cross-site scripting vulnerability via the squid.svg endpoint. An attacker can possibly obtain sensitive information, modify data, and/or execute unauthorized administrative operations in the context of the affected site.
  reference:
    - https://census-labs.com/news/2023/03/16/reflected-xss-vulnerabilities-in-squidex-squidsvg-endpoint/
    - https://www.openwall.com/lists/oss-security/2023/03/16/1
    - https://nvd.nist.gov/vuln/detail/CVE-2023-24278
  classification:
    cvss-metrics: CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N
    cvss-score: 6.1
    cve-id: CVE-2023-24278
    cwe-id: CWE-79
  metadata:
    shodan-query: http.favicon.hash:1099097618
    verified: "true"
  tags: cve,cve2023,xss,squidex,cms,unauth

variables:
  a1: "{{rand_int(1000,9000)}}"
  a2: "{{rand_int(1000,9000)}}"
  a3: "{{rand_int(1000,9000)}}{{a1}}"
  a4: "{{rand_int(1000,9000)}}{{a2}}------{{a1+a2}}=={{a1}}+{{a2}}  {{to_number(a1)*to_number(a2)}}=={{a1}}*{{a2}}"
  a5: "{{randstr}}"

requests:
  - method: GET
    path:
      - "{{BaseURL}}/squid.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
      - "{{BaseURL}}/squi{{a4}}d.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
      - "{{BaseURL}}/squi{{md5(a4)}}d.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
      - "{{BaseURL}}/squi{{md5(a4)}}{{a1}}d.svg?title=Not%20Found&text=This%20is%20not%20the%20page%20you%20are%20looking%20for!&background=%22%3E%3Cscript%3Ealert(document.domain)%3C/script%3E%3Cimg%20src=%22&small"
    headers:
      Authorization: "{{a1+a3}} {{a2}} {{BaseURL}}"
      Test-Payload: "{{name}} {{a6}}"

    payloads:
      name:
        - "admin123"
        - "aaa123"
      a6:
        - "321nimda"
        - 321aaa

    matchers-condition: and
    matchers:
      - type: word
        part: body
        words:
          - "<script>alert(document.domain)</script>"
          - "looking for!"
          - "{{md5(a4)}}"
        condition: or

      - type: word
        part: header
        words:
          - "image/svg+xml"

      - type: status
        status:
          - 200

# Enhanced by md on 2023/04/14`
	root := &yaml.Node{}
	err := yaml.Unmarshal([]byte(testYaml), root)
	require.NoError(t, err)
	t.Run("nodeGetRaw", func(t *testing.T) {
		node := nodeGetRaw(root, "id")
		require.NotNil(t, node)
		require.Equal(t, "CVE-2023-24278", node.Value)
	})
	t.Run("nodeGetFirstRaw", func(t *testing.T) {
		infoNode := nodeGetRaw(root, "info")
		require.NotNil(t, infoNode)
		testNode := nodeGetFirstRaw(infoNode, "name", "author")
		require.NotNil(t, testNode)
		require.Equal(t, "Squidex <7.4.0 - Cross-Site Scripting", testNode.Value)
	})
	t.Run("nodeGetString", func(t *testing.T) {
		str := nodeGetString(root, "id")
		require.Equal(t, "CVE-2023-24278", str)
	})
	t.Run("nodeGetBool", func(t *testing.T) {
		infoNode := nodeGetRaw(root, "info")
		metaNode := nodeGetRaw(infoNode, "metadata")
		verifiedNode := nodeGetBool(metaNode, "verified")
		require.True(t, verifiedNode)
	})
	t.Run("nodeGetInt64", func(t *testing.T) {
		infoNode := nodeGetRaw(root, "info")
		testNode := nodeGetRaw(infoNode, "testInt")
		require.NotNil(t, testNode)
		require.Equal(t, int64(123), nodeGetInt64(infoNode, "testInt"))
	})
	t.Run("nodeGetFloat64", func(t *testing.T) {
		infoNode := nodeGetRaw(root, "info")
		testNode := nodeGetRaw(infoNode, "testFloat")
		require.NotNil(t, testNode)
		require.Equal(t, 123.456, nodeGetFloat64(infoNode, "testFloat"))
	})
	wantReferences := []string{
		"https://census-labs.com/news/2023/03/16/reflected-xss-vulnerabilities-in-squidex-squidsvg-endpoint/",
		"https://www.openwall.com/lists/oss-security/2023/03/16/1",
		"https://nvd.nist.gov/vuln/detail/CVE-2023-24278",
	}
	t.Run("nodeGetStringSlice", func(t *testing.T) {
		infoNode := nodeGetRaw(root, "info")
		references := nodeGetStringSlice(infoNode, "reference")
		require.NotNil(t, references)
		require.Equal(t, wantReferences, references)
	})
	t.Run("sequenceNodeForEach", func(t *testing.T) {
		infoNode := nodeGetRaw(root, "info")
		i := 0
		sequenceNodeForEach(nodeGetRaw(infoNode, "reference"), func(value *yaml.Node) error {
			require.Equal(t, wantReferences[i], value.Value)
			i++
			return nil
		})
	})
	t.Run("mappingNodeForEach", func(t *testing.T) {
		wantVariables := []struct {
			Key, Value string
		}{
			{
				Key:   "a1",
				Value: "{{rand_int(1000,9000)}}",
			},
			{
				Key:   "a2",
				Value: "{{rand_int(1000,9000)}}",
			},
			{
				Key:   "a3",
				Value: "{{rand_int(1000,9000)}}{{a1}}",
			},
			{
				Key:   "a4",
				Value: "{{rand_int(1000,9000)}}{{a2}}------{{a1+a2}}=={{a1}}+{{a2}}  {{to_number(a1)*to_number(a2)}}=={{a1}}*{{a2}}",
			},
			{
				Key:   "a5",
				Value: "{{randstr}}",
			},
		}
		i := 0
		mappingNodeForEach(nodeGetRaw(root, "variables"), func(key string, node *yaml.Node) error {
			require.Equal(t, wantVariables[i].Key, key)
			require.Equal(t, wantVariables[i].Value, node.Value)

			i++
			return nil
		})
	})
}
