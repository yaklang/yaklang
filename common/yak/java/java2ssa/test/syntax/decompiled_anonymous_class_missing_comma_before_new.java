package com.example;

import java.util.concurrent.LinkedBlockingQueue;
import java.util.concurrent.RejectedExecutionHandler;
import java.util.concurrent.ThreadFactory;
import java.util.concurrent.ThreadPoolExecutor;
import java.util.concurrent.TimeUnit;

public class Main {
    void run() {
        ThreadPoolExecutor executor = new ThreadPoolExecutor(10, 100, 60L, TimeUnit.SECONDS, new LinkedBlockingQueue<>(1000), new ThreadFactory() {
            public Thread newThread(Runnable r) {
                return new Thread(r, "xxl-rpc");
            }
        }new RejectedExecutionHandler() {
            public void rejectedExecution(Runnable r, ThreadPoolExecutor executor) {}
        });
    }
}
