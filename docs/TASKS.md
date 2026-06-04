# 任务执行文档

> 当前阶段：Web UI 优化完善

---

## Phase 1-13：已完成 ✅

详见 [归档文档](20260604-TASKS.md)。

## Phase 14：沙箱增强

详见 [沙箱迭代计划](sandbox-roadmap.md)。

## Phase 15：Web UI 优化（当前）

### 15.1 Bug 修复 ✅

- [x] 15.1.1 用户管理页面返回 HTML 问题（添加 /api/users 端点）
- [x] 15.1.2 内置模板为空（/api/templates 返回 PromptManager 内置模板）
- [x] 15.1.3 Prompt 版本记录（Agent 编辑时自动创建 prompt_history）
- [x] 15.1.4 查看模板按钮无响应（添加对话框）

### 15.2 系统配置页面 ✅

- [x] 15.2.1 system_config 表 + store CRUD
- [x] 15.2.2 ConfigManager（DB 配置 > 文件配置 > 默认值）
- [x] 15.2.3 GET/PUT/DELETE /api/config 端点
- [x] 15.2.4 LLM Registry 热更新
- [x] 15.2.5 SystemConfig.vue 页面（Gitea/LLM/Dispatcher/Agent 默认值）
- [x] 15.2.6 菜单顺序调整 + 图标

### 15.3 Web UI 体验优化

#### A. Agent 创建增强
- [ ] A1 表单添加 Agent Loop 配置（max_iterations, max_tokens, timeout, total_timeout）
- [ ] A2 表单添加模板选择下拉框，选中后自动填充 System Prompt + User Template
- [ ] A3 表单分组：基本信息 / Gitea / LLM / Loop / Prompt

#### B. Agent 详情页
- [ ] B1 新建 AgentDetail.vue，路由 /agents/:id
- [ ] B2 Tab 1：基本信息（编辑 Agent 配置）
- [ ] B3 Tab 2：触发规则（Route CRUD，内嵌在 Agent 页）
- [ ] B4 Tab 3：Prompt 版本历史
- [ ] B5 Tab 4：最近任务（该 Agent 的任务列表）
- [ ] B6 Agent 列表页名称可点击跳转详情

#### C. 触发规则管理（Route CRUD in Agent Detail）
- [ ] C1 后端 GET /api/agents/:id/routes
- [ ] C2 前端触发规则 Tab — 表格展示 + 添加/删除
- [ ] C3 添加规则表单：事件类型 + 动作 + Label + Assignee + Mention
- [ ] C4 预设快捷配置：一键添加常用规则

#### D. 新用户引导
- [ ] D1 Dashboard 检测是否有 Agent，无则显示引导卡片
- [ ] D2 引导卡片：配置 Gitea → 创建 Agent → 配置规则

#### E. Sandbox 配置（per Agent）
- [ ] E1 Agent 表添加 sandbox_config 字段（JSON）
- [ ] E2 Agent 表单添加 Sandbox 配置折叠面板
- [ ] E3 后端读取 Agent 级别 sandbox_config

#### F. 交互细节优化
- [ ] F1 Agent 列表显示触发规则数量标签
- [ ] F2 操作确认和成功反馈统一
- [ ] F3 加载状态统一
