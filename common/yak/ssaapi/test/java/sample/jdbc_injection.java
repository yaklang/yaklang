package org.vuln.javasec.controller.basevul.sqli;

import org.springframework.beans.factory.annotation.Value;
import org.springframework.stereotype.Controller;
import org.springframework.ui.Model;
import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.RequestMapping;

import java.sql.*;


@Controller
@RequestMapping("/home/sqli/jdbc")
public class JDBC {

    @Value("${spring.datasource.url}")
    private String db_url;

    @Value("${spring.datasource.username}")
    private String db_user;

    @Value("${spring.datasource.password}")
    private String db_pass;


    @GetMapping("/error_based")
    public String error_based(String id, Model model) {

        StringBuilder result = new StringBuilder();

        try {
            Class.forName("com.mysql.cj.jdbc.Driver");
            Connection conn = DriverManager.getConnection(db_url, db_user, db_pass);

            Statement stmt = conn.createStatement();
            String sql = "select * from users where id = '" + id + "'";
            result.append("执行SQL语句: ").append(sql).append(System.lineSeparator());
            result.append("查询结果： ").append(System.lineSeparator());

            ResultSet rs = stmt.executeQuery(sql);

            while (rs.next()) {
                String res_name = rs.getString("username");
                String res_pass = rs.getString("password");
                String info = String.format("%s : %s%n", res_name, res_pass);
                result.append(info).append(System.lineSeparator());
            }

            rs.close();
            stmt.close();
            conn.close();
        } catch (Exception e) {
            result.append(e).append(System.lineSeparator());
        }
        model.addAttribute("results", result.toString());
        return "basevul/sqli/jdbc_error_based";
    }


    @GetMapping("/int_based")
    public String int_based(String id, Model model) {

        StringBuilder result = new StringBuilder();

        try {
            Class.forName("com.mysql.cj.jdbc.Driver");
            Connection conn = DriverManager.getConnection(db_url, db_user, db_pass);

            String sql = "select * from users where id = " + id;
            result.append("执行SQL语句: ").append(sql).append(System.lineSeparator());
            result.append("查询结果: ").append(System.lineSeparator());
            PreparedStatement st = conn.prepareStatement(sql);
            ResultSet rs = st.executeQuery();

            while (rs.next()) {
                String res_name = rs.getString("username");
                String res_pass = rs.getString("password");
                String info = String.format("%s : %s%n", res_name, res_pass);
                result.append(info).append(System.lineSeparator());
            }
            rs.close();
            st.close();
            conn.close();
            model.addAttribute("results", result.toString());
        } catch (Exception e) {
            model.addAttribute("results", e.toString());
        }
        return "basevul/sqli/jdbc_int_based";
    }

    @GetMapping("/blind_time_based")
    public String blind_time_based(String id, Model model) {
        try {
            Class.forName("com.mysql.cj.jdbc.Driver");
            Connection conn = DriverManager.getConnection(db_url, db_user, db_pass);

            String sql = "select * from users where id = " + id;
            PreparedStatement st = conn.prepareStatement(sql);
            ResultSet rs = st.executeQuery();

            if (rs.next()) {
                model.addAttribute("results", "查询成功！");
                return "basevul/sqli/jdbc_blind_time_based";
            }
            rs.close();
            st.close();
            conn.close();
        } catch (Exception e) {
            e.printStackTrace();
        }
        model.addAttribute("results", "查询失败！");
        return "basevul/sqli/jdbc_blind_time_based";
    }

}