package java

import (
	"strings"
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
			checkRng := false
			params.Recursive(func(vo sfvm.ValueOperator) error {
				if v, ok := vo.(*ssaapi.Value); ok {
					for _, p := range v.Predecessors {
						p.Node.ShowWithRange()
						rng := p.Node.GetRange()
						// str := p.Node.StringWithSourceCode()
						// log.Infof("str: %s", str)
						if editor := rng.GetEditor(); editor != nil && editor.GetFilename() == "sqlmap.xml" {
							check = true
						}
						if strings.Contains(p.Node.StringWithRange(), "\"${id}\"\t22:63 - 22:68: ${id}") {
							checkRng = true
						}
					}
				}
				return nil
			})
			require.True(t, check)
			require.True(t, checkRng, "mybatis 位置信息错误")
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})

	t.Run("test mybatis weak param range with chinese", func(t *testing.T) {
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
	<!--    这是一个带有中文的注释 -->
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

			checkRng := false
			params.Recursive(func(vo sfvm.ValueOperator) error {
				if v, ok := vo.(*ssaapi.Value); ok {
					for _, p := range v.Predecessors {
						p.Node.ShowWithRange()
						if strings.Contains(p.Node.StringWithRange(), "\"${id}\"\t22:63 - 22:68: ${id}") {
							checkRng = true
						}
					}
				}
				return nil
			})
			require.True(t, checkRng, "mybatis 位置信息错误")
			return nil
		}, ssaapi.WithLanguage(consts.JAVA))
	})
}

func TestRealMyBatisSink1(t *testing.T) {
	vf := filesys.NewVirtualFs()
	vf.AddFile("ReportMapper.java", `package org.itstec.report.mapper;

import java.util.List;

import org.apache.ibatis.annotations.Mapper;
import org.apache.ibatis.annotations.Param;
import org.itstec.report.entity.Report;
import org.itstec.user.entity.User;

import com.xxx.mybatisplus.core.mapper.BaseMapper;

@Mapper
public interface ReportMapper extends BaseMapper<User>{

	int addRep(Report report);
	
	int updateRep(@Param("doctorId") String doctorId, @Param("userId") String userId, 
			@Param("dateTime") String dateTime, 
			@Param("subject") String subject, @Param("sData") double sData);
	
	List<Report> queryByUser(@Param("userId") String userId);
	
	List<Report> queryRep(@Param("dateTime") String dateTime, @Param("userId") String userId);
	
	Report queryRepByDoctor(@Param("dateTime") String dateTime, 
			@Param("userId") String userId, @Param("doctorId") String doctorId);
	
	List<Report> querySubjData(@Param("doctorId") String doctorId, @Param("dateTime") String dateTime, 
			@Param("subject") String subject, @Param("condition") String condition, @Param("sData") double sData);

	List<Report> queryCustOrder(@Param("doctorId") String doctorId, @Param("dateTime") String dateTime, 
			@Param("subject") String subject, @Param("condition") String condition, @Param("sData") double sData, @Param("order") String order);

	int queryCountUser(@Param("doctorId") String doctorId, @Param("dateTime") String dateTime, @Param("subject") String subject);
	
}
`)

	vf.AddFile("ReportMapper.xml", `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE mapper SYSTEM "file:///D:/mybatis-3-mapper.dtd">
<mapper namespace="org.itstec.report.mapper.ReportMapper">
	<select id="queryCountUser" resultType="org.itstec.report.entity.Report">
		SELECT COUNT(userId)
		FROM t_report
		WHERE dateTime = #{dateTime} and doctorId = #{doctorId}
		group by ${subject} > 0
	</select>
</mapper>`)
	ssatest.CheckWithFS(vf, t, func(programs ssaapi.Programs) error {
		prog := programs[0]
		res, err := prog.SyntaxFlowWithError(`<mybatisSink> as $params`)
		require.NoError(t, err)
		res.Show()
		params := res.GetValues("params")

		checkRng := false
		params.Recursive(func(vo sfvm.ValueOperator) error {
			if v, ok := vo.(*ssaapi.Value); ok {
				for _, p := range v.Predecessors {
					p.Node.ShowWithRange()
					if strings.Contains(p.Node.StringWithRange(), `"${subject}"	8:12 - 8:22: ${subject}`) {
						checkRng = true
					}
				}
			}
			return nil
		})
		require.True(t, checkRng, "mybatis 位置信息错误")
		return nil
	}, ssaapi.WithLanguage(consts.JAVA))
}
