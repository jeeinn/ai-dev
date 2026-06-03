<template>
  <div class="agents-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>Agent 管理</span>
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>
            创建 Agent
          </el-button>
        </div>
      </template>

      <el-table :data="agents" style="width: 100%">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="name" label="名称" />
        <el-table-column prop="gitea_username" label="Gitea 用户" />
        <el-table-column prop="provider" label="Provider" width="100" />
        <el-table-column prop="model" label="模型" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ row.status }}</el-tag>
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
    <el-dialog v-model="showCreateDialog" :title="editingAgent ? '编辑 Agent' : '创建 Agent'" width="600px">
      <el-form :model="form" label-width="120px">
        <el-form-item label="名称">
          <el-input v-model="form.name" />
        </el-form-item>
        <el-form-item label="Gitea 用户名">
          <el-input v-model="form.gitea_username" :disabled="!!editingAgent" />
          <div v-if="editingAgent" class="form-tip">Gitea 用户名创建后不可修改</div>
        </el-form-item>
        <el-form-item label="Provider">
          <el-select v-model="form.provider">
            <el-option label="DeepSeek" value="deepseek" />
            <el-option label="OpenAI" value="openai" />
            <el-option label="Anthropic" value="anthropic" />
          </el-select>
        </el-form-item>
        <el-form-item label="模型">
          <el-input v-model="form.model" />
        </el-form-item>
        <el-form-item label="System Prompt">
          <el-input v-model="form.system_prompt" type="textarea" :rows="4" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="saveAgent">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const agents = ref([])
const showCreateDialog = ref(false)
const editingAgent = ref(null)

const form = ref({
  name: '',
  gitea_username: '',
  provider: 'deepseek',
  model: 'deepseek-chat',
  system_prompt: ''
})

const loadAgents = async () => {
  agents.value = await api.get('/agents')
}

const editAgent = (agent) => {
  editingAgent.value = agent
  form.value = { ...agent }
  showCreateDialog.value = true
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
    showCreateDialog.value = false
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

onMounted(loadAgents)
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
</style>
