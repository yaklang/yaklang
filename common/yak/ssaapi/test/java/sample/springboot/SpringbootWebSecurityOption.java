import org.springframework.context.annotation.Configuration;
import org.springframework.security.config.annotation.authentication.builders.AuthenticationManagerBuilder;
import org.springframework.security.config.annotation.web.builders.HttpSecurity;
import org.springframework.security.config.annotation.web.configuration.EnableWebSecurity;
import org.springframework.security.config.annotation.web.configuration.WebSecurityConfigurerAdapter;

@Configuration
@EnableWebSecurity
public class SecurityConfig extends WebSecurityConfigurerAdapter {

    @Override
    protected void configure(HttpSecurity http) throws Exception {
        http
            // 启用 HSTS
            .headers()
                .httpStrictTransportSecurity()
                    .includeSubDomains(true)
                    .maxAgeInSeconds(31536000)  // 设置为一年
                .and()
            // 配置 CSP
                .contentSecurityPolicy("script-src 'self'; object-src 'none';")
                .and()
            // 启用 CSRF
            .csrf().and()
            // 配置权限规则
            .authorizeRequests()
                .antMatchers("/", "/home", "/public/**").permitAll()
                .antMatchers("/admin/**").hasRole("ADMIN")
                .anyRequest().authenticated()
                .and()
            // 配置登录
            .formLogin()
                .loginPage("/login")
                .permitAll()
                .and()
            // 配置登出
            .logout()
                .permitAll()
                .and()
            // 配置异常处理
            .exceptionHandling().accessDeniedPage("/403");
    }

    @Override
    protected void configure(AuthenticationManagerBuilder auth) throws Exception {
        auth
            .inMemoryAuthentication()
            .withUser("user").password("{noop}password").roles("USER")
            .and()
            .withUser("admin").password("{noop}admin").roles("ADMIN");
    }
}