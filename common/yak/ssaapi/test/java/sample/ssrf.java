package com.vuln.controller;

import com.squareup.okhttp.Call;
import com.squareup.okhttp.OkHttpClient;
import com.squareup.okhttp.Response;
import org.apache.http.HttpResponse;
import org.apache.http.client.methods.HttpGet;
import org.apache.http.impl.client.DefaultHttpClient;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RequestParam;
import org.springframework.web.bind.annotation.RestController;
import org.apache.http.client.fluent.Request;

import java.io.IOException;
import java.io.InputStream;
import java.net.HttpURLConnection;
import java.net.URL;

@RestController
@RequestMapping(value = "/ssrf")
public class SSRFController {

    @RequestMapping(value = "/one")
    public String One(@RequestParam(value = "url") String imageUrl) {
        try {
            URL url = new URL(imageUrl);
            HttpURLConnection connection = (HttpURLConnection) url.openConnection();
            connection.setRequestMethod("GET");
            return connection.getResponseMessage();
        } catch (IOException var3) {
            System.out.println(var3);
            return "Hello";
        }
    }

    @RequestMapping(value = "/two")
    public String Two(@RequestParam(value = "url") String imageUrl) {
        try {
            URL url = new URL(imageUrl);
            HttpResponse response = Request.Get(String.valueOf(url)).execute().returnResponse();
            return response.toString();
        } catch (IOException var1) {
            System.out.println(var1);
            return "Hello";
        }
    }

    @RequestMapping(value = "/three")
    public String Three(@RequestParam(value = "url") String imageUrl) {
        try {
            URL url = new URL(imageUrl);
            OkHttpClient client = new OkHttpClient();
            com.squareup.okhttp.Request request = new com.squareup.okhttp.Request.Builder().get().url(url).build();
            Call call = client.newCall(request);
            Response response = call.execute();
            return response.toString();
        } catch (IOException var1) {
            System.out.println(var1);
            return "Hello";
        }
    }

    @RequestMapping(value = "/four")
    public String Four(@RequestParam(value = "url") String imageUrl) {
        try {
            DefaultHttpClient client = new DefaultHttpClient();
            HttpGet get = new HttpGet(imageUrl);
            HttpResponse response = client.execute(get);
            return response.toString();
        } catch (IOException var1) {
            System.out.println(var1);
            return "Hello";
        }
    }

    @RequestMapping(value = "five")
    public String Five(@RequestParam(value = "url") String imageUrl) {
        try {
            URL url = new URL(imageUrl);
            InputStream inputStream = url.openStream();
            return String.valueOf(inputStream.read());
        } catch (IOException var1) {
            System.out.println(var1);
            return "Hello";
        }
    }
}