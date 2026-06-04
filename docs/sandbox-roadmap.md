# 沙箱增强迭代计划

> 目标版本：v0.4
> 状态：🔶 进行中（14.5 ✅）

## 14.1 临时目录模式

- [ ] 14.1.1 sandbox/sandbox.go — 支持临时目录模式（`os.MkdirTemp`）
- [ ] 14.1.2 sandbox/sandbox.go — 配置化选择固定目录或临时目录
- [ ] 14.1.3 sandbox/sandbox.go — 临时目录自动清理（成功立即清理，失败保留可配置时间）
- [ ] 14.1.4 sandbox/workspace.go — 工作目录生命周期管理

## 14.2 更丰富的上下文工具

- [ ] 14.2.1 agent/tools.go — `cat` 工具增强（支持行号范围：`cat file.go:10-20`）
- [ ] 14.2.2 agent/tools.go — `tree` 工具（显示目录结构，可配置深度）
- [ ] 14.2.3 agent/tools.go — `git_log` 工具（显示 Git 提交历史）
- [ ] 14.2.4 agent/tools.go — `git_blame` 工具（显示文件修改历史）
- [ ] 14.2.5 agent/tools.go — `rg` 工具（ripgrep 搜索，比 grep 更快）
- [ ] 14.2.6 agent/tools.go — `find` 工具增强（支持 glob 模式）

## 14.3 配置化的超时和限制

- [ ] 14.3.1 config/schema.go — SandboxConfig 结构定义
  ```go
  type SandboxConfig struct {
      Mode           string        // "temp" | "fixed"
      BaseDir        string        // 固定模式的基础目录
      CommandTimeout time.Duration // 单命令超时
      TaskTimeout    time.Duration // 总任务超时
      MaxOutput      int           // 最大输出字节数
      MaxFileSize    int           // 最大文件大小
      CleanupAfter   time.Duration // 失败任务保留时间
  }
  ```
- [ ] 14.3.2 config.example.yaml — 添加 sandbox 配置段
- [ ] 14.3.3 sandbox/sandbox.go — 从配置加载参数（替代硬编码默认值）
- [ ] 14.3.4 agent/tools.go — 工具执行超时独立配置

## 14.4 安全增强

- [ ] 14.4.1 sandbox/sandbox.go — 文件路径验证（防止路径穿越攻击）
- [ ] 14.4.2 sandbox/sandbox.go — 文件大小限制（防止写入超大文件）
- [ ] 14.4.3 sandbox/sandbox.go — 命令参数验证（防止注入攻击）
- [ ] 14.4.4 sandbox/audit.go — 审计日志增强（记录文件操作内容摘要）

## 14.5 Agent 迭代控制配置化 ✅

- [x] 14.5.1 config/schema.go — AgentLoopConfig 结构定义
- [x] 14.5.2 config/schema.go — 在 AgentsConfig 中添加 AgentLoopConfig
- [x] 14.5.3 config.example.yaml — 添加 agents.loop 配置段
- [x] 14.5.4 agent/loop.go — AgentLoop 从配置加载参数（NewAgentLoopWithConfig）
- [x] 14.5.5 agents/runners.go — DevRunner/BugfixRunner 传递配置给 AgentLoop
- [x] 14.5.6 store/agent.go — Agent 表添加 loop_config 字段（JSON 存储）
- [x] 14.5.7 api/router.go — Agent CRUD API 支持 loop_config 字段
- [ ] 14.5.8 config/config.go — 配置加载时验证参数范围（max_iterations: 1-100, timeout: 1m-1h）
