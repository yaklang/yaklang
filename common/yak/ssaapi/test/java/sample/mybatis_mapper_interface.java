package com.example.mapper;

import org.apache.ibatis.annotations.Select;
import com.example.model.User;

public interface UserMapper {
    // 通过 ID 查询用户
    @Select("SELECT * FROM users WHERE id = #{id}")
    User getUserById(Integer id);

    // 插入新用户，并返回自动生成的主键
    @Insert("INSERT INTO users(name, email) VALUES(#{name}, #{email})")
    @Options(useGeneratedKeys = true, keyProperty = "id")
    void insertUser(User user);

    // 更新用户信息
    @Update("UPDATE users SET name = #{name}, email = #{email} WHERE id = #{id}")
    void updateUser(User user);

    // 删除用户
    @Delete("DELETE FROM users WHERE id = #{id}")
    void deleteUser(Integer id);
}