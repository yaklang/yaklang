package java

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestDepencyVersionIn(t *testing.T) {
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
	t.Run("test simple versionInFilter 1", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver in(1.2.3,2.3.4] as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test simple versionInFilter 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver in(,1.2.24] as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test simple versionInFilter 3", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*fastjson.version as $ver;
$ver in["1.2.4",) as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.2.24\""},
			"vulnVersion": {"\"1.2.24\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test the same artifactId 1 ", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__./com.org/.version as $ver;
$ver in[1.1.0,3.0.0) as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.11.1\""},
			"vulnVersion": {"\"1.11.1\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test the same artifactId 2", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__.*test1.version as $ver;
$ver in[3.0.0,) as $vulnVersion`, map[string][]string{
			"ver":         {"\"1.11.1\"", "\"3.22.2\""},
			"vulnVersion": {"\"3.22.2\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
	t.Run("test abnormal version ", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf, `__dependency__./jackson-databind/.version as $ver;
$ver in["2.12.1-release","3.12.3-release"] as $vulnVersion`, map[string][]string{
			"ver":         {"\"2.12.3-release\""},
			"vulnVersion": {"\"2.12.3-release\""},
		}, false, ssaapi.WithLanguage(consts.JAVA))
	})
}
