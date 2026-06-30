package com.example.demo;
import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RestController;
@SpringBootApplication
@RestController
public class Application {
    private static final String API_KEY = "AKIAIOSFODNN7EXAMPLE";
    public static void main(String[] args) { SpringApplication.run(Application.class, args); }
    @GetMapping("/hello")
    public String hello() { return "hi"; }
}
