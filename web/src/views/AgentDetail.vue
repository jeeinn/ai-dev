<template>
  <div class="agent-detail-page">
    <el-page-header @back="router.push('/agents')" :title="'返回 Agent 列表'">
      <template #content>
        <span class="page-title">{{ agent?.name || '加载中...' }}</span>
        <el-tag v-if="agent?.status === 'active'" type="success" size="small" style="margin-left: 12px">活跃</el-tag>
        <el-tag v-else type="info" size="small" style="margin-left: 12px">禁用</el-tag>
      </template>
    </el-page-header>

    <el-tabs v-if="agent" v-model="activeTab" style="margin-top: 20px">
      <!-- Tab 1: 基本信息 -->
      <el-tab-pane label="基本信息" name="info">
        <el-card>
          <el-form :model="form" label-width="140px" style="max-width: 700px">
            <el-divider content-position="left">基本信息</el-divider>
            <el-form-item label="名称">
              <el-input v-model="form.name" />
            </el-form-item>
            <el-form-item label="状态">
              <el-select v-model="form.status">
                <el-option label="活跃" value="active" />
                <el-option label="禁用" value="inactive" />
              </el-select>
            </el-form-item>
            <el-form-item label="Gitea 用户">
              <el-input :model-value="form.gitea_username" disabled />
            </el-form-item>

            <el-divider content-position="left">LLM 配置</el-divider>
            <el-form-item label="Provider">
              <el-select v-model="form.provider" style="width: 100%">
                <el-option label="DeepSeek" value="deepseek" />
                <el-option label="OpenAI" value="openai" />
                <el-option label="Anthropic" value="anthropic" />
              </el-select>
            </el-form-item>
            <el-form-item label="模型">
              <el-input v-model="form.model" />
            </el-form-item>
            <el-form-item label="Max Tokens">
              <el-input-number v-model="form.max_tokens" :min="256" :max="128000" :step="512" />
            </el-form-item>
            <el-form-item label="Temperature">
              <el-slider v-model="form.temperature" :min="0" :max="2" :step="0.1" show-input style="width: 100%" />
            </el-form-item>

            <el-divider content-position="left">Agent Loop</el-divider>
            <el-form-item label="最大迭代轮数">
              <el-input-number v-model="form.loop_config.max_iterations" :min="1" :max="100" />
            </el-form-item>
            <el-form-item label="最大 Tokens">
              <el-input-number v-model="form.loop_config.max_tokens" :min="1024" :max="32768" :step="1024" />
            </el-form-item>
            <el-form-item label="单轮超时">
              <el-input v-model="form.loop_config.timeout" placeholder="5m" />
            </el-form-item>
            <el-form-item label="总超时">
              <el-input v-model="form.loop_config.total_timeout" placeholder="30m" />
            </el-form-item>

            <el-divider content-position="left">Prompt</el-divider>
            <el-form-item label="System Prompt">
              <el-input v-model="form.system_prompt" type="textarea" :rows="6" />
            </el-form-item>
            <el-form-item label="User Template">
              <el-input v-model="form.user_template" type="textarea" :rows="4" />
            </el-form-item>

            <el-form-item>
              <el-button type="primary" :loading="saving" @click="saveAgent">保存修改</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-tab-pane>

      <!-- Tab 2: 触发规则 -->
      <el-tab-pane label="触发规则" name="routes">
        <el-card>
          <template #header>
            <div class="card-header">
              <span>触发规则 <el-tag size="small" style="margin-left: 8px">{{ routes.length }} 条</el-tag></span>
              <el-button type="primary" size="small" @click="showAddRoute = true">
                <el-icon><Plus /></el-icon> 添加规则
              </el-button>
            </div>
          </template>

          <el-empty v-if="!routes.length" description="暂无触发规则，点击上方按钮添加" />
          <el-table v-else :data="routes" style="width: 100%">
            <el-table-column prop="event" label="事件" width="120" />
            <el-table-column prop="action" label="动作" width="120">
              <template #default="{ row }">{{ row.action || '-' }}</template>
            </el-table-column>
            <el-table-column prop="label" label="Label" width="150">
              <template #default="{ row }">
                <el-tag v-if="row.label" size="small">{{ row.label }}</el-tag>
                <span v-else>-</span>
              </template>
            </el-table-column>
            <el-table-column prop="assignee" label="Assignee" width="120">
              <template #default="{ row }">{{ row.assignee || '-' }}</template>
            </el-table-column>
            <el-table-column prop="mention" label="Mention" width="120">
              <template #default="{ row }">{{ row.mention || '-' }}</template>
            </el-table-column>
            <el-table-column prop="priority" label="优先级" width="80" />
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button size="small" type="danger" @click="deleteRoute(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>

          <!-- 快捷规则 -->
          <el-divider content-position="left">快捷配置</el-divider>
          <el-space wrap>
            <el-button size="small" @click="quickAddRoute('issues', 'labeled', 'ai:analyze')">Issue + ai:analyze</el-button>
            <el-button size="small" @click="quickAddRoute('issues', 'labeled', 'ai:solve')">Issue + ai:solve</el-button>
            <el-button size="small" @click="quickAddRoute('issues', 'labeled', 'ai:fix')">Issue + ai:fix</el-button>
            <el-button size="small" @click="quickAddRoute('pull_request', 'labeled', 'ai:review')">PR + ai:review</el-button>
            <el-button size="small" @click="quickAddRoute('issue_comment', '', '', '', agent?.gitea_username)">@mention 回复</el-button>
          </el-space>
        </el-card>
      </el-tab-pane>

      <!-- Tab 3: Prompt 版本 -->
      <el-tab-pane label="Prompt 版本" name="prompts">
        <el-card>
          <el-empty v-if="!prompts.length" description="暂无 Prompt 版本记录" />
          <el-table v-else :data="prompts" style="width: 100%">
            <el-table-column prop="version" label="版本" width="80" />
            <el-table-column prop="note" label="备注" />
            <el-table-column prop="is_active" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.is_active ? 'success' : 'info'" size="small">{{ row.is_active ? '活跃' : '历史' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="创建时间" width="180" />
            <el-table-column label="操作" width="150">
              <template #default="{ row }">
                <el-button v-if="!row.is_active" size="small" @click="rollbackPrompt(row)">回滚</el-button>
                <el-button size="small" type="danger" @click="deletePrompt(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-tab-pane>

      <!-- Tab 4: 最近任务 -->
      <el-tab-pane label="最近任务" name="tasks">
        <el-card>
          <el-empty v-if="!tasks.length" description="暂无任务记录" />
          <el-table v-else :data="tasks" style="width: 100%">
            <el-table-column prop="id" label="ID" width="60" />
            <el-table-column prop="task_type" label="类型" width="120" />
            <el-table-column prop="repo" label="仓库" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="statusType(row.status)" size="small">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="创建时间" width="180" />
          </el-table>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- 添加规则对话框 -->
    <el-dialog v-model="showAddRoute" title="添加触发规则" width="500px">
      <el-form :model="routeForm" label-width="100px">
        <el-form-item label="事件类型">
          <el-select v-model="routeForm.event" style="width: 100%">
            <el-option label="Issues" value="issues" />
            <el-option label="Pull Request" value="pull_request" />
            <el-option label="Issue Comment" value="issue_comment" />
            <el-option label="Push" value="push" />
          </el-select>
        </el-form-item>
        <el-form-item label="动作">
          <el-select v-model="routeForm.action" clearable style="width: 100%">
            <el-option label="(任意)" value="" />
            <el-option label="assigned" value="assigned" />
            <el-option label="labeled" value="labeled" />
            <el-option label="opened" value="opened" />
            <el-option label="created" value="created" />
          </el-select>
        </el-form-item>
        <el-form-item label="Label">
          <el-input v-model="routeForm.label" placeholder="如 ai:analyze" />
        </el-form-item>
        <el-form-item label="Assignee">
          <el-input v-model="routeForm.assignee" placeholder="指定分配人" />
        </el-form-item>
        <el-form-item label="Mention">
          <el-input v-model="routeForm.mention" placeholder="@用户名" />
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="routeForm.priority" :min="0" :max="100" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddRoute = false">取消</el-button>
        <el-button type="primary" @click="addRoute">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const route = useRoute()
