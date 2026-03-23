package com.example;

import java.util.ArrayList;
import java.util.Enumeration;
import java.util.List;
import java.util.Vector;

public class Main {
    public int count(Vector values) {
        int count = 0;
        for (Enumeration enum = values.elements(); enum.hasMoreElements(); ) {
            enum.nextElement();
            count++;
        }
        return count;
    }

    public List<Object> collect(Vector values) {
        List<Object> result = new ArrayList<>();
        Enumeration enum = values.elements();
        if (enum != null) {
            while (enum.hasMoreElements()) {
                result.add(enum.nextElement());
            }
        }
        return result;
    }
}
