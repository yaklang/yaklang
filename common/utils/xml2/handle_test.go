package xml2

import (
	"encoding/xml"
	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

const code = `<?xml version="1.0" encoding="UTF-8" ?>
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
</mapper>`

type desc struct {
	Name string
	Attr map[string]string
	Text [][]byte
}

func TestHandle(t *testing.T) {
	stack := utils.NewStack[*desc]()

	var m []*desc

	stack.Push(&desc{
		Name: "",
		Attr: make(map[string]string),
	})
	Handle(code, WithStartElementHandler(func(element xml.StartElement) {
		var name string = element.Name.Space
		if name != "" {
			name += ":"
		}
		name += element.Name.Local

		d := &desc{
			Name: name,
			Attr: make(map[string]string),
		}
		m = append(m, d)
		stack.Push(d)
	}), WithEndElementHandler(func(element xml.EndElement) {
		stack.Pop()
	}), WithCharDataHandler(func(data xml.CharData, offset int64) {
		top := stack.Peek()
		top.Text = append(top.Text, data)
	}), WithDirectiveHandler(func(directive xml.Directive) bool {
		utils.MatchAnyOfSubString(directive, "mybatis-3-mapper.dtd")
		return true
	}))
	assert.Greater(t, len(m), 0)
}

func TestHandle_ISO8859_1(t *testing.T) {
	// Hadoop web.xml / resources declare iso-8859-1 with a DOCTYPE.
	// Without CharsetReader the decoder errors with "CharsetReader is nil"
	// and the XML is skipped. With CharsetReader the ISO-8859-1 bytes
	// decode and the handler fires.
	prefix := `<?xml version="1.0" encoding="iso-8859-1"?>
<!DOCTYPE web-app PUBLIC "-//Sun Microsystems, Inc.//DTD Web Application 2.3//EN" "http://java.sun.com/dtd/web-app_2_3.dtd">
<web-app><filter><filter-name>`
	suffix := `</filter-name></filter></web-app>`
	src := prefix + "caf" + string([]byte{0xe9}) + suffix
	var elementFound bool
	var got string
	Handle(src, WithStartElementHandler(func(e xml.StartElement) {
		if e.Name.Local == "filter-name" {
			elementFound = true
		}
	}), WithCharDataHandler(func(data xml.CharData, _ int64) {
		got = string(data)
	}))
	assert.True(t, elementFound, "iso-8859-1 XML with DOCTYPE should be parsed")
	assert.Equal(t, "café", got, "ISO-8859-1 bytes should be decoded as UTF-8")
}
