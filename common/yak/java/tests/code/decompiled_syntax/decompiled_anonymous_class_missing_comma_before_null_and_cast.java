package com.example;

class HttpPost {}
class Entity {}
enum Method { POST; }

abstract class Setting {
    public void headerSet(HttpPost req) {}
}

public class Main {
    public static String execute(Method m, String url, Setting setting, Object entity) {
        return "";
    }

    public static String postHeader(String reqURL) {
        return execute(Method.POST, reqURL, new Setting() {
            public void headerSet(HttpPost req) {}
        }null);
    }

    public static String postJson(String reqURL, Entity stringEntity) {
        return execute(Method.POST, reqURL, new Setting() {
            public void headerSet(HttpPost req) {}
        }(Object)stringEntity);
    }
}
