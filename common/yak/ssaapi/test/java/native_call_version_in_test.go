package java

import (
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestDependencyVersionInCondition(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("pom.xml",
		`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-parent</artifactId>
        <version>3.2.7</version>
        <relativePath/> <!-- lookup parent from repository -->
    </parent>
    <groupId>com.example</groupId>
    <artifactId>demo</artifactId>
    <version>0.0.1-SNAPSHOT</version>
    <name>demo</name>
    <description>Demo project for Spring Boot</description>
    <url/>
    <properties>
        <java.version>8</java.version>
    </properties>
    <dependencies>
        <dependency>
            <groupId>com.alibaba</groupId>
            <artifactId>fastjson</artifactId>
            <version>1.2.24</version>
        </dependency>
		<dependency>
            <groupId>com.org</groupId>
            <artifactId>test1</artifactId>
            <version>1.11.1</version>
        </dependency>
		<dependency>
            <groupId>com.example</groupId>
            <artifactId>test1</artifactId>
            <version>3.22.2</version>
        </dependency>
		 <dependency>
            <groupId>com.fasterxml.jackson.core</groupId>
            <artifactId>jackson-databind</artifactId>
            <version>2.12.3-release</version>
        </dependency>
    </dependencies>
</project>
`)
	t.Run("test simple versionIn condition filter 1", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver?{version_in:(0.1.0,1.3.0]}  as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test simple versionIn condition filter 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver ?{version_in:(,1.2.24]} as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test simple versionIn condition filter 3", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver ?{version_in:["1.2.24",]}as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test the same artifactId 1 ", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__./com.org/.version as $ver;
$ver ?{version_in:[1.1.0,3.0.0)} as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.11.1\""},
			"vulnVersion": {"\"1.11.1\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test the same artifactId 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*test1.version as $ver;
$ver ?{version_in:[3.0.0,)}as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.11.1\"", "\"3.22.2\""},
			"vulnVersion": {"\"3.22.2\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test abnormal version ", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__./jackson-databind/.version as $ver;
$ver?{version_in:["2.12.1-release","3.12.3-release"]}as $vulnVersion `, map[string][]string{
			"ver":         {"\"2.12.3-release\""},
			"vulnVersion": {"\"2.12.3-release\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test complex versionIn condition filter 1", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver?{version_in:(0.1.0,1.3.0]||(1.1.0,2.3.0] }  as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test complex versionIn condition filter 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver?{version_in:(0.1.0,1.0.0] || (1.5.0,2.3.0] || [0.2.4,5.2.2)  }  as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
}

func TestDependencyVersionInFilter(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("pom.xml",
		`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-parent</artifactId>
        <version>3.2.7</version>
        <relativePath/> <!-- lookup parent from repository -->
    </parent>
    <groupId>com.example</groupId>
    <artifactId>demo</artifactId>
    <version>0.0.1-SNAPSHOT</version>
    <name>demo</name>
    <description>Demo project for Spring Boot</description>
    <url/>
    <properties>
        <java.version>8</java.version>
    </properties>
    <dependencies>
        <dependency>
            <groupId>com.alibaba</groupId>
            <artifactId>fastjson</artifactId>
            <version>1.2.24</version>
        </dependency>
		<dependency>
            <groupId>com.org</groupId>
            <artifactId>test1</artifactId>
            <version>1.11.1</version>
        </dependency>
		<dependency>
            <groupId>com.example</groupId>
            <artifactId>test1</artifactId>
            <version>3.22.2</version>
        </dependency>
		 <dependency>
            <groupId>com.fasterxml.jackson.core</groupId>
            <artifactId>jackson-databind</artifactId>
            <version>2.12.3-release</version>
        </dependency>
    </dependencies>
</project>
`)
	t.Run("test simple versionIn condition filter 1", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver in (0.1.0,1.3.0]  as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test simple versionIn condition filter 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver in (,1.2.24] as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test simple versionIn condition filter 3", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver in ["1.2.24",] as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test the same artifactId 1 ", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__./com.org/.version as $ver;
$ver in [1.1.0,3.0.0) as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.11.1\""},
			"vulnVersion": {"\"1.11.1\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test the same artifactId 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*test1.version as $ver;
$ver in [3.0.0,) as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.11.1\"", "\"3.22.2\""},
			"vulnVersion": {"\"3.22.2\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test abnormal version ", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__./jackson-databind/.version as $ver;
$ver in ["2.12.1-release","3.12.3-release"]  as $vulnVersion`, map[string][]string{
			"ver":         {"\"2.12.3-release\""},
			"vulnVersion": {"\"2.12.3-release\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test complex versionIn condition filter 1", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver in (0.1.0,1.3.0]||(1.1.0,2.3.0]   as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test complex versionIn condition filter 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver in (1.0,1.0.0] || (1.5.0,2.3.0] || [0.2.4,5.2.2)    as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
}

func TestDependencyRange(t *testing.T) {
	t.Run("test range with Chinese", func(t *testing.T) {
		vf := filesys.NewVirtualFs()
		vf.AddFile("pom.xml", `<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>
    <parent>
        <groupId>org.springframework.boot</groupId>
        <artifactId>spring-boot-starter-parent</artifactId>
        <version>2.4.1</version>
        <relativePath/>
    </parent>
    <properties>
        <java.version>1.8</java.version>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>
    <dependencies>
        <dependency>
        	<!-- Fastjson 1.2.24存在rce漏洞 -->
            <groupId>com.alibaba</groupId>
            <artifactId>fastjson</artifactId>
            <version>1.2.41</version>
        </dependency>
    </dependencies>
</project>
`)
		ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
			res, err := programs.SyntaxFlowWithError(`
__dependency__.*alibaba*fastjson.version as $ver;
$ver in (,1.2.47] as $vuln_1_2_47;
alert $vuln_1_2_47 for {
    message: 'SCA: com.alibaba.fastjson <= 1.2.47 RCE Easy to exploit',
    severity: critical,
    cvss: "9.8"
}
`)
			require.NoError(t, err)
			vals := res.GetValues("vuln_1_2_47")
			vals.ShowWithSource()
			require.Contains(t, vals.StringEx(1), "fastjson")
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}
