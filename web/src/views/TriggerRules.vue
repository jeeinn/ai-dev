<template>
  <div class="trigger-rules-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>触发规则管理</span>
          <el-button type="primary" @click="showAddDialog = true">
            <el-icon><Plus /></el-icon> 添加规则
          </el-button>
        </div>
      </template>

      <!-- 筛选栏 -->
      <div class="filter-bar">
        <el-select v-model="filterEvent" placeholder="事件类型" clearable style="width: 160px" @change="loadRoutes">
          <el-option label="issues" value="issues" />
          <el-option label="pull_request" value="pull_request" />
          <el-option label="issue_comment" value="issue_comment" />
          <el-option label="push" value="push" />
        </el-select>
        <el-select v-model="filterAction" placeholder="动作" clearable style="width: 140px" @change="loadRoutes">
          <el-option label="assigned" value="assigned" />
          <el-option label="labeled" value="labeled" />
          <el-option label="opened" value="opened" />
          <el-option label="created" value="created" />
        </el-select>
        <el-select v-model="filterAgent" placeholder="Agent" clearable style="width: 180px" @change="loadRoutes">
          <el-option v-for="a in agents" :key="a.id" :label="a.name" :value="a.id" />
        </el-select>
        <el-input v-model="filterLabel" placeholder="Label 关键字" clearable style="width: 160px" @input="loadRoutes" />
        <span class="filter-count">共 {{ routes.length }} 条规则</span>
      </div>

      <!-- 规则表格 -->
      <el-table :data="routes" style="width: 100%">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="event" label="事件" width="130" />
        <el-table-column label="动作" width="100">
          <template #default="{ row }">{{ row.action || '*' }}</template>
        </el-table-column>
        <el-table-column label="Label" width="140">
          <template #default="{ row }">
            <el-tag v-if="row.label" size="small">{{ row.label }}</el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="Agent" width="150">
          <template #default="{ row }">
            <el-link type="primary" @click="router.push(`/agents/${row.agent_id}`)">{{ agentMap[row.agent_id] || row.agent_id }}</el-link>
          </template>
        </el-table-column>
        <el-table-column label="预计执行行为" min-width="200">
          <template #default="{ row }">
            <el-tag :type="getBehaviorTag(row).type" size="small">{{ getBehaviorTag(row).icon }}</el-tag>
            <span style="margin-left: 6px">{{ getBehaviorTag(row).text }}</span>
          </template>
        </el-table-column>
        <el-table-column label="Mention" width="120">
          <template #default="{ row }">{{ row.mention || '-' }}</template>
        </el-table-column>
        <el-table-column label="优先级" width="80">
          <template #default="{ row }">
            <el-tag v-if="row.priority > 0" size="small" type="warning">{{ row.priority }}</el-tag>
            <span v-else class="text-muted">{{ row.priority }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button size="small" type="danger" link @click="deleteRoute(row)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 添加规则对话框 -->
    <el-dialog v-model="showAddDialog" title="添加触发规则" width="550px" :close-on-click-modal="false">
      <el-form :model="form" label-width="100px">
        <el-form-item label="事件类型">
          <el-select v-model="form.event" style="width: 100%">
            <el-option label="Issues" value="issues" />
            <el-option label="Pull Request" value="pull_request" />
            <el-option label="Issue Comment" value="issue_comment" />
            <el-option label="Push" value="push" />
          </el-select>
        </el-form-item>
        <el-form-item label="动作">
          <el-select v-model="form.action" clearable style="width: 100%">
            <el-option label="(任意)" value="" />
            <el-option label="assigned" value="assigned" />
            <el-option label="labeled" value="labeled" />
            <el-option label="opened" value="opened" />
            <el-option label="created" value="created" />
          </el-select>
        </el-form-item>
        <el-form-item label="Label">
          <el-input v-model="form.label" placeholder="如 ai:analyze、ai:solve" />
        </el-form-item>
        <el-form-item label="Agent">
          <el-select v-model="form.agent_id" placeholder="选择 Agent" style="width: 100%">
            <el-option v-for="a in agents" :key="a.id" :label="a.name" :value="a.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="Mention">
          <el-input v-model="form.mention" placeholder="@用户名（可选）" />
        </el-form-item>
        <el-form-item label="优先级">
          <el-input-number v-model="form.priority" :min="0" :max="100" />
          <div class="form-tip">值越大越优先匹配</div>
        </el-form-item>
      </el-form>

      <!-- 快捷配置 -->
      <el-divider content-position="left">快捷配置</el-divider>
      <el-space wrap>
        <el-button size="small" @click="quickFill('issues', 'labeled', 'ai:analyze')">Issue + ai:analyze</el-button>
        <el-button size="small" @click="quickFill('issues', 'labeled', 'ai:solve')">Issue + ai:solve</el-button>
        <el-button size="small" @click="quickFill('issues', 'labeled', 'ai:fix')">Issue + ai:fix</el-button>
        <el-button size="small" @click="quickFill('pull_request', 'labeled', 'ai:review')">PR + ai:review</el-button>
      </el-space>

      <template #footer>
        <el-button @click="showAddDialog = false">取消</el-button>
        <el-button type="primary" @click="addRoute">添加</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const router = useRouter()
const routes = ref([])
const agents = ref([])

const filterEvent = ref('')
const filterAction = ref('')
const filterAgent = ref('')
const filterLabel = ref('')

const showAddDialog = ref(false)
const form = ref({
  event: 'issues',
  action: 'labeled',
  label: '',
  agent_id: null,
  mention: '',
  priority: 0
})

const agentMap = computed(() => {
  const map = {}
  for (const a of agents.value) map[a.id] = a.name
  return map
})

const getBehaviorTag = (route) => {
  const label = route.label || ''
  const event = route.event || ''
  const action = route.action || ''

  if (label === 'ai:solve') return { text: '自动开发，写代码并提 PR', type: 'warning', icon: '🛠️' }
  if (label === 'ai:fix') return { text: '自动修复 Bug 并提 PR', type: 'danger', icon: '🔧' }
  if (label === 'ai:analyze') return { text: '分析 Issue，输出需求报告', type: 'primary', icon: '📋' }
  if (label === 'ai:review') return { text: '审查 PR 代码，输出审查报告', type: 'primary', icon: '🔍' }

  if (event === 'issue_comment' || event === 'pull_request_comment') {
    return { text: '回复评论（只读）', type: 'success', icon: '💬' }
  }
  if (event === 'pull_request') return { text: '审查 PR，输出审查报告', type: 'primary', icon: '🔍' }
  if (event === 'issues' && (action === 'assigned' || action === 'labeled')) return { text: '分析 Issue，输出需求报告', type: 'primary', icon: '📋' }
  if (event === 'issues') return { text: '分析 Issue（默认行为）', type: 'info', icon: '📋' }

  return { text: '分析事件并回复', type: 'info', icon: '🤖' }
}

const loadRoutes = async () => {
  let allRoutes = await api.get('/routes') || []

  // Client-side filtering
  if (filterEvent.value) allRoutes = allRoutes.filter(r => r.event === filterEvent.value)
  if (filterAction.value) allRoutes = allRoutes.filter(r => r.action === filterAction.value)
  if (filterAgent.value) allRoutes = allRoutes.filter(r => r.agent_id === filterAgent.value)
  if (filterLabel.value) allRoutes = allRoutes.filter(r => r.label && r.label.includes(filterLabel.value))

  routes.value = allRoutes
}

const loadAgents = async () => {
  agents.value = await api.get('/agents') || []
}

const addRoute = async () => {
  if (!form.value.agent_id) {
    ElMessage.warning('请选择 Agent')
    return
  }
  try {
    await api.post('/routes', form.value)
    ElMessage.success('规则添加成功')
    showAddDialog.value = false
    form.value = { event: 'issues', action: 'labeled', label: '', agent_id: null, mention: '', priority: 0 }
    await loadRoutes()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '添加失败')
  }
}

const quickFill = (event, action, label) => {
  form.value.event = event
  form.value.action = action
  form.value.label = label
}

const deleteRoute = async (route) => {
  try {
    await ElMessageBox.confirm('确定删除这条规则？', '确认')
    await api.delete(`/routes/${route.id}`)
    ElMessage.success('删除成功')
    await loadRoutes()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error('删除失败')
  }
}

onMounted(() => {
  loadRoutes()
  loadAgents()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.filter-bar {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.filter-count {
  font-size: 13px;
  color: #909399;
  margin-left: auto;
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
