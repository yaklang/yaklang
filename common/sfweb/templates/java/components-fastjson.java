// controller
package com.example.fastjsondemo.controller;

import com.alibaba.fastjson.JSON;
import com.example.fastjsondemo.model.User;
import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api")
public class UserController {

    @PostMapping("/user")
    public User createUser(@RequestBody String jsonString) {
        // 使用 FastJSON 将 JSON 字符串解析为 User 对象
        User user = JSON.parseObject(jsonString, User.class);
        System.out.println("Received user: " + user);
        return user;
    }

    @GetMapping("/user")
    public String getUser() {
        // 创建一个 User 对象并将其转换为 JSON 字符串
        User user = new User("John", 30);
        String jsonString = JSON.toJSONString(user);
        System.out.println("Generated JSON: " + jsonString);
        return jsonString;
    }
}
// config
import com.alibaba.fastjson.support.config.FastJsonConfig;
import com.alibaba.fastjson.support.spring.FastJsonHttpMessageConverter;
import org.springframework.context.annotation.Configuration;
import org.springframework.http.converter.HttpMessageConverter;
import org.springframework.web.servlet.config.annotation.WebMvcConfigurer;

import java.util.List;

@Configuration
public class FastJsonConfig implements WebMvcConfigurer {

    @Override
    public void configureMessageConverters(List<HttpMessageConverter<?>> converters) {
        FastJsonHttpMessageConverter fastConverter = new FastJsonHttpMessageConverter();
        FastJsonConfig fastJsonConfig = new FastJsonConfig();
        fastJsonConfig.setDateFormat("yyyy-MM-dd HH:mm:ss");
        fastConverter.setFastJsonConfig(fastJsonConfig);
        converters.add(0, fastConverter);
    }
}