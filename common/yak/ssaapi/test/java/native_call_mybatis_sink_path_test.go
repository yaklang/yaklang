package java

import (
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
	"testing"
)

func TestMyBatisSinkPath(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("SqliMapper.xml", `<?xml version="1.0" encoding="UTF-8" ?>
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
</mapper>`)
	vf.AddFile("SqliMapper.java", `package com.mycompany.myapp;

import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;

import java.util.List;

@Mapper
public interface UserMapper {

    User getUser(@Param("id") Long id);

    void insertUser(User user);

    void updateUser(User user);

    void deleteUser(@Param("id") Long id);

    List<User> getAllUsers(); // 可选，获取所有用户
}`)
	vf.AddFile("MyBatisController.java", `package com.mycompany.myapp;

import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import java.util.List;

@RestController
@RequestMapping("/users")
public class UserController {

    @Autowired
    private UserMapper userMapper;

    @GetMapping("/{id}")
    public ResponseEntity<User> getUser(@PathVariable Long id) {
        User user = userMapper.getUser(id);
        return user != null ? ResponseEntity.ok(user) : ResponseEntity.notFound().build();
    }

    @PostMapping
    public ResponseEntity<User> insertUser(@RequestBody User user) {
        userMapper.insertUser(user);
        return ResponseEntity.ok(user);
    }

    @PutMapping("/{id}")
    public ResponseEntity<User> updateUser(@PathVariable Long id, @RequestBody User user) {
        user.setId(id); // 确保更新的用户 ID 是正确的
        userMapper.updateUser(user);
        return ResponseEntity.ok(user);
    }

    @DeleteMapping("/{id}")
    public ResponseEntity<Void> deleteUser(@PathVariable Long id) {
        userMapper.deleteUser(id);
        return ResponseEntity.noContent().build();
    }

    @GetMapping
    public ResponseEntity<List<User>> getAllUsers() {
        List<User> users = userMapper.getAllUsers();
        return ResponseEntity.ok(users);
    }
}`)

	rule := `
*Mapping.__ref__?{opcode: function} as $start;
$start<getFormalParams>?{opcode: param && !have: this} as $params;
$params?{!<typeName>?{have:'javax.servlet.http'}} as $source;
<mybatisSink()> #{
    until:<<<UNTIL
    * & $source
UNTIL
}-> as $result
`
	ssatest.CheckSyntaxFlowGraphInfoWithFs(
		t,
		vf,
		rule,
		"result",
		map[string]ssatest.GraphNodeInfo{
			"n1": {
				Label: "\"${id}\"",
				CodeRange: &ssaapi.CodeRange{
					URL:            "/SqliMapper.xml",
					StartLine:      22,
					StartColumn:    63,
					EndLine:        22,
					EndColumn:      68,
					SourceCodeLine: 18,
				},
			},
		},
		nil,
		ssaapi.WithLanguage(consts.JAVA),
	)
}
