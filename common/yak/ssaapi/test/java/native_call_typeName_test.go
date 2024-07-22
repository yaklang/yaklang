package java

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestNativeCallTypeName(t *testing.T) {
	ssatest.Check(t, XXE_Code, func(prog *ssaapi.Program) error {
		typeName := prog.SyntaxFlowChain(`documentBuilder<typeName> as $id;`)[0]
		assert.Contains(t, typeName.String(), "DocumentBuilder")
		typeName = prog.SyntaxFlowChain(`documentBuilder<fullTypeName> as $id;`)[0]
		assert.Contains(t, typeName.String(), "javax.xml.parsers.DocumentBuilder")
		return nil
	}, ssaapi.WithLanguage(ssaapi.JAVA))
}

func TestNativeCallTypeNameWithSCAVersion(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("FastJSONDemoController.java",
		`package com.example.demo.controller.fastjsondemo;

import com.alibaba.fastjson.JSON;
import org.apache.ibatis.annotations.Param;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/fastjson")
public class FastJSONDemoController {
    @GetMapping("/fromId")
    public ResponseEntity<Object> loadFromParam(@RequestParam(name = "id") String id) {
        // This is a FASTJSON Vuln typically.
        Object anyJSON = JSON.parse(id);
        JSON a =(JSON) anyJSON;
        return ResponseEntity.ok(anyJSON);
    }
}
`)
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
    <licenses>
        <license/>
    </licenses>
    <developers>
        <developer/>
    </developers>
    <scm>
        <connection/>
        <developerConnection/>
        <tag/>
        <url/>
    </scm>
    <properties>
        <java.version>8</java.version>
    </properties>
    <dependencies>
        <dependency>
            <groupId>org.xerial</groupId>
            <artifactId>sqlite-jdbc</artifactId>
            <version>3.36.0.3</version>  <!-- 请检查最新版本 -->
        </dependency>

        <dependency>
            <groupId>com.alibaba</groupId>
            <artifactId>fastjson</artifactId>
            <version>1.2.24</version>
        </dependency>

        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-web</artifactId>
        </dependency>
        <dependency>
            <groupId>org.mybatis.spring.boot</groupId>
            <artifactId>mybatis-spring-boot-starter</artifactId>
            <version>3.0.3</version>
        </dependency>
        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-freemarker</artifactId>
        </dependency>

        <dependency>
            <groupId>org.springframework.boot</groupId>
            <artifactId>spring-boot-starter-test</artifactId>
            <scope>test</scope>
        </dependency>
        <dependency>
            <groupId>org.mybatis.spring.boot</groupId>
            <artifactId>mybatis-spring-boot-starter-test</artifactId>
            <version>3.0.3</version>
            <scope>test</scope>
        </dependency>
    </dependencies>

    <build>
        <plugins>
            <plugin>
                <groupId>org.springframework.boot</groupId>
                <artifactId>spring-boot-maven-plugin</artifactId>
            </plugin>
        </plugins>
    </build>

</project>
`)

	progs1, err := ssaapi.ParseProject(vf, ssaapi.WithLanguage(ssaapi.JAVA))
	if err != nil {
		t.Fatal(err)
	}
	for _, prog := range progs1 {
		typeName := prog.SyntaxFlowChain(`a<typeName> as $id;`)[0]
		assert.Contains(t, typeName.String(), "JSON")
		typeName = prog.SyntaxFlowChain(`a<fullTypeName> as $id`)[0]
		assert.Contains(t, typeName.String(), "com.alibaba.fastjson.JSON:1.2.24")
	}
	projectName := "testFastJsonFullTypeName"
	_, err = ssaapi.ParseProject(vf, ssaapi.WithLanguage(ssaapi.JAVA), ssaapi.WithDatabaseProgramName(projectName))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}
	prog, err := ssaapi.FromDatabase(projectName)
	typeName := prog.SyntaxFlowChain(`a<typeName> as $id;`)[0]
	assert.Contains(t, typeName.String(), "JSON")
	typeName = prog.SyntaxFlowChain(`a<fullTypeName> as $id`)[0]
	assert.Contains(t, typeName.String(), "com.alibaba.fastjson.JSON:1.2.24")

}
