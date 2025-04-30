package syntaxflow

import (
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
${*.properties}.regexp(/(?i)(.*access[_-]?((token)|(key)).*)\s*=\s*((?!\{\{)(?!(?i)^(true|false|on|off|yes|no|y|n|null)).+)/) as $accessKey
	`, map[string][]string{
			"accessKey": {
				`"jsc.accessKey.id=aaaaaaaaaaaa"`,
				`"jsc.accessKey.secret=bbbbbbbbbbbbbbbbb"`,
			},
		}, false, ssaapi.WithLanguage(consts.JAVA),
		)
	})

	t.Run("check normal file filter", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `
${.+\.(properties|prop|config|cfg|ini)$}.regexp()
		`, map[string][]string{
			"target": {
				`spring.datasource.password=woshishujukumima`,
			},
		}, false, ssaapi.WithLanguage(consts.JAVA))

	})

}
