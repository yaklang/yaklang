package com.yaklang.sgnds.codec;

import com.yaklang.sgnds.pool.ConnectionPool;

import java.sql.*;
import java.util.ArrayList;
import java.util.List;
import java.util.Properties;

/**
 * 处理简单文本协议请求，映射到 JDBC 操作。
 *
 * 请求格式：每行一个 key=value 列表，用 '&' 分隔，类似 URL query string。
 *   action=ping
 *   action=open&url=jdbc:nds://...&user=u&password=p
 *   action=close&connId=xxx
 *   action=query&connId=xxx&sql=select+...&params=1,2
 *   action=exec&connId=xxx&sql=update+...&params=a,b
 *   action=pingConnection&connId=xxx
 *
 * 响应格式：同样 key=value，多个字段用 '&' 分隔。
 *   ok=true&pong=true
 *   ok=true&connId=xxx
 *   ok=true&columns=ID,NAME&rows=1|foo,2|bar
 *   ok=false&error=...
 */
public class ProtocolHandler {

    private final ConnectionPool pool;

    public ProtocolHandler(ConnectionPool pool) {
        this.pool = pool;
    }

    public String handle(String line) {
        try {
            return doHandle(line);
        } catch (Exception e) {
            return encodeError(e);
        }
    }

    private String doHandle(String line) throws Exception {
        Params p = new Params(line);
        String action = p.get("action");
        if (action == null) throw new IllegalArgumentException("missing action");
        switch (action) {
            case "ping":
                return "ok=true&pong=true";
            case "open":
                return handleOpen(p);
            case "close":
                return handleClose(p);
            case "pingConnection":
                return handlePingConnection(p);
            case "query":
                return handleQuery(p);
            case "exec":
                return handleExec(p);
            default:
                throw new IllegalArgumentException("unknown action: " + action);
        }
    }

    private String handleOpen(Params p) throws SQLException {
        String url = p.get("url");
        String user = p.get("user");
        String password = p.get("password");
        if (url == null) throw new IllegalArgumentException("missing url");

        Properties props = new Properties();
        if (user != null) props.setProperty("user", user);
        if (password != null) props.setProperty("password", password);

        String connId = pool.open(url, props);
        return "ok=true&connId=" + escape(connId);
    }

    private String handleClose(Params p) throws SQLException {
        String connId = p.get("connId");
        pool.close(connId);
        return "ok=true";
    }

    private String handlePingConnection(Params p) throws SQLException {
        String connId = p.get("connId");
        Connection conn = pool.get(connId);
        boolean alive = conn != null && !conn.isClosed();
        return alive ? "ok=true" : "ok=false&error=connection+closed+or+not+found";
    }

    private String handleQuery(Params p) throws SQLException {
        String connId = p.get("connId");
        String sql = p.get("sql");
        if (connId == null || sql == null) throw new IllegalArgumentException("missing connId or sql");
        List<String> params = p.getList("params");

        Connection conn = pool.get(connId);
        if (conn == null) throw new SQLException("connection not found: " + connId);

        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            bindParams(ps, params);
            try (ResultSet rs = ps.executeQuery()) {
                return resultSetToString(rs);
            }
        }
    }

    private String handleExec(Params p) throws SQLException {
        String connId = p.get("connId");
        String sql = p.get("sql");
        if (connId == null || sql == null) throw new IllegalArgumentException("missing connId or sql");
        List<String> params = p.getList("params");

        Connection conn = pool.get(connId);
        if (conn == null) throw new SQLException("connection not found: " + connId);

        try (PreparedStatement ps = conn.prepareStatement(sql)) {
            bindParams(ps, params);
            int affected = ps.executeUpdate();
            return "ok=true&affectedRows=" + affected;
        }
    }

    private void bindParams(PreparedStatement ps, List<String> params) throws SQLException {
        for (int i = 0; i < params.size(); i++) {
            String v = params.get(i);
            if (v == null || "null".equalsIgnoreCase(v)) {
                ps.setNull(i + 1, Types.NULL);
            } else if ("true".equalsIgnoreCase(v) || "false".equalsIgnoreCase(v)) {
                ps.setBoolean(i + 1, Boolean.parseBoolean(v));
            } else {
                try {
                    ps.setObject(i + 1, Long.parseLong(v));
                } catch (NumberFormatException e1) {
                    try {
                        ps.setObject(i + 1, Double.parseDouble(v));
                    } catch (NumberFormatException e2) {
                        ps.setString(i + 1, v);
                    }
                }
            }
        }
    }

    private String resultSetToString(ResultSet rs) throws SQLException {
        ResultSetMetaData meta = rs.getMetaData();
        int cols = meta.getColumnCount();
        StringBuilder colBuf = new StringBuilder();
        for (int i = 1; i <= cols; i++) {
            if (i > 1) colBuf.append(',');
            colBuf.append(escape(meta.getColumnLabel(i)));
        }

        StringBuilder rowBuf = new StringBuilder();
        while (rs.next()) {
            if (rowBuf.length() > 0) rowBuf.append(',');
            for (int i = 1; i <= cols; i++) {
                if (i > 1) rowBuf.append('|');
                Object v = rs.getObject(i);
                rowBuf.append(v == null ? "" : escape(v.toString()));
            }
        }

        return "ok=true&columns=" + colBuf + "&rows=" + rowBuf;
    }

    private String encodeError(Exception e) {
        String msg = e.getMessage();
        return "ok=false&error=" + escape(msg != null ? msg : e.getClass().getName())
                + "&errorClass=" + escape(e.getClass().getName());
    }

    private String escape(String s) {
        if (s == null) return "";
        return s.replace("&", "%26")
                .replace("=", "%3D")
                .replace("\n", "%0A")
                .replace("\r", "%0D")
                .replace("|", "%7C")
                .replace(",", "%2C");
    }

    private static class Params {
        private final java.util.Map<String, String> map = new java.util.HashMap<>();

        Params(String line) {
            if (line == null || line.isEmpty()) return;
            String[] parts = line.split("&");
            for (String part : parts) {
                int idx = part.indexOf('=');
                if (idx > 0) {
                    String key = part.substring(0, idx);
                    String val = unescape(part.substring(idx + 1));
                    map.put(key, val);
                }
            }
        }

        String get(String key) {
            return map.get(key);
        }

        List<String> getList(String key) {
            String v = map.get(key);
            if (v == null || v.isEmpty()) return new ArrayList<>();
            String[] parts = v.split(",");
            List<String> res = new ArrayList<>(parts.length);
            for (String p : parts) res.add(unescape(p));
            return res;
        }

        String unescape(String s) {
            return s.replace("%26", "&")
                    .replace("%3D", "=")
                    .replace("%0A", "\n")
                    .replace("%0D", "\r")
                    .replace("%7C", "|")
                    .replace("%2C", ",");
        }
    }
}
