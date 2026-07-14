# Web UI 设计方案

> 记录时间：2026-06-03
> 方案：数据库用户表 + JWT 认证

## 一、架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                    Web UI (Vue 3 + Element Plus)             │
│  ┌─────────────────────────────────────────────────────────┐│
│  │  登录页面 → 输入用户名/密码                              ││
│  │  Dashboard → Agent 管理 → Task 列表 → Prompt 编辑        ││
│  │  localStorage: JWT Token                                ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────┬───────────────────────────────────┘
                          │ Authorization: Bearer <JWT>
                          ▼
┌─────────────────────────────────────────────────────────────┐
│                    API Server (Go)                           │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐│
│  │ Auth API    │  │ Agent API   │  │ Task/Prompt API     ││
│  │ /api/login  │  │ /api/agents │  │ /api/tasks          ││
│  │ /api/logout │  │             │  │ /api/prompts        ││
│  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘│
│         │                │                     │           │
│  ┌──────▼────────────────▼─────────────────────▼──────────┐│
│  │              JWT Middleware                              ││
│  │  验证 Token → 提取用户信息 → 注入 Context               ││
│  └──────────────────────┬──────────────────────────────────┘│
│                         │                                   │
│  ┌──────────────────────▼──────────────────────────────────┐│
│  │              SQLite Database                             ││
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌───────────┐ ││
│  │  │ users   │  │ agents  │  │ tasks   │  │ prompts   │ ││
│  │  └─────────┘  └─────────┘  └─────────┘  └───────────┘ ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

---

## 二、数据库设计

```sql
-- 用户表
CREATE TABLE IF NOT EXISTS users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role          TEXT NOT NULL DEFAULT 'user',  -- admin | user | viewer
    display_name  TEXT DEFAULT '',
    email         TEXT DEFAULT '',
    is_active     INTEGER DEFAULT 1,
    last_login    DATETIME,
    created_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- 角色权限
-- admin:  完全权限 (CRUD Agent/Route/Prompt, 管理用户)
-- user:   操作权限 (创建/编辑 Agent, 查看 Task)
-- viewer: 只读权限 (查看 Agent/Task/日志)
```

---

## 三、API 设计

### 3.1 认证 API

```
POST /api/auth/login
  请求: { "username": "admin", "password": "xxx" }
  响应: { "token": "eyJ...", "user": { "id": 1, "username": "admin", "role": "admin" } }

POST /api/auth/logout
  请求: (无，JWT 无状态)
  响应: { "status": "ok" }

GET /api/auth/me
  响应: { "id": 1, "username": "admin", "role": "admin", "display_name": "管理员" }

PUT /api/auth/password
  请求: { "old_password": "xxx", "new_password": "yyy" }
  响应: { "status": "ok" }
```

### 3.2 用户管理 API（仅 admin）

```
GET    /api/users              # 列出所有用户
POST   /api/users              # 创建用户
GET    /api/users/{id}         # 获取用户详情
PUT    /api/users/{id}         # 更新用户
DELETE /api/users/{id}         # 删除用户
```

### 3.3 权限控制

```go
// 权限矩阵
var permissions = map[string][]string{
    // 资源: [允许的角色]
    "agents:read":   {"admin", "user", "viewer"},
    "agents:write":  {"admin", "user"},
    "agents:delete": {"admin"},
    "routes:read":   {"admin", "user", "viewer"},
    "routes:write":  {"admin", "user"},
    "routes:delete": {"admin"},
    "tasks:read":    {"admin", "user", "viewer"},
    "tasks:write":   {"admin"},  // 重试/取消任务
    "prompts:read":  {"admin", "user", "viewer"},
    "prompts:write": {"admin", "user"},
    "users:read":    {"admin"},
    "users:write":   {"admin"},
    "logs:read":     {"admin", "user", "viewer"},
}
```

---

## 四、后端实现

### 4.1 数据库模型

```go
// store/user.go
type User struct {
    ID           int64     `json:"id"`
    Username     string    `json:"username"`
    PasswordHash string    `json:"-"` // 不返回给前端
    Role         string    `json:"role"`
    DisplayName  string    `json:"display_name"`
    Email        string    `json:"email"`
    IsActive     bool      `json:"is_active"`
    LastLogin    *time.Time `json:"last_login"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}

