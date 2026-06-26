`twofa` 库提供二步验证（TOTP）能力，根据密钥生成与校验动态验证码，用于自动化登录带两步验证的系统或测试 2FA 实现。

典型使用场景：

- 生成验证码：`twofa.TOTPCode(secret)` 生成当前验证码，`twofa.GetUTCCode` 基于 UTC 生成。
- 校验验证码：`twofa.TOTPVerify(secret, code)` / `twofa.VerifyUTCCode` 校验是否有效。

与相邻库的关系：`twofa` 与 `mfa` 功能一致（TOTP），常配合 `http`/`poc`（自动登录）、`brute`（认证测试）使用。
