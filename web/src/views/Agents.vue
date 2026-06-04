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

      <el-table :data="paginatedAgents" style="width: 100%">
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
        <el-table-column label="操作" width="250">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click="router.push(`/agents/${row.id}`)">详情</el-button>
            <el-button size="small" @click="editAgent(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteAgent(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pagination-bar">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :total="agents.length"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          small
        />
      </div>
    </el-card>

    <!-- Create/Edit Dialog -->
    <el-dialog v-model="showCreateDialog" :title="editingAgent ? '编辑 Agent' : '创建 Agent'" width="700px" top="5vh"
      :close-on-click-modal="false" :close-on-press-escape="false">
      <el-form :model="form" label-width="120px">
        <!-- 基本信息 -->
        <el-form-item label="名称">
          <el-input v-model="form.name" placeholder="如 code-reviewer" />
        </el-form-item>
        <el-form-item label="Gitea 用户名">
          <el-input v-model="form.gitea_username" :disabled="!!editingAgent" placeholder="自动创建 Gitea 账号" />
          <div v-if="editingAgent" class="form-tip">创建后不可修改</div>
        </el-form-item>
        <el-form-item label="Provider">
          <el-col :span="11">
            <el-select v-model="form.provider" placeholder="选择 Provider" style="width: 100%">
              <el-option v-for="(_, name) in providers" :key="name" :label="name" :value="name" />
            </el-select>
          </el-col>
          <el-col :span="2" style="text-align: center; line-height: 32px">:</el-col>
          <el-col :span="11">
            <el-input v-model="form.model" placeholder="模型名称" />
          </el-col>
        </el-form-item>

        <!-- Prompt -->
        <el-form-item label="从模板导入">
          <el-select v-model="selectedTemplate" placeholder="选择内置模板快速填充" @change="applyTemplate" clearable style="width: 100%">
            <el-option v-for="tmpl in builtinTemplates" :key="tmpl.name" :label="tmpl.name" :value="tmpl.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="System Prompt">
          <el-input v-model="form.system_prompt" type="textarea" :rows="5" placeholder="Agent 的系统提示词" />
        </el-form-item>

        <!-- 折叠：高级配置 -->
        <el-collapse v-model="advancedOpen">
          <el-collapse-item title="高级配置" name="advanced">
            <el-form-item label="状态">
              <el-select v-model="form.status">
                <el-option label="活跃" value="active" />
                <el-option label="禁用" value="inactive" />
              </el-select>
            </el-form-item>
            <el-form-item label="Max Tokens">
              <el-input-number v-model="form.max_tokens" :min="256" :max="128000" :step="512" />
            </el-form-item>
            <el-form-item label="Temperature">
              <el-slider v-model="form.temperature" :min="0" :max="2" :step="0.1" show-input style="width: 100%" />
            </el-form-item>
            <el-form-item label="User Template">
              <el-input v-model="form.user_template" type="textarea" :rows="3" placeholder="用户消息模板（可选）" />
              <el-button type="primary" link size="small" style="margin-top: 4px" @click="$refs.templateHelp.show()">查看模板变量说明</el-button>
            </el-form-item>
          </el-collapse-item>

          <el-collapse-item title="Agent Loop 配置" name="loop">
            <el-form-item label="最大迭代轮数">
              <el-input-number v-model="form.loop_config.max_iterations" :min="1" :max="100" :step="1" />
              <span class="form-tip" style="margin-left: 12px">默认 20</span>
            </el-form-item>
            <el-form-item label="最大 Tokens">
              <el-input-number v-model="form.loop_config.max_tokens" :min="1024" :max="32768" :step="1024" />
              <span class="form-tip" style="margin-left: 12px">默认 4096</span>
            </el-form-item>
            <el-form-item label="单轮超时">
              <el-input v-model="form.loop_config.timeout" placeholder="5m" style="width: 200px" />
            </el-form-item>
            <el-form-item label="总超时">
              <el-input v-model="form.loop_config.total_timeout" placeholder="30m" style="width: 200px" />
            </el-form-item>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button @click="closeDialog">取消</el-button>
        <el-button type="primary" @click="saveAgent">保存</el-button>
      </template>
    </el-dialog>
    <TemplateHelp ref="templateHelp" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import TemplateHelp from '../components/TemplateHelp.vue'

const router = useRouter()
const agents = ref([])
const agentRouteCounts = ref({})
const currentPage = ref(1)
const pageSize = ref(20)

const paginatedAgents = computed(() => {
  const start = (currentPage.value - 1) * pageSize.value
  return agents.value.slice(start, start + pageSize.value)
})
const showCreateDialog = ref(false)
const editingAgent = ref(null)
const builtinTemplates = ref([])
const selectedTemplate = ref('')
const advancedOpen = ref([])
const providers = ref({})

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
    const data = await api.get('/prompt-templates')
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

const loadProviders = async () => {
  try {
    const data = await api.get('/config')
    if (data && data['llm.providers']) {
      providers.value = data['llm.providers']
    }
  } catch {
    providers.value = {}
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
  loadProviders()
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

.pagination-bar {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