// CRUD
func (db *DB) CreateUser(u *User) error
func (db *DB) GetUser(id int64) (*User, error)
func (db *DB) GetUserByUsername(username string) (*User, error)
func (db *DB) ListUsers() ([]*User, error)
func (db *DB) UpdateUser(u *User) error
func (db *DB) DeleteUser(id int64) error
func (db *DB) UpdateLastLogin(id int64) error
```

### 4.2 JWT 认证

```go
// auth/jwt.go
type JWTManager struct {
    secret     []byte
    expiration time.Duration
}

type Claims struct {
    UserID   int64  `json:"user_id"`
    Username string `json:"username"`
    Role     string `json:"role"`
    jwt.RegisteredClaims
}

func (m *JWTManager) GenerateToken(user *store.User) (string, error)
func (m *JWTManager) ValidateToken(tokenStr string) (*Claims, error)
```

### 4.3 认证中间件

```go
// api/auth.go
type AuthMiddleware struct {
    jwtManager *auth.JWTManager
    db         *store.DB
}

// 认证中间件
func (m *AuthMiddleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc

// 角色中间件
func (m *AuthMiddleware) RequireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc

// 使用示例
mux.HandleFunc("GET /api/agents",
    auth.RequireAuth(
        auth.RequireRole("admin", "user", "viewer")(h.listAgents)))
```

### 4.4 登录 API

```go
// api/auth_handler.go
func (h *AuthHandler) login(w http.ResponseWriter, r *http.Request) {
    var req struct {
        Username string `json:"username"`
        Password string `json:"password"`
    }
    json.NewDecoder(r.Body).Decode(&req)

    // 1. 查找用户
    user, err := h.db.GetUserByUsername(req.Username)
    if err != nil {
        writeError(w, 401, "invalid credentials")
        return
    }

    // 2. 验证密码
    if !auth.CheckPassword(req.Password, user.PasswordHash) {
        writeError(w, 401, "invalid credentials")
        return
    }

    // 3. 检查用户状态
    if !user.IsActive {
        writeError(w, 403, "account disabled")
        return
    }

    // 4. 生成 JWT
    token, err := h.jwtManager.GenerateToken(user)
    if err != nil {
        writeError(w, 500, "failed to generate token")
        return
    }

    // 5. 更新最后登录时间
    h.db.UpdateLastLogin(user.ID)

    // 6. 返回
    writeJSON(w, 200, map[string]interface{}{
        "token": token,
        "user":  user,
    })
}
```

---

## 五、前端设计

### 5.1 页面结构

```
src/
├── views/
│   ├── Login.vue           # 登录页面
│   ├── Dashboard.vue       # 仪表盘
│   ├── Agents.vue          # Agent 列表
│   ├── AgentDetail.vue     # Agent 详情/编辑
│   ├── Tasks.vue           # 任务列表
│   ├── TaskDetail.vue      # 任务详情
│   ├── Prompts.vue         # Prompt 管理
│   ├── Routes.vue          # 路由规则
│   ├── Users.vue           # 用户管理 (admin)
│   └── Settings.vue        # 系统设置
├── components/
│   ├── Layout.vue          # 布局组件
│   ├── Sidebar.vue         # 侧边栏
│   └── ...
├── stores/
│   ├── auth.js             # 认证状态
│   └── ...
├── api/
│   ├── index.js            # API 客户端
│   └── ...
└── router/
    └── index.js            # 路由配置
```

### 5.2 认证流程

```javascript
// stores/auth.js
export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || null,
    user: JSON.parse(localStorage.getItem('user') || 'null'),
  }),

  actions: {
    async login(username, password) {
      const { token, user } = await api.post('/auth/login', { username, password })
      this.token = token
      this.user = user
      localStorage.setItem('token', token)
      localStorage.setItem('user', JSON.stringify(user))
    },

    logout() {
      this.token = null
      this.user = null
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    },

    isAuthenticated() {
      return !!this.token
    },

    hasRole(role) {
      return this.user?.role === role
    }
  }
})

// api/index.js
const api = axios.create({ baseURL: '/api' })

api.interceptors.request.use(config => {
  const authStore = useAuthStore()
  if (authStore.token) {
    config.headers.Authorization = `Bearer ${authStore.token}`
  }
  return config
})

