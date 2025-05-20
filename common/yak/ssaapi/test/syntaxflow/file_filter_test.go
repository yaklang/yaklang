package syntaxflow

import (
	"github.com/stretchr/testify/require"
	"testing"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestFileFilter(t *testing.T) {

	vf := filesys.NewVirtualFs()
	vf.AddFile("application.properties", `
spring.datasource.url=jdbc:mysql://localhost:3306/java_sec_code?allowPublicKeyRetrieval=true&useSSL=false&serverTimezone=UTC
spring.datasource.username=root
spring.datasource.password=woshishujukumima
spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
mybatis.mapper-locations=classpath:mapper/*.xml
# mybatis SQL log
logging.level.org.joychou.mapper=debug

# Spring Boot Actuator Config
management.security.enabled=false

# logging.config=classpath:logback-online.xml

# jsonp callback parameter
joychou.business.callback = callback_


### check referer configuration begins ###
joychou.security.referer.enabled = false
joychou.security.referer.host = joychou.org, joychou.com
# Only support ant url style.
joychou.security.referer.uri = /jsonp/**
### check referer configuration ends ###
# Fake aksk. Simulate actuator info leak.
jsc.accessKey.id=aaaaaaaaaaaa
jsc.accessKey.secret=bbbbbbbbbbbbbbbbb
		`)

	t.Run("check normal file filter ", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
${*.properties}.regexp(/(?i).*access[_-]?[token|key].*\s*=\s*((?!\{\{)(?!(?i)^(true|false|on|off|yes|no|y|n|null)).+)/) as $accessKey
	`, map[string][]string{
			"accessKey": {
				`"jsc.accessKey.id=aaaaaaaaaaaa"`,
				`"jsc.accessKey.secret=bbbbbbbbbbbbbbbbb"`,
			},
		}, true, ssaapi.WithLanguage(consts.JAVA),
		)
	})

}

func TestFileFilterJson(t *testing.T) {
	t.Run("test simple json path", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.json", `{
  "auths": {
    "https://index.docker.io/v1/": {
      "auth": "dXNlcm5hbWU6cGFzc3dvcmQxMjM=",
      "email": "user@example.com"
    },
    "https://private-registry.example.com": {
      "auth": "YWRtaW46c2VjcmV0cGFzc3dvcmQ=",
      "email": "admin@example.com"
    }
  }
}`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			vals, err := programs.SyntaxFlowWithError(`${*.json}.json("$.auths.*.auth") as $result`)
			require.NoError(t, err)
			result := vals.GetValues("result")
			require.Contains(t, result.String(), "dXNlcm5hbWU6cGFzc3dvcmQxMjM=")
			require.Contains(t, result.String(), "YWRtaW46c2VjcmV0cGFzc3dvcmQ=")

			require.Contains(t, result.StringEx(1), "4:16 - 4:44")
			require.Contains(t, result.StringEx(1), "8:16 - 8:44")
			return nil
		})
	})

	t.Run("test match mutli pos", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.json", `{
  "auths": {
    "https://index.docker.io/v1/": {
      "auth": "dXNlcm5hbWU6cGFzc3dvcmQxMjM=",
      "email": "admin@example.com"
    },
    "https://private-registry.example.com": {
      "auth": "YWRtaW46c2VjcmV0cGFzc3dvcmQ=",
      "email": "admin@example.com"
    },
	"other":"admin@example.com"
  }
}`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			vals, err := programs.SyntaxFlowWithError(`${*.json}.json("$.auths.*.email") as $result`)
			require.NoError(t, err)
			result := vals.GetValues("result")
			result.ShowWithSource()

			require.Contains(t, result.String(), "admin@example.com")
			require.Contains(t, result.StringEx(1), "5:17 - 5:34")
			require.Contains(t, result.StringEx(1), "9:17 - 9:34")
			// TODO:现在jsonPath的位置使用strings.Index，会导致也匹配到"other":"admin@example.com"也会分析到
			// 如果需要更准确的位置，可能需要在jsonpath的Parser加入位置信息
			//require.NotContains(t, result.StringEx(1), "11:11 - 11:28")
			return nil
		})
	})
}
