package com.example;

import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/items")
public class ItemController {

    @GetMapping
    public String list() { return "[]"; }

    @PostMapping
    public String create() { return "created"; }

    @DeleteMapping("/{id}")
    public String delete() { return "deleted"; }
}
