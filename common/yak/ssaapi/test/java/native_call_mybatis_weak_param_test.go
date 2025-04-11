package java

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/syntaxflow/sfvm"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssaapi"
	"github.com/yaklang/yaklang/common/yak/ssaapi/test/ssatest"
)

func TestNativeCall_MybatisSupport(t *testing.T) {
	t.Run("test mybatis weak param1", func(t *testing.T) {
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
		ssatest.CheckWithFS(f, t, func(prog ssaapi.Programs) error {
			vals, err := prog.SyntaxFlowWithError(`<mybatisSink> as $params`)
			require.NoError(t, err)
			vals.Show()
			params := vals.GetValues("params")
			require.Contains(t, params.String(), "Parameter-user")
			require.Equal(t, 1, params.Len())
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test mybatis weak param2", func(t *testing.T) {
		f := filesys.NewVirtualFs()
		f.AddFile(`DictMapper.xml`, `
		<?xml version="1.0" encoding="UTF-8" ?>
		<!DOCTYPE mapper
				PUBLIC "-//mybatis.org//DTD Mapper 3.0//EN"
				"http://mybatis.org/dtd/mybatis-3-mapper.dtd">
		<mapper namespace="com.codermy.myspringsecurityplus.admin.dao.DictDao">
			<sql id="selectDictVo">
				select di.dict_id,di.dict_name,di.description,di.sort,di.create_by,di.update_by,di.create_time,di.update_time
				from my_dict di
			</sql>
			<select id="getFuzzyDictByPage" resultType="com.codermy.myspringsecurityplus.admin.entity.MyDict">
				<include refid="selectDictVo"/>
				<where>
					<if test="dictName != null and dictName != ''">
						AND di.dict_name like CONCAT('%', ${dictName}, '%')
					</if>
				</where>
			</select>
			<select id="getDictByName" parameterType="string" resultType="com.codermy.myspringsecurityplus.admin.entity.MyDict">
				<include refid="selectDictVo"/>
				where di.dict_name = #{dictName}
			</select>
			<update id="updateDict" parameterType="com.codermy.myspringsecurityplus.admin.entity.MyDict">
				update my_dict
				<set>
					<if test="dictName != null and dictName != ''">dict_name = #{dictName},</if>
					<if test="description != null">description = #{description},</if>
					<if test="sort != null and sort != ''">sort = #{sort},</if>
					update_time = #{updateTime}
				</set>
				where dict_id = #{dictId}
			</update>
		</mapper>
`)
		f.AddFile("DictMapper.java", `package com.mycompany.myapp;
@Mapper
public interface DictDao {
    List<MyDict> getFuzzyDictByPage(MyDict myDict);
    MyDict getDictByName(String dictName);
    @Select("select di.dict_id,di.dict_name,di.description,di.sort,di.create_time,di.update_time from my_dict di  where di.dict_id = #{dictId}")
    MyDict getDictById(Integer dictId);
    int deleteDictByIds(Integer[] dictIds);
}

`)
		ssatest.CheckWithFS(f, t, func(programs ssaapi.Programs) error {
			prog := programs[0]
			param := prog.SyntaxFlowChain(`<mybatisSink> as $params`).Show()
			require.Contains(t, param.String(), "Parameter-myDict")
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test mybatis weak param range", func(t *testing.T) {
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
			res, err := prog.SyntaxFlowWithError(`<mybatisSink> as $params`)
			require.NoError(t, err)
			res.Show()
			params := res.GetValues("params")

			check := false
			params.Recursive(func(vo sfvm.ValueOperator) error {
				if v, ok := vo.(*ssaapi.Value); ok {
					for _, p := range v.Predecessors {
						// p.Node.ShowWithRange()
						rng := p.Node.GetRange()
						// str := p.Node.StringWithSourceCode()
						// log.Infof("str: %s", str)
						if editor := rng.GetEditor(); editor != nil && editor.GetFilename() == "sqlmap.xml" {
							check = true
						}
					}
				}
				return nil
			})
			require.True(t, check)
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}