const router = useRouter()
const agentId = ref(route.params.id)

const activeTab = ref('info')
const agent = ref(null)
const routes = ref([])
const prompts = ref([])
const tasks = ref([])
const saving = ref(false)
const showAddRoute = ref(false)

const routeForm = ref({
  event: 'issues',
  action: 'labeled',
  label: '',
  assignee: '',
  mention: '',
  priority: 0
})

const defaultLoopConfig = {
  max_iterations: 20,
  max_tokens: 4096,
  timeout: '5m',
  total_timeout: '30m'
}

const defaultForm = {
  name: '',
  gitea_username: '',
  provider: 'deepseek',
  model: 'deepseek-chat',
  max_tokens: 4096,
  temperature: 0.3,
  system_prompt: '',
  user_template: '',
  status: 'active',
  loop_config: { ...defaultLoopConfig }
}

const form = ref({ ...defaultForm, loop_config: { ...defaultLoopConfig } })

const statusType = (status) => {
  const types = { pending: 'warning', running: 'primary', success: 'success', failed: 'danger' }
  return types[status] || 'info'
}

const loadAgent = async () => {
  try {
    const data = await api.get(`/agents/${agentId.value}`)
    agent.value = data
    form.value = {
      ...defaultForm,
      ...data,
      loop_config: { ...defaultLoopConfig, ...(data.loop_config || {}) }
    }
  } catch (error) {
    ElMessage.error('加载 Agent 信息失败')
    router.push('/agents')
  }
}

