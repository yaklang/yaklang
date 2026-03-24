package com.example;

import java.util.Arrays;
import java.util.List;
import java.util.Map;
import java.util.stream.Collectors;

public class Main {
    public Map<String, List<String>> run() {
        return Arrays.asList("a", "a").stream().collect(Collectors.toMap(v -> v, v -> {
            List<String> list = new java.util.ArrayList<>();
            list.add(v);
            return list;
        }(list1, list2) -> {
            list1.addAll(list2);
            return list1;
        }));
    }
}
