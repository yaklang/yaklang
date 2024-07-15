package java

import (
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

//go:embed sample/springboot
var springbootLoader embed.FS

func TestExtraFileAnalyzer(t *testing.T) {
	prog, err := ssaapi.ParseProject(
		filesys.NewEmbedFS(springbootLoader),
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
	if err != nil {
		t.Fatal(err)
	}
	res, err := prog.SyntaxFlowWithError(
		`${application.properties}.re(/url=(.*)/) as $url`,
	)
	assert.NoErrorf(t, err, "SyntaxFlowWithError error: %v", err)
	res.Show()
	assert.Greater(t, res.GetValues("url").Len(), 0)
}

func TestSimpleExtraFile(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/resources/application.properties", `
	spring.datasource.url=jdbc:mysql://localhost:3306/your_database
	spring.datasource.username=your_username
	spring.datasource.password=your_password
	spring.datasource.driver-class-name=com.mysql.cj.jdbc.Driver
	spring.jpa.hibernate.ddl-auto=update
	spring.jpa.properties.hibernate.dialect=org.hibernate.dialect.MySQL5InnoDBDialect
	`)
	vf.AddFile("src/resources/mapper/AMapper.xml", `
	<?xml version="1.0" encoding="UTF-8" ?>
	<!DOCTYPE mapper
	PUBLIC "-//mybatis.org//DTD Mapper 3.0//EN"
	"http://mybatis.org/dtd/mybatis-3-mapper.dtd">
	<mapper namespace="com.mapper.UserMapper">

	<select id="selectUserByUsername" parameterType="String" resultType="com.po.User">
		select * from user where username=${username}
	</select>

	<insert id="insertUsetByUsername" parameterType="com.po.User">
		insert into user(username,userpasswd)values(#{username},#{userpasswd});
	</insert>

	</mapper>
	`)

	t.Run("test simple config file", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf,
			`${*.properties}.re(/spring.datasource.url=(.*)/) as $url`,
			map[string][]string{
				"url": {`"jdbc:mysql://localhost:3306/your_database"`},
			}, false, ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})

	t.Run("test simple mybatis mapper file", func(t *testing.T) {
		ssatest.CheckSyntaxFlowWithFS(t, vf,
			`
		${*Mapper.xml}.xpath("//mapper/*[contains(., '${') and @id]/@id") as $url
		${*Mapper.xml}.xpath("string(//mapper/*[contains(., '${') and @id]/@id)") as $url2
		`,
			map[string][]string{
				"url":  {`"selectUserByUsername"`},
				"url2": {`"selectUserByUsername"`},
			}, false,
			ssaapi.WithLanguage(ssaapi.JAVA),
		)
	})
}

func TestMultipleResultExtraFile(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("src/resources/a.properties", `
	a.url=http://a.com
	`)
	vf.AddFile("src/resources/b.properties", `
	b.url=http://b.com
	`)
	ssatest.CheckSyntaxFlowWithFS(t, vf,
		`${*.properties}.re(/url=(.*)/) as $url`,
		map[string][]string{
			"url": {`"http://a.com"`, `"http://b.com"`},
		}, false,
		ssaapi.WithLanguage(ssaapi.JAVA),
	)
}
