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

      <el-table :data="tasks" style="width: 100%">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="task_type" label="类型" width="120" />
        <el-table-column prop="repo" label="仓库" />
        <el-table-column prop="issue_id" label="Issue#" width="80" />
        <el-table-column prop="status" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="getStatusType(row.status)">{{ row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" @click="viewTask(row)">详情</el-button>
          </template>
        </el-table-column>
      </el-table>
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
import { ref, onMounted } from 'vue'
import api from '../api'
import { Refresh } from '@element-plus/icons-vue'

const tasks = ref([])
const showDetail = ref(false)
const selectedTask = ref(null)

const getStatusType = (status) => {
  const types = {
    pending: 'warning',
    running: 'info',
    success: 'success',
    failed: 'danger'
  }
  return types[status] || 'info'
}

const formatDate = (dateStr) => {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleString('zh-CN')
}

const loadTasks = async () => {
  tasks.value = await api.get('/tasks?limit=50')
}

const viewTask = (task) => {
  selectedTask.value = task
  showDetail.value = true
}

onMounted(loadTasks)
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
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
