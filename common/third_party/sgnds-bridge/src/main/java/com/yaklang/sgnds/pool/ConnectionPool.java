package com.yaklang.sgnds.pool;

import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.SQLException;
import java.util.Map;
import java.util.Properties;
import java.util.UUID;
import java.util.concurrent.ConcurrentHashMap;
import java.util.logging.Level;
import java.util.logging.Logger;

/**
 * 维护 connId 到 JDBC Connection 的映射。
 *
 * 注意：SG-JDBC 驱动本身在隔离装置另一侧有连接池概念，
 * 本层只负责为每个 Yakit Go 端连接保留一个 JDBC Connection 句柄。
 */
public class ConnectionPool {

    private static final Logger LOG = Logger.getLogger(ConnectionPool.class.getName());

    private final Map<String, Connection> connections = new ConcurrentHashMap<>();

    static {
        try {
            Class.forName("sgcc.nds.jdbc.driver.NdsDriver");
            java.sql.Driver driver = (java.sql.Driver) Class.forName("sgcc.nds.jdbc.driver.NdsDriver").getDeclaredConstructor().newInstance();
            DriverManager.registerDriver(driver);
            LOG.info("SG-JDBC NdsDriver loaded and registered");
        } catch (Exception e) {
            LOG.log(Level.SEVERE, "failed to load SG-JDBC NdsDriver", e);
            throw new ExceptionInInitializerError(e);
        }
    }

    public String open(String url, Properties props) throws SQLException {
        Connection conn;
        try {
            conn = DriverManager.getConnection(url, props);
        } catch (SQLException e) {
            // 部分驱动在 JDK 17/模块化环境下无法通过 DriverManager 发现，
            //  fallback 直接用 NdsDriver.connect。
            if (e.getMessage() != null && e.getMessage().contains("No suitable driver")) {
                conn = directConnect(url, props);
            } else {
                throw e;
            }
        }
        String connId = UUID.randomUUID().toString().replace("-", "");
        connections.put(connId, conn);
        return connId;
    }

    private Connection directConnect(String url, Properties props) throws SQLException {
        try {
            java.sql.Driver driver = (java.sql.Driver) Class.forName("sgcc.nds.jdbc.driver.NdsDriver").getDeclaredConstructor().newInstance();
            Connection conn = driver.connect(url, props);
            if (conn == null) {
                throw new SQLException("SG-JDBC driver returned null connection (target unreachable or URL invalid)");
            }
            return conn;
        } catch (SQLException e) {
            throw e;
        } catch (Exception e) {
            // SG-JDBC 驱动在目标不可达时可能抛 NullPointerException 而不是 SQLException。
            String msg = e.getMessage();
            if (msg != null && msg.contains("serverPid")) {
                throw new SQLException("SG-JDBC connect failed: target isolation device unreachable");
            }
            throw new SQLException("direct connect failed: " + (msg != null ? msg : e.getClass().getName()), e);
        }
    }

    public Connection get(String connId) {
        return connections.get(connId);
    }

    public void close(String connId) {
        Connection conn = connections.remove(connId);
        if (conn != null) {
            try {
                conn.close();
            } catch (SQLException e) {
                LOG.log(Level.WARNING, "close connection failed", e);
            }
        }
    }

    public void closeAll() {
        for (Connection conn : connections.values()) {
            try {
                conn.close();
            } catch (SQLException e) {
                LOG.log(Level.WARNING, "close connection failed", e);
            }
        }
        connections.clear();
    }
}
