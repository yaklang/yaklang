package com.example;

import java.util.Timer;
import java.util.TimerTask;

public class Main {
    long reconnectInterval;

    void run() {
        Timer timer = new Timer("tmc-reconnect", true);
        timer.schedule(new TimerTask() {
            public void run() {}
        }this.reconnectInterval, this.reconnectInterval);
    }
}
