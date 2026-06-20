`mfa` 库提供多因素认证（TOTP）相关能力，根据密钥生成/校验动态验证码，常用于自动化登录带二次验证的系统或测试 MFA 实现。

典型使用场景：

- 生成验证码：`mfa.TOTPCode(secret)` 生成当前 TOTP 验证码，`mfa.GetUTCCode` 基于 UTC 生成。
- 校验验证码：`mfa.TOTPVerify(secret, code)` / `mfa.VerifyUTCCode` 校验验证码是否有效。

与相邻库的关系：`mfa` 与 `twofa`（二步验证）定位相近，常配合 `http`/`poc`（自动登录流程）、`brute`（认证测试）使用。