const loadRoutes = async () => {
  try {
    const data = await api.get(`/agents/${agentId.value}/routes`)
    routes.value = Array.isArray(data) ? data : []
  } catch {
    routes.value = []
  }
}

const loadPrompts = async () => {
  try {
    const data = await api.get(`/agents/${agentId.value}/prompts`)
    prompts.value = Array.isArray(data) ? data : []
  } catch {
    prompts.value = []
  }
}

const loadTasks = async () => {
  try {
    const data = await api.get(`/agents/${agentId.value}/tasks`)
    tasks.value = Array.isArray(data) ? data : []
  } catch {
    tasks.value = []
  }
}

const saveAgent = async () => {
  saving.value = true
  try {
    await api.put(`/agents/${agentId.value}`, form.value)
    ElMessage.success('保存成功')
    await loadAgent()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const addRoute = async () => {
  try {
    await api.post('/routes', {
      ...routeForm.value,
      agent_id: parseInt(agentId.value)
    })
    ElMessage.success('规则添加成功')
    showAddRoute.value = false
    routeForm.value = { event: 'issues', action: 'labeled', label: '', assignee: '', mention: '', priority: 0 }
    await loadRoutes()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '添加失败')
  }
}

const quickAddRoute = async (event, action, label, assignee, mention) => {
  try {
    await api.post('/routes', {
      event,
      action,
      label: label || '',
      assignee: assignee || '',
      mention: mention || '',
      agent_id: parseInt(agentId.value),
      priority: 0
    })
    ElMessage.success('规则添加成功')
    await loadRoutes()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '添加失败')
  }
}

const deleteRoute = async (r) => {
  try {
    await ElMessageBox.confirm('确定删除这条规则？', '确认')
    await api.delete(`/routes/${r.id}`)
    ElMessage.success('删除成功')
    await loadRoutes()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error('删除失败')
  }
}

const rollbackPrompt = async (prompt) => {
  try {
    await api.post(`/prompts/${prompt.id}/activate`)
    ElMessage.success('回滚成功')
    await loadPrompts()
  } catch {
    ElMessage.error('回滚失败')
  }
}

const deletePrompt = async (prompt) => {
  try {
    await api.delete(`/prompts/${prompt.id}`)
    ElMessage.success('删除成功')
    await loadPrompts()
  } catch {
    ElMessage.error('删除失败')
  }
}

// Reload data when tab changes
watch(activeTab, (tab) => {
  if (tab === 'routes') loadRoutes()
  else if (tab === 'prompts') loadPrompts()
  else if (tab === 'tasks') loadTasks()
})

onMounted(loadAgent)
</script>

<style scoped>
.page-title {
  font-size: 18px;
  font-weight: 600;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
