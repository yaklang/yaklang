package com.foo.controller;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/users")
public class UserController {

    @GetMapping("/{id}")
    public String getUser() { return "user"; }

    @PostMapping
    public String createUser() { return "created"; }
}
