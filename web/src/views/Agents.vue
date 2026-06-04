<template>
  <div class="agents-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>Agent 管理</span>
          <el-button type="primary" @click="openCreateDialog">
            <el-icon><Plus /></el-icon>
            创建 Agent
          </el-button>
        </div>
      </template>

      <el-table :data="agents" style="width: 100%">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="名称">
          <template #default="{ row }">
            <el-link type="primary" @click="router.push(`/agents/${row.id}`)">{{ row.name }}</el-link>
          </template>
        </el-table-column>
        <el-table-column prop="gitea_username" label="Gitea 用户" />
        <el-table-column prop="provider" label="Provider" width="100" />
        <el-table-column prop="model" label="模型" />
        <el-table-column label="触发规则" width="90">
          <template #default="{ row }">
            <el-tag v-if="agentRouteCounts[row.id] > 0" size="small">{{ agentRouteCounts[row.id] }} 条</el-tag>
            <span v-else class="text-muted">未配置</span>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="80">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'" size="small">{{ row.status === 'active' ? '活跃' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" @click="editAgent(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteAgent(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Create/Edit Dialog -->
    <el-dialog v-model="showCreateDialog" :title="editingAgent ? '编辑 Agent' : '创建 Agent'" width="700px" top="5vh">
      <el-form :model="form" label-width="120px">
        <!-- 基本信息 -->
        <el-divider content-position="left">基本信息</el-divider>
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="如 code-reviewer" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="form.status">
            <el-option label="活跃" value="active" />
            <el-option label="禁用" value="inactive" />
          </el-select>
        </el-form-item>

        <!-- Gitea 配置 -->
        <el-divider content-position="left">Gitea 配置</el-divider>
        <el-form-item label="Gitea 用户名">
          <el-input v-model="form.gitea_username" :disabled="!!editingAgent" placeholder="自动创建 Gitea 账号" />
          <div v-if="editingAgent" class="form-tip">Gitea 用户名创建后不可修改</div>
          <div v-else class="form-tip">系统将自动在 Gitea 创建此用户并生成 Token</div>
        </el-form-item>

        <!-- LLM 配置 -->
        <el-divider content-position="left">LLM 配置</el-divider>
        <el-form-item label="Provider">
          <el-select v-model="form.provider" style="width: 100%">
            <el-option label="DeepSeek" value="deepseek" />
            <el-option label="OpenAI" value="openai" />
            <el-option label="Anthropic (Claude)" value="anthropic" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="form.model" placeholder="deepseek-chat" />
        </el-form-item>
        <el-form-item label="Max Tokens">
          <el-input-number v-model="form.max_tokens" :min="256" :max="128000" :step="512" />
        </el-form-item>
        <el-form-item label="Temperature">
          <el-slider v-model="form.temperature" :min="0" :max="2" :step="0.1" show-input style="width: 100%" />
        </el-form-item>

        <!-- Agent Loop 配置 -->
        <el-divider content-position="left">Agent Loop 配置</el-divider>
        <el-form-item label="最大迭代轮数">
          <el-input-number v-model="form.loop_config.max_iterations" :min="1" :max="100" :step="1" />
          <div class="form-tip">Agent 最大对话轮数 (默认 20)</div>
        </el-form-item>
        <el-form-item label="最大 Tokens">
          <el-input-number v-model="form.loop_config.max_tokens" :min="1024" :max="32768" :step="1024" />
          <div class="form-tip">单次 LLM 调用最大 Tokens (默认 4096)</div>
        </el-form-item>
        <el-form-item label="单轮超时">
          <el-input v-model="form.loop_config.timeout" placeholder="5m" />
          <div class="form-tip">单轮 LLM 调用超时 (默认 5m)</div>
        </el-form-item>
        <el-form-item label="总超时">
          <el-input v-model="form.loop_config.total_timeout" placeholder="30m" />
          <div class="form-tip">整个任务超时 (默认 30m)</div>
        </el-form-item>

        <!-- Prompt 配置 -->
        <el-divider content-position="left">Prompt 配置</el-divider>
        <el-form-item label="从模板导入">
          <el-select v-model="selectedTemplate" placeholder="选择内置模板" @change="applyTemplate" clearable style="width: 100%">
            <el-option v-for="tmpl in builtinTemplates" :key="tmpl.name" :label="tmpl.name" :value="tmpl.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="System Prompt">
          <el-input v-model="form.system_prompt" type="textarea" :rows="6" placeholder="Agent 的系统提示词" />
        </el-form-item>
        <el-form-item label="User Template">
          <el-input v-model="form.user_template" type="textarea" :rows="4" placeholder="用户消息模板（可选，支持 Go template 语法）" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="closeDialog">取消</el-button>
        <el-button type="primary" @click="saveAgent">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const router = useRouter()
const agents = ref([])
const agentRouteCounts = ref({})
const showCreateDialog = ref(false)
const editingAgent = ref(null)
const builtinTemplates = ref([])
const selectedTemplate = ref('')

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
  loop_config: {
    max_iterations: 20,
    max_tokens: 4096,
    timeout: '5m',
    total_timeout: '30m'
  }
}

const form = ref({ ...defaultForm })

const loadAgents = async () => {
  agents.value = await api.get('/agents')
  // Load route counts for each agent
  try {
    const routes = await api.get('/routes')
    const counts = {}
    for (const route of routes) {
      counts[route.agent_id] = (counts[route.agent_id] || 0) + 1
    }
    agentRouteCounts.value = counts
  } catch {
    // ignore
  }
}

const loadTemplates = async () => {
  try {
    const data = await api.get('/templates')
    if (data && typeof data === 'object') {
      builtinTemplates.value = Object.entries(data).map(([key, value]) => ({
        name: key,
        ...value
      }))
    }
  } catch {
    builtinTemplates.value = []
  }
}

const applyTemplate = (name) => {
  if (!name) return
  const tmpl = builtinTemplates.value.find(t => t.name === name)
  if (tmpl) {
    form.value.system_prompt = tmpl.system_prompt || ''
    form.value.user_template = tmpl.user_template || ''
    ElMessage.success(`已应用模板：${name}`)
  }
}

const openCreateDialog = () => {
  editingAgent.value = null
  form.value = { ...defaultForm, loop_config: { ...defaultForm.loop_config } }
  selectedTemplate.value = ''
  showCreateDialog.value = true
}

const editAgent = (agent) => {
  editingAgent.value = agent
  form.value = {
    ...agent,
    loop_config: { ...defaultForm.loop_config, ...(agent.loop_config || {}) }
  }
  selectedTemplate.value = ''
  showCreateDialog.value = true
}

const closeDialog = () => {
  showCreateDialog.value = false
  editingAgent.value = null
  form.value = { ...defaultForm }
}

const saveAgent = async () => {
  try {
    if (editingAgent.value) {
      await api.put(`/agents/${editingAgent.value.id}`, form.value)
      ElMessage.success('更新成功')
    } else {
      await api.post('/agents', form.value)
      ElMessage.success('创建成功')
    }
    closeDialog()
    loadAgents()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '操作失败')
  }
}

const deleteAgent = async (agent) => {
  try {
    await ElMessageBox.confirm('确定要删除这个 Agent 吗？', '确认')
    await api.delete(`/agents/${agent.id}`)
    ElMessage.success('删除成功')
    loadAgents()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

onMounted(() => {
  loadAgents()
  loadTemplates()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.text-muted {
  font-size: 12px;
  color: #c0c4cc;
}
</style>
