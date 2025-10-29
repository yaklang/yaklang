package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
)

func TestParsePropertiesFile(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("application.properties", `
		# 应用程序配置
		spring.application.name=myApplication
		server.port=8080
		server.servlet.context-path=/api
		
		# 数据源配置
		spring.datasource.url=jdbc:mysql://localhost:3306/mydb?useSSL=false&serverTimezone=UTC
		spring.datasource.username=root
		spring.datasource.password=secret
		spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
		spring.datasource.hikari.connection-timeout=60000
		spring.datasource.hikari.maximum-pool-size=10
		spring.datasource.hikari.idle-timeout=300000
		spring.datasource.hikari.max-lifetime=2000000
		
		# JPA 配置
		spring.jpa.hibernate.ddl-auto=update
		spring.jpa.show-sql=true
		spring.jpa.properties.hibernate.dialect=org.hibernate.dialect.MySQL5InnoDBDialect
		
		# 日志配置
		logging.level.root=INFO
		logging.level.org.springframework.web=DEBUG
		logging.level.com.example=DEBUG
		logging.file.name=application.log
		logging.pattern.console=%d{yyyy-MM-dd HH:mm:ss} - %msg%n
		
		# 邮件发送服务配置
		spring.mail.host=smtp.example.com
		spring.mail.port=587
		spring.mail.username=email@example.com
		spring.mail.password=secret
		spring.mail.properties.mail.smtp.auth=true
		spring.mail.properties.mail.smtp.starttls.enable=true
		
		# Thymeleaf 模板引擎配置
		spring.thymeleaf.prefix=classpath:/templates/
		spring.thymeleaf.suffix=.html
		spring.thymeleaf.mode=HTML
		spring.thymeleaf.encoding=UTF-8
		spring.thymeleaf.cache=false
		
		# 国际化配置
		spring.messages.basename=messages
		spring.mvc.locale=zh_CN
		
		# 安全配置
		spring.security.user.name=admin
		spring.security.user.password=secret
		spring.security.user.roles=ADMIN
		
		# Actuator 配置
		management.endpoints.web.exposure.include=health,info,metrics
		management.endpoint.health.show-details=always
		management.server.port=8081
	`)

	t.Run("test parse config", func(t *testing.T) {
		programs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.JAVA))
		require.NoError(t, err)
		app := programs[0].Program.GetApplication()
		require.Equal(t, "myApplication", app.GetProjectConfigValue("spring.application.name"))
		require.Equal(t, "8080", app.GetProjectConfigValue("server.port"))
		require.Equal(t, ".html", app.GetProjectConfigValue("spring.thymeleaf.suffix"))
		require.Equal(t, "classpath:/templates/", app.GetProjectConfigValue("spring.thymeleaf.prefix"))
	})

	//	t.Run("test config file position", func(t *testing.T) {
	//		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
	//			prog := programs[0]
	//			rule := `
	//
	// __projectConfig__."management.endpoints.web.exposure.include" as $include;
	// `
	//
	//			vals, err := prog.SyntaxFlowWithError(rule)
	//			require.NoError(t, err)
	//			appName := vals.GetValues("include")
	//			appName.ShowWithSource()
	//			require.Contains(t, appName.StringEx(1), "health,info,metrics")
	//			return nil
	//		})
	//	})
}

func TestParseYamlFile(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("application.yaml", `
server:
  port: 8080
  servlet:
    context-path: /myapp

spring:
  datasource:
    url: jdbc:mysql://localhost:3306/mydb
    username: user
    password: pass
    driver-class-name: com.mysql.cj.jdbc.Driver
  jpa:
    hibernate:
      ddl-auto: update
    show-sql: true
    properties:
      hibernate:
        dialect: org.hibernate.dialect.MySQL5InnoDBDialect

logging:
  level:
    root: INFO
    org.springframework.web: DEBUG
    org.hibernate: ERROR

# 自定义属性
myapp:
  welcome-message: Welcome to My Application
  feature-enabled: true
`)

	t.Run("test parse config", func(t *testing.T) {
		programs, err := ssaapi.ParseProjectWithFS(vf, ssaapi.WithLanguage(ssaconfig.JAVA))
		require.NoError(t, err)
		app := programs[0].Program.GetApplication()
		for k, v := range app.ProjectConfig {
			t.Log(k + ":" + v.ConfigValue)
		}
		require.Equal(t, "user", app.GetProjectConfigValue("spring.datasource.username"))
		require.Equal(t, "Welcome to My Application", app.GetProjectConfigValue("myapp.welcome-message"))
		require.Equal(t, "true", app.GetProjectConfigValue("myapp.feature-enabled"))
	})

	// 	t.Run("test syntaxflow search config", func(t *testing.T) {
	// 		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
	// 			prog := programs[0]
	// 			vals, err := prog.SyntaxFlowWithError(`
	// __projectConfig__."spring.datasource.username" as $username;
	// __projectConfig__."myapp.welcome-message" as $welcome
	// `)
	// 			require.NoError(t, err)
	// 			userName := vals.GetValues("username")
	// 			userName.Show()
	// 			require.Contains(t, userName.String(), "user")

	//			welcome := vals.GetValues("welcome")
	//			welcome.Show()
	//			require.Contains(t, welcome.String(), "Welcome to My Application")
	//			return nil
	//		})
	//	})
}
