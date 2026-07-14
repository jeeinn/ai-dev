# 任务执行文档

> 当前阶段：Phase 15 Web UI 优化 ✅ 已完成

---

## Phase 1-13：已完成 ✅

详见 [归档文档](20260604-TASKS.md)。

## Phase 14：沙箱增强

详见 [沙箱迭代计划](sandbox-roadmap.md)。

## Phase 15：Web UI 优化 ✅

### 15.1 Bug 修复 ✅

- [x] 用户管理页面返回 HTML（添加 /api/users 端点 + JWT 认证）
- [x] 内置模板为空（/api/prompt-templates 返回内置 + 自定义模板）
- [x] Prompt 版本记录（Agent 编辑时自动创建 prompt_history）
- [x] 查看模板按钮无响应（添加对话框）
- [x] AgentDetail 页面空白（form 初始化 + 错误处理 + Array.isArray 保护）
- [x] Agent 列表触发规则数量显示
- [x] 启动时 prompt.templates 警告（配置校验 + 详细 WARN 提示）
- [x] Dashboard /api/tasks 返回格式适配

### 15.2 系统配置页面 ✅

- [x] system_config 表 + store CRUD
- [x] ConfigManager（DB 配置 > 文件配置 > 默认值）
- [x] GET/PUT/DELETE /api/config 端点（含 key 校验）
- [x] LLM Registry 热更新
- [x] SystemConfig.vue 标签页布局（Gitea 连接 / LLM 配置 / 任务调度 / Agent 默认参数 / Prompt 模板）
- [x] 配置项说明 tips（MaxTokens/Temperature 含义区分）
- [x] Prompt 模板管理（查看 / 新增 / 删除自定义模板，DB 持久化）

### 15.3 Agent 管理增强 ✅

- [x] Agent 表单分组折叠（核心字段直接展示，高级配置/Lloop 折叠）
- [x] 模板选择下拉框（从 /api/prompt-templates 动态加载）
- [x] Provider 下拉从配置动态读取（不再硬编码）
- [x] 创建表单从 agents.defaults 读取默认值
- [x] Agent 详情页（AgentDetail.vue）
  - [x] Tab 1：基本信息（编辑配置 + 模板变量说明）
  - [x] Tab 2：触发规则（Route CRUD + 快捷配置 + 预计执行行为）
  - [x] Tab 3：Prompt 版本历史（详情查看 + 回滚 + 删除）
- [x] Agent 列表名称可点击 + 详情按钮
- [x] 客户端分页

### 15.4 触发规则增强 ✅

- [x] 后端 GET /api/agents/:id/routes
- [x] 触发规则 Tab — 表格展示 + 添加/删除
- [x] 添加规则表单：事件类型 + 动作 + Label + Assignee + Mention + 优先级
- [x] 预设快捷配置：一键添加常用规则（ai:analyze/ai:solve/ai:fix/ai:review/@mention）
- [x] 预计执行行为列（根据 event+action+label 自动推断，图标+中文描述）
- [x] 防重复规则（CreateRoute 唯一性检查）
- [x] 优先级说明（值越大越优先）

### 15.5 任务列表 ✅

- [x] 服务端分页（limit/offset + total）
- [x] 筛选：状态 / 任务类型 / Agent
- [x] 分页条（支持切换每页条数）
- [x] Agent 名称显示（非 ID）
- [x] 类型列 Tag 样式

### 15.6 Dashboard 优化 ✅

- [x] 新用户引导卡片（无 Agent 时显示，三步跳转）
- [x] 最近任务 / Agent 列表限 10 条
- [x] 查看全部链接跳转
- [x] Agent 名称可点击跳转详情

### 15.7 配置值生效链路 ✅

- [x] RunnerFactory 持有 defaultMaxTokens / defaultTemp
- [x] runners 所有 LLM 调用使用 resolveMaxTokens / resolveTemperature
- [x] Agent.MaxTokens 为 0 时回退到 agents.defaults.max_tokens
- [x] 配置值提取为包级常量（defaultMaxTokens / defaultTemp）
- [x] 前端配置值 Number() 类型转换保护

### 15.8 其他 ✅

- [x] Prompt 管理拆分（内置模板→系统配置，自定义版本→Agent 详情页）
- [x] 删除独立 Prompts.vue 页面及菜单
- [x] 所有弹窗禁用点击外部关闭（close-on-click-modal=false）
- [x] User Template 模板变量说明组件（TemplateHelp.vue，三处复用）
- [x] ARCHITECTURE.md 校正 + mermaid 图
- [x] TASKS.md 归档

---

## 后续计划

### Phase 14：沙箱增强（待开发）

详见 [沙箱迭代计划](sandbox-roadmap.md)

- 14.1 临时目录模式
- 14.2 更丰富的上下文工具
- 14.3 配置化的超时和限制
- 14.4 安全增强
- ~~14.5 Agent 迭代控制配置化~~ ✅ 已完成
