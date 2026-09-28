package com.yaklang.sgnds.bridge;

import org.junit.After;
import org.junit.Before;
import org.junit.Test;

import java.io.*;
import java.net.Socket;
import java.nio.charset.StandardCharsets;

import static org.junit.Assert.*;

/**
 * 本地单元测试：启动 BridgeServer，用 socket 发送文本协议。
 * 没有真实隔离装置时，open 会失败，但可验证协议层与错误转发。
 */
public class BridgeServerTest {

    private BridgeServer server;

    @Before
    public void setUp() throws Exception {
        server = new BridgeServer("127.0.0.1", 19091, 2);
        server.start();
        Thread.sleep(300);
    }

    @After
    public void tearDown() {
        server.stop();
    }

    @Test
    public void testPing() throws Exception {
        String res = send("action=ping");
        assertTrue(res.contains("ok=true"));
        assertTrue(res.contains("pong=true"));
    }

    @Test
    public void testOpenToUnreachableDevice() throws Exception {
        String url = "jdbc:nds://127.0.0.1:18600/v_test?appname=yakit_test";
        String req = "action=open&url=" + escape(url) + "&user=user&password=pass";
        String res = send(req);
        assertTrue(res.contains("ok=false"));
        assertTrue(res.contains("error="));
        System.out.println("open error: " + res);
    }

    private String send(String line) throws Exception {
        try (Socket s = new Socket("127.0.0.1", 19091);
             BufferedWriter w = new BufferedWriter(new OutputStreamWriter(s.getOutputStream(), StandardCharsets.UTF_8));
             BufferedReader r = new BufferedReader(new InputStreamReader(s.getInputStream(), StandardCharsets.UTF_8))) {
            w.write(line);
            w.write('\n');
            w.flush();
            String reply = r.readLine();
            assertNotNull("server returned nothing", reply);
            return reply;
        }
    }

    private String escape(String s) {
        return s.replace("&", "%26")
                .replace("=", "%3D")
                .replace("\n", "%0A")
                .replace("\r", "%0D")
                .replace("|", "%7C")
                .replace(",", "%2C");
    }
}
