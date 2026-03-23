package com.example;

import java.util.Arrays;
import java.util.Map;
import java.util.stream.Collectors;

public class Main {
    public Map<String, String> run() {
        return Arrays.asList("a").stream().collect(Collectors.toMap(v -> v, v -> v, ()));
    }
}
