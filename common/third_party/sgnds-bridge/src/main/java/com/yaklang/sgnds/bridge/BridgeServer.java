package com.yaklang.sgnds.bridge;

import com.yaklang.sgnds.codec.ProtocolHandler;
import com.yaklang.sgnds.pool.ConnectionPool;

import java.io.*;
import java.net.ServerSocket;
import java.net.Socket;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * 轻量级 TCP 服务，把 Yakit Go 端的请求转发给 SG-JDBC 驱动。
 *
 * 启动参数：
 *   java -jar yakit-sg-jdbc-bridge.jar [listenHost] [listenPort] [maxThreads]
 * 默认监听 127.0.0.1:19090。
 */
public class BridgeServer {

    private static final Logger LOG = Logger.getLogger(BridgeServer.class.getName());

    public static final String DEFAULT_HOST = "127.0.0.1";
    public static final int DEFAULT_PORT = 19090;
    public static final int DEFAULT_THREADS = 32;

    private final String host;
    private final int port;
    private final int maxThreads;
    private final AtomicBoolean running = new AtomicBoolean(false);
    private ServerSocket serverSocket;
    private ExecutorService executor;

    private final ConnectionPool pool;
    private final ProtocolHandler protocol;

    public BridgeServer(String host, int port, int maxThreads) {
        this.host = host != null ? host : DEFAULT_HOST;
        this.port = port > 0 ? port : DEFAULT_PORT;
        this.maxThreads = maxThreads > 0 ? maxThreads : DEFAULT_THREADS;
        this.pool = new ConnectionPool();
        this.protocol = new ProtocolHandler(pool);
    }

    public void start() throws IOException {
        if (running.compareAndSet(false, true)) {
            executor = Executors.newFixedThreadPool(maxThreads, r -> {
                Thread t = new Thread(r, "sgnds-bridge-worker");
                t.setDaemon(true);
                return t;
            });
            serverSocket = new ServerSocket(port, 128, java.net.InetAddress.getByName(host));
            LOG.info("SG-JDBC bridge listening on " + host + ":" + port);
            Thread acceptor = new Thread(this::acceptLoop, "sgnds-bridge-acceptor");
            acceptor.setDaemon(true);
            acceptor.start();
        }
    }

    private void acceptLoop() {
        while (running.get()) {
            try {
                Socket client = serverSocket.accept();
                client.setTcpNoDelay(true);
                client.setKeepAlive(true);
                executor.submit(() -> handleClient(client));
            } catch (IOException e) {
                if (running.get()) {
                    LOG.log(Level.WARNING, "accept failed", e);
                }
            }
        }
    }

    private void handleClient(Socket client) {
        try (Socket c = client;
             BufferedReader reader = new BufferedReader(new InputStreamReader(c.getInputStream(), "UTF-8"));
             BufferedWriter writer = new BufferedWriter(new OutputStreamWriter(c.getOutputStream(), "UTF-8"))) {
            String line;
            while ((line = reader.readLine()) != null) {
                if (line.isEmpty()) continue;
                String response = protocol.handle(line);
                writer.write(response);
                writer.write('\n');
                writer.flush();
            }
        } catch (IOException e) {
            LOG.log(Level.FINE, "client connection closed", e);
        }
    }

    public void stop() {
        if (running.compareAndSet(true, false)) {
            try {
                if (serverSocket != null) serverSocket.close();
            } catch (IOException ignored) {
            }
            pool.closeAll();
            if (executor != null) {
                executor.shutdown();
                try {
                    executor.awaitTermination(5, TimeUnit.SECONDS);
                } catch (InterruptedException ignored) {
                }
            }
        }
    }

    public static void main(String[] args) throws Exception {
        String host = DEFAULT_HOST;
        int port = DEFAULT_PORT;
        int threads = DEFAULT_THREADS;

        if (args.length >= 1) host = args[0];
        if (args.length >= 2) port = Integer.parseInt(args[1]);
        if (args.length >= 3) threads = Integer.parseInt(args[2]);

        BridgeServer server = new BridgeServer(host, port, threads);
        server.start();

        Runtime.getRuntime().addShutdownHook(new Thread(server::stop));

        // 阻塞主线程，JVM 不退出
        Thread.sleep(Long.MAX_VALUE);
    }
}
