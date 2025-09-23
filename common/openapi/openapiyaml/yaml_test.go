package openapiyaml

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestYaml_FlattenJson(t *testing.T) {
	data := []byte(`
{
  "id": 123,
  "name": "Alice",
  "email": "alice@example.com",
  "isActive": true,
  "orders": [
    {
      "orderId": 456,
      "product": "Laptop",
      "price": 999.99
    },
    {
      "orderId": 789,
      "product": "Mouse",
      "price": 25.00
    }
  ]
}
`)

	t.Run("test FlattenJSON", func(t *testing.T) {
		var jsonMap map[string]interface{}
		err := json.Unmarshal(data, &jsonMap)
		require.NoError(t, err)
		result := make(map[string]string)
		FlattenJSON(jsonMap, "", result)
		haveResult1 := false
		haveResult2 := false
		for k, v := range result {
			t.Log(k + ":" + v)
			if k == "orders[0].product" && v == "Laptop" {
				haveResult1 = true
			}
			if k == "email" && v == "alice@example.com" {
				haveResult2 = true
			}
		}
		require.True(t, haveResult1)
		require.True(t, haveResult2)
	})

}

func TestYaml_Yaml_To_KVParis(t *testing.T) {
	data := []byte(`
# 应用配置
app:
  name: MyApp
  version: 1.0.0
  environment: production
  port: 8080
  enableFeatureX: true

# 数据库配置
database:
  host: localhost
  port: 3306
  username: dbuser
  password: dbpassword
  name: mydatabase
  pool:
    maxConnections: 20
    idleTimeout: 300s

# 外部服务配置
externalServices:
  - name: paymentGateway
    url: https://api.paymentgateway.com/v1
    timeout: 5000ms
    retries: 3
  - name: emailService
    url: https://api.emailservice.com/v1
    timeout: 3000ms
    retries: 2

`)
	t.Run("test yamlToKVParis", func(t *testing.T) {
		result, err := YamlToKVParis(data)
		require.NoError(t, err)
		haveResult1 := false
		haveResult2 := false
		for k, v := range result {
			t.Log(k + ":" + v)
			if k == "externalServices[0].retries" && v == "3" {
				haveResult1 = true
			}
			if k == "database.pool.idleTimeout" && v == "300s" {
				haveResult2 = true
			}
		}
		require.True(t, haveResult1)
		require.True(t, haveResult2)
	})
}
