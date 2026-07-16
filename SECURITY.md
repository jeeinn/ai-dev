# 安全策略

本项目（[jeeinn/ai-dev](https://github.com/jeeinn/ai-dev)）重视安全问题的负责任披露。

## 支持的版本

当前以仓库默认分支上的最新代码为准。发布 tag 后，优先修复仍受支持的发行版本；具体范围以 Release 说明为准。

## 如何报告漏洞

请**不要**在公开 Issue、Discussions 或 PR 中描述可利用细节，也**不要**附带 exploit、PoC 攻击载荷或可直接打穿生产环境的复现脚本。

推荐渠道（任选其一）：

1. **GitHub Security Advisories（优先）**  
   在仓库页面打开 [Security → Advisories → Report a vulnerability](https://github.com/jeeinn/ai-dev/security/advisories/new)，通过私密报告提交。

2. **私密 Issue（若 Advisories 不可用）**  
   创建 Issue 时仅写简短标题与「已通过其他渠道发送细节」的说明，**不要**在正文公开复现步骤；或联系维护者约定私密沟通方式后再补充细节。

报告中尽量包含：

- 受影响组件 / 版本（commit 或 tag）
- 影响说明（权限绕过、信息泄露、RCE 等）
- 最小复现步骤（足以验证即可，避免武器化脚本）
- 你认为的修复方向（可选）

## 处理流程

维护者收到报告后会尽量尽快确认与回复。确认属实的问题将优先私下修复，并在合适时机通过 Security Advisory 或 Release Note 披露；披露时通常会致谢报告者（若你希望匿名可说明）。

## 安全相关配置提醒

部署时请务必：

- 修改 Web UI 默认管理员密码（示例默认为 `admin` / `admin123`）
- 更换生产环境 `JWT_SECRET` / `auth.jwt_secret`
- 使用环境变量或本地未入库配置管理 Token / API Key（参见 `.env.example`、`docs/DEPLOYMENT.md`）

感谢帮助我们把项目做得更安全。

