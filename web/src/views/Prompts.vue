<template>
  <div class="prompts-page">
    <el-card>
      <template #header>
        <span>Prompt 管理</span>
      </template>

      <el-tabs v-model="activeTab">
        <el-tab-pane label="内置模板" name="builtin">
          <el-table :data="templates" style="width: 100%">
            <el-table-column prop="name" label="名称" width="150" />
            <el-table-column prop="system_prompt" label="System Prompt">
              <template #default="{ row }">
                <div class="prompt-preview">{{ row.system_prompt?.substring(0, 100) }}...</div>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="100">
              <template #default="{ row }">
                <el-button size="small" @click="viewTemplate(row)">查看</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>

        <el-tab-pane label="自定义版本" name="custom">
          <el-alert title="选择一个 Agent 查看其 Prompt 历史版本。编辑 Agent 时修改 Prompt 会自动创建版本记录。" type="info" :closable="false" style="margin-bottom: 16px" />
          <el-select v-model="selectedAgent" placeholder="选择 Agent" @change="loadPrompts" style="width: 300px">
            <el-option v-for="agent in agents" :key="agent.id" :label="agent.name" :value="agent.id" />
          </el-select>

          <el-empty v-if="selectedAgent && (!prompts || prompts.length === 0)" description="暂无 Prompt 版本记录" />
          <el-table v-if="prompts && prompts.length" :data="prompts" style="width: 100%; margin-top: 20px">
            <el-table-column prop="version" label="版本" width="80" />
            <el-table-column prop="note" label="备注" />
            <el-table-column prop="is_active" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.is_active ? 'success' : 'info'">{{ row.is_active ? '活跃' : '历史' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="创建时间" width="180" />
            <el-table-column label="操作" width="150">
              <template #default="{ row }">
                <el-button v-if="!row.is_active" size="small" @click="rollback(row)">回滚</el-button>
                <el-button size="small" type="danger" @click="deletePrompt(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 查看模板对话框 -->
    <el-dialog v-model="viewDialogVisible" :title="'模板详情：' + (viewingTemplate?.name || '')" width="700px">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="名称">{{ viewingTemplate?.name }}</el-descriptions-item>
      </el-descriptions>
      <el-divider />
      <h4>System Prompt</h4>
      <el-input :model-value="viewingTemplate?.system_prompt" type="textarea" :rows="8" readonly />
      <h4 style="margin-top: 16px">User Template</h4>
      <el-input :model-value="viewingTemplate?.user_template" type="textarea" :rows="4" readonly />
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'
import { ElMessage } from 'element-plus'

const activeTab = ref('builtin')
const templates = ref([])
const agents = ref([])
const prompts = ref([])
const selectedAgent = ref(null)
const viewDialogVisible = ref(false)
const viewingTemplate = ref(null)

const viewTemplate = (row) => {
  viewingTemplate.value = row
  viewDialogVisible.value = true
}

const loadTemplates = async () => {
  try {
    const data = await api.get('/templates')
    // API 返回的是对象，转换为数组
    if (data && typeof data === 'object') {
      templates.value = Object.entries(data).map(([key, value]) => ({
        name: key,
        ...value
      }))
    } else {
      templates.value = []
    }
  } catch (e) {
    templates.value = []
  }
}

const loadAgents = async () => {
  try {
    agents.value = await api.get('/agents') || []
  } catch (e) {
    agents.value = []
  }
}

const loadPrompts = async () => {
  if (!selectedAgent.value) return
  try {
    prompts.value = await api.get(`/agents/${selectedAgent.value}/prompts`) || []
  } catch (e) {
    prompts.value = []
  }
}

const rollback = async (prompt) => {
  try {
    await api.post(`/prompts/${prompt.id}/activate`)
    ElMessage.success('回滚成功')
    loadPrompts()
  } catch (error) {
    ElMessage.error('回滚失败')
  }
}

const deletePrompt = async (prompt) => {
  try {
    await api.delete(`/prompts/${prompt.id}`)
    ElMessage.success('删除成功')
    loadPrompts()
  } catch (error) {
    ElMessage.error('删除失败')
  }
}

onMounted(() => {
  loadTemplates()
  loadAgents()
})
</script>

<style scoped>
.prompt-preview {
  max-width: 400px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
</style>
