package com.example.discoverydemo.web;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

@RestController
@RequestMapping("/api")
public class HelloController {

    @GetMapping("/health")
    public String health() {
        return "ok";
    }

    @GetMapping("/echo/{id}")
    public String echo(@PathVariable String id) {
        return id;
    }
}
