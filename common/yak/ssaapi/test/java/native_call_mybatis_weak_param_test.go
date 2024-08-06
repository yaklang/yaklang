package java

import (
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestNativeCall_MybatisSupport(t *testing.T) {
	f := filesys.NewVirtualFs()
	f.AddFile(`sqlmap.xml`, `<?xml version="1.0" encoding="UTF-8" ?>
<!DOCTYPE mapper
        PUBLIC "-//mybatis.org//DTD Mapper 3.0//EN"
        "http://mybatis.org/dtd/mybatis-3-mapper.dtd">

<mapper namespace="com.mycompany.myapp.UserMapper">
    <resultMap id="UserResult" type="com.mycompany.myapp.User">
        <id property="id" column="id" />
        <result property="name" column="name" />
        <result property="email" column="email" />
    </resultMap>

    <select id="getUser" resultMap="UserResult">
        SELECT * FROM User WHERE id = #{id}
    </select>

    <insert id="insertUser" useGeneratedKeys="true" keyProperty="id">
        INSERT INTO User (name, email) VALUES (#{name}, #{email})
    </insert>

    <update id="updateUser">
        UPDATE User SET name=#{name}, email=#{email} WHERE id=${id}
    </update>

    <delete id="deleteUser">
        DELETE FROM User WHERE id=#{id}
    </delete>
</mapper>
`)
	f.AddFile("UserMapper.java", `package com.mycompany.myapp;

public interface UserMapper {
    User getUser(int id);
    int insertUser(User user);
    void updateUser(User user);
    void deleteUser(int id);
}
`)
	ssatest.CheckWithFS(f, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		vars := prog.SyntaxFlowChain(`<weakMybatisParams> as $params`).Show()
		assert.GreaterOrEqual(t, vars.Len(), 1)
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
