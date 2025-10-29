package syntaxflow

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/ssaconfig"
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
		}, true, ssaapi.WithLanguage(ssaconfig.JAVA),
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

func TestFileFilterMatchJsonByXpath(t *testing.T) {
	t.Run("test match json by xpath 1", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.json", `{
  "store": {
    "book": [
      {
        "id": 1,
        "category": "reference",
        "author": "Nigel Rees",
        "title": "Sayings of the Century",
        "price": 8.95
      },
      {
        "id": 2,
        "category": "fiction",
        "author": "Evelyn Waugh",
        "title": "Sword of Honour",
        "price": 12.99
      },
      {
        "id": 3,
        "category": "fiction",
        "author": "Herman Melville",
        "title": "Moby Dick",
        "isbn": "0-553-21311-3",
        "price": 8.99
      },
      {
        "id": 4,
        "category": "fiction",
        "author": "J. R. R. Tolkien",
        "title": "The Lord of the Rings",
        "isbn": "0-395-19395-8",
        "price": 22.99
      }
    ],
    "bicycle": {
      "color": "red",
      "price": 19.95
    }
  },
  "expensive": 10
}`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			vals, err := programs.SyntaxFlowWithError(`
${*.json}.xpath("//*/category[contains(.,'refer')]") as $result1;
${*.json}.xpath("//bicycle") as $result2;
${*.json}.xpath("//*[price<12.99]") as $result3;
${*.json}.xpath("//book/*/author") as $result4;
`)
			require.NoError(t, err)
			result1 := vals.GetValues("result1")
			result1.ShowWithSource()
			require.Contains(t, result1.StringEx(1), "6:22 - 6:31")
			require.Contains(t, result1.StringEx(1), "reference")

			// 不是精确搜索到string元素 会不准确
			//result2 := vals.GetValues("result2")
			//result2.ShowWithSource()
			//
			//result3 := vals.GetValues("result3")
			//result3.ShowWithSource()
			result4 := vals.GetValues("result4")
			result4.ShowWithSource()
			require.Contains(t, result4.String(), "Nigel Rees")
			require.Contains(t, result4.String(), "Evelyn Waugh")
			require.Contains(t, result4.String(), "Herman Melville")
			require.Contains(t, result4.String(), "J. R. R. Tolkien")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}

func TestFileFilterMatchYaml(t *testing.T) {
	t.Run("test match yaml by xpath 1", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("test.yml", `
# data.yaml
name: "Alice"
age: 30
is_active: true
hobbies:
  - reading
  - hiking
  - coding
address:
  city: "New York"
  zip: 10001
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			vals, err := programs.SyntaxFlowWithError(`${*.yml}.xpath("//address[city='New York']/zip") as $result`)
			require.NoError(t, err)
			result := vals.GetValues("result")
			result.ShowWithSource()
			require.Contains(t, result.String(), "10001")
			require.Contains(t, result.StringEx(1), "12:8 - 12:13")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})

	t.Run("test match docker file by xpath", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("docker.yml", `
version: '3.8'

services:
  mysql_db:
    image: mysql:8.0
    environment:
      MYSQL_ROOT_PASSWORD: "Root@Pass123!" 
      MYSQL_USER: "app_user"
      MYSQL_PASSWORD: "User@Secret456!"      
    ports:
      - "3306:3306"
    volumes:
      - mysql_data:/var/lib/mysql

  redis_cache:
    image: redis:6.2
    environment:
      REDIS_PASSWORD: "Redis@Auth789!"      
    ports:
      - "6379:6379"
    command: redis-server --requirepass $$REDIS_PASSWORD


  web_app:
    image: my-web-app:latest
    environment:
      DB_PASSWORD: "App@DB!Secure"          
    depends_on:
      - mysql_db
      - redis_cache

volumes:
  mysql_data:
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			vals, err := programs.SyntaxFlowWithError(`${*.yml}.xpath(<<<XPATH
//*[contains(lower-case(local-name()), 'passwd') or contains(lower-case(local-name()), 'password')
]
XPATH) as $result
`)
			require.NoError(t, err)
			result := vals.GetValues("result")
			result.ShowWithSource()
			require.Contains(t, result.String(), "Root@Pass123!")
			require.Contains(t, result.String(), "User@Secret456!")
			require.Contains(t, result.String(), "Redis@Auth789!")
			require.Contains(t, result.String(), "App@DB!Secure")
			return nil
		}, ssaapi.WithLanguage(ssaconfig.JAVA))
	})
}
