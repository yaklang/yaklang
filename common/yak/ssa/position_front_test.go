package ssa

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"testing"
)

func Test_Get_RangeByText(t *testing.T) {
	t.Run("test get first range by text", func(t *testing.T) {
		content := `
		server.port=8080
		server.servlet.context-path=/api
		
		spring.datasource.url=jdbc:mysql://localhost:3306/mydb?useSSL=false&serverTimezone=UTC
		spring.datasource.username=root
		spring.datasource.password=secret
		spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
		spring.datasource.hikari.connection-timeout=60000
		spring.datasource.hikari.maximum-pool-size=10
		spring.datasource.hikari.idle-timeout=300000
		spring.datasource.hikari.max-lifetime=2000000
		management.endpoints.web.exposure.include=health,info,metrics
		management.endpoint.health.show-details=always
		management.server.port=8081
`
		editor := memedit.NewMemEditor(content)
		rng := GetFirstRangeByText(editor, "spring.datasource.url")
		require.Equal(t, `spring.datasource.url`, rng.GetText())

		rng2 := GetFirstRangeByText(editor, "management.endpoint.health.show-details")
		require.Equal(t, `management.endpoint.health.show-details`, rng2.GetText())
	})

	t.Run("test get ranges by text", func(t *testing.T) {
		content := `
		server.port=8080
		server.servlet.context-path=/api
		
		spring.datasource.url=jdbc:mysql://localhost:3306/mydb?useSSL=false&serverTimezone=UTC
		spring.datasource.username=root
				server.port=8080

		spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
		spring.datasource.hikari.connection-timeout=60000
		spring.datasource.hikari.maximum-pool-size=10
		spring.datasource.hikari.idle-timeout=300000
				server.port=8080

`
		editor := memedit.NewMemEditor(content)
		rngs := GetRangesByText(editor, "server.port")
		require.Equal(t, 3, len(rngs))
		for _, rng := range rngs {
			require.Equal(t, `server.port`, rng.GetText())
		}
	})
}