api.interceptors.response.use(
  response => response.data,
  error => {
    if (error.response?.status === 401) {
      const authStore = useAuthStore()
      authStore.logout()
      router.push('/login')
    }
    return Promise.reject(error)
  }
)
```

### 5.3 路由守卫

```javascript
// router/index.js
const routes = [
  { path: '/login', component: Login, meta: { guest: true } },
  { path: '/', component: Dashboard, meta: { requiresAuth: true } },
  { path: '/agents', component: Agents, meta: { requiresAuth: true } },
  { path: '/tasks', component: Tasks, meta: { requiresAuth: true } },
  { path: '/users', component: Users, meta: { requiresAuth: true, roles: ['admin'] } },
  // ...
]

router.beforeEach((to, from, next) => {
  const authStore = useAuthStore()

  if (to.meta.requiresAuth && !authStore.isAuthenticated()) {
    next('/login')
  } else if (to.meta.roles && !to.meta.roles.includes(authStore.user?.role)) {
    next('/') // 无权限，跳转首页
  } else {
    next()
  }
})
```

---

## 六、初始化流程

```go
// main.go 启动时
func main() {
    // ...

    // 初始化数据库（包含 users 表）
    db, _ := store.Open(cfg.Database.Path)

    // 创建默认管理员（如果不存在）
    ensureDefaultAdmin(db, cfg)

    // ...
}

func ensureDefaultAdmin(db *store.DB, cfg *config.Config) {
    _, err := db.GetUserByUsername("admin")
    if err == nil {
        return // 已存在
    }

    // 从配置读取默认密码
    password := cfg.Auth.DefaultAdminPassword
    if password == "" {
        password = "admin123" // 默认密码
    }

    hash, _ := auth.HashPassword(password)
    db.CreateUser(&store.User{
        Username:     "admin",
        PasswordHash: hash,
        Role:         "admin",
        DisplayName:  "Administrator",
        IsActive:     true,
    })

    log.Printf("[INFO] Default admin user created (username: admin)")
}
```

---

## 七、配置文件

```yaml
# config.yaml
auth:
  # JWT 密钥 (生产环境请修改)
  jwt_secret: "${JWT_SECRET:-your-secret-key-change-in-production}"
  # JWT 过期时间
  jwt_expiration: "24h"
  # 默认管理员密码 (首次启动时创建)
  default_admin_password: "${ADMIN_PASSWORD:-admin123}"

server:
  host: "0.0.0.0"
  port: 8080
  # 静态文件目录 (Web UI)
  static_dir: "./web/dist"
```

---

## 八、实现优先级

### Phase 1：核心认证

- [ ] users 表 + CRUD
- [ ] JWT 认证 (auth/jwt.go)
- [ ] 密码哈希 (bcrypt)
- [ ] 登录/登出 API
- [ ] 认证中间件
- [ ] 前端登录页面

### Phase 2：权限控制

- [ ] 角色定义 (admin/user/viewer)
- [ ] 权限中间件
- [ ] 前端权限控制 (路由守卫)

### Phase 3：用户管理

- [ ] 用户管理页面 (admin)
- [ ] 密码修改
- [ ] 登录日志

---

## 九、安全考虑

| 措施 | 说明 |
|------|------|
| **密码哈希** | bcrypt 或 argon2 |
| **JWT 过期** | 24 小时，可配置 |
| **HTTPS** | 生产环境必须 |
| **CORS** | 限制允许的域名 |
| **Rate Limiting** | 防止暴力破解 |
| **输入验证** | 防止 SQL 注入和 XSS |

---

## 十、技术选型

| 维度 | 选型 |
|------|------|
| **前端框架** | Vue 3 |
| **UI 组件库** | Element Plus |
| **状态管理** | Pinia |
| **HTTP 客户端** | Axios |
| **路由** | Vue Router |
| **构建工具** | Vite |
| **认证方式** | JWT (无状态) |
| **密码存储** | bcrypt 哈希 |

---

## 十一、工作量估算

| 阶段 | 任务 | 工作量 |
|------|------|--------|
| Phase 1 | 核心认证 | 1 周 |
| Phase 2 | 权限控制 | 3 天 |
| Phase 3 | 用户管理 | 2 天 |
| 前端 | 完整 Web UI | 2 周 |
| **总计** | | **约 3 周** |
