<template>
  <div class="tasks-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>任务列表</span>
          <el-button @click="loadTasks">
            <el-icon><Refresh /></el-icon>
            刷新
          </el-button>
        </div>
      </template>

      <!-- 筛选栏 -->
      <div class="filter-bar">
        <el-select v-model="filterStatus" placeholder="状态" clearable style="width: 140px" @change="onFilterChange">
          <el-option label="待处理" value="pending" />
          <el-option label="运行中" value="running" />
          <el-option label="成功" value="success" />
          <el-option label="失败" value="failed" />
        </el-select>
        <el-select v-model="filterType" placeholder="任务类型" clearable style="width: 160px" @change="onFilterChange">
          <el-option v-for="t in taskTypes" :key="t" :label="t" :value="t" />
        </el-select>
        <el-select v-model="filterAgent" placeholder="Agent" clearable style="width: 180px" @change="onFilterChange">
          <el-option v-for="a in agents" :key="a.id" :label="a.name" :value="a.id" />
        </el-select>
        <span class="filter-count">共 {{ total }} 条</span>
      </div>

      <el-table v-loading="loading" :data="tasks" style="width: 100%">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="task_type" label="类型" width="120">
          <template #default="{ row }">
            <el-tag size="small" type="info">{{ row.task_type }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Agent" width="120">
          <template #default="{ row }">
            {{ agentMap[row.agent_id] || row.agent_id }}
          </template>
        </el-table-column>
        <el-table-column prop="repo" label="仓库" />
        <el-table-column prop="issue_id" label="Issue#" width="80" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)" size="small">{{ statusLabels[row.status] || row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="160">
          <template #default="{ row }">
            <el-button size="small" type="primary" link @click="viewTask(row)">详情</el-button>
            <el-button
              v-if="row.status === 'pending' || row.status === 'running'"
              size="small"
              type="warning"
              link
              :loading="resettingId === row.id"
              @click="resetTask(row)"
            >重置</el-button>
          </template>
        </el-table-column>
      </el-table>
      <div class="pagination-bar">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @current-change="loadTasks"
          @size-change="onPageSizeChange"
        />
      </div>
    </el-card>

    <!-- Task Detail Dialog -->
    <el-dialog v-model="showDetail" title="任务详情" width="700px" :close-on-click-modal="false">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="ID">{{ selectedTask?.id }}</el-descriptions-item>
        <el-descriptions-item label="类型">{{ selectedTask?.task_type }}</el-descriptions-item>
        <el-descriptions-item label="仓库">{{ selectedTask?.repo }}</el-descriptions-item>
        <el-descriptions-item label="Issue">{{ selectedTask?.issue_id }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="getStatusType(selectedTask?.status)">{{ selectedTask?.status }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ formatDate(selectedTask?.created_at) }}</el-descriptions-item>
      </el-descriptions>

      <div v-if="selectedTask?.result" class="task-result">
        <h4>执行结果</h4>
        <el-input type="textarea" :model-value="selectedTask.result" :rows="10" readonly />
      </div>

      <div v-if="selectedTask?.error" class="task-error">
        <h4>错误信息</h4>
        <el-alert :title="selectedTask.error" type="error" :closable="false" />
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import api from '../api'
import { Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const tasks = ref([])
const agents = ref([])
const showDetail = ref(false)
const selectedTask = ref(null)
const loading = ref(false)
const resettingId = ref(null)
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(20)

const filterStatus = ref('')
const filterType = ref('')
const filterAgent = ref('')

const statusLabels = { pending: '待处理', running: '运行中', success: '成功', failed: '失败' }

const agentMap = computed(() => {
  const map = {}
  for (const a of agents.value) map[a.id] = a.name
  return map
})

const taskTypes = ref(['analyze_issue', 'review_pr', 'reply_comment', 'solve_issue', 'fix_bug', 'trigger'])

const loadTasks = async () => {
  loading.value = true
  try {
    const offset = (currentPage.value - 1) * pageSize.value
    let params = `?limit=${pageSize.value}&offset=${offset}`
    if (filterStatus.value) params += `&status=${filterStatus.value}`
    if (filterType.value) params += `&type=${filterType.value}`
    if (filterAgent.value) params += `&agent_id=${filterAgent.value}`
    const res = await api.get(`/tasks${params}`)
    tasks.value = res?.data || []
    total.value = res?.total || 0
  } catch {
    tasks.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

const getStatusType = (status) => {
  const types = { pending: 'warning', running: 'info', success: 'success', failed: 'danger' }
  return types[status] || 'info'
}

const formatDate = (dateStr) => {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN')
}

const onFilterChange = () => {
  currentPage.value = 1
  loadTasks()
}

const onPageSizeChange = () => {
  currentPage.value = 1
  loadTasks()
}

const loadAgents = async () => {
  agents.value = await api.get('/agents') || []
}

const viewTask = (task) => {
  selectedTask.value = task
  showDetail.value = true
}

const resetTask = async (task) => {
  try {
    await ElMessageBox.confirm(
      `将任务 #${task.id}（${task.status}）标记为失败，以便重新触发该 Issue。确认重置？`,
      '重置任务状态',
      { type: 'warning', confirmButtonText: '重置', cancelButtonText: '取消' }
    )
  } catch {
    return
  }
  resettingId.value = task.id
  try {
    const res = await api.post(`/tasks/${task.id}/reset`)
    ElMessage.success(res?.message || '已重置')
    await loadTasks()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '重置失败')
  } finally {
    resettingId.value = null
  }
}

onMounted(() => {
  loadTasks()
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

.pagination-bar {
  display: flex;
  justify-content: flex-end;
  margin-top: 16px;
}

.task-result,
.task-error {
  margin-top: 20px;
}

.task-result h4,
.task-error h4 {
  margin-bottom: 10px;
}
</style>
