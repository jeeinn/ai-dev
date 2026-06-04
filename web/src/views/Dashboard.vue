<template>
  <div class="dashboard">
    <!-- 新用户引导 -->
    <el-card v-if="agents.length === 0" class="welcome-card" shadow="hover">
      <div class="welcome-content">
        <h2>👋 欢迎使用 Gitea Agent Gateway</h2>
        <p class="welcome-desc">按照以下步骤快速开始使用</p>
        <el-steps :active="welcomeStep" direction="vertical" class="welcome-steps">
          <el-step title="配置 Gitea 连接" description="在系统配置中填写 Gitea 地址和管理员 Token">
            <template #icon>
              <el-icon><Setting /></el-icon>
            </template>
            <template #extra>
              <el-button size="small" type="primary" @click="router.push('/config')">去配置</el-button>
            </template>
          </el-step>
          <el-step title="创建第一个 Agent" description="选择内置模板快速创建，或自定义配置">
            <template #icon>
              <el-icon><User /></el-icon>
            </template>
            <template #extra>
              <el-button size="small" type="primary" @click="router.push('/agents')">去创建</el-button>
            </template>
          </el-step>
          <el-step title="配置触发规则" description="在 Agent 详情页设置 Label 触发条件">
            <template #icon>
              <el-icon><Connection /></el-icon>
            </template>
          </el-step>
        </el-steps>
      </div>
    </el-card>

    <el-row :gutter="20">
      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>
            <div class="card-header">
              <span>Agent 数量</span>
              <el-icon><User /></el-icon>
            </div>
          </template>
          <div class="stat-value">{{ stats.total_agents || 0 }}</div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>
            <div class="card-header">
              <span>任务总数</span>
              <el-icon><List /></el-icon>
            </div>
          </template>
          <div class="stat-value">{{ stats.total_tasks || 0 }}</div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>
            <div class="card-header">
              <span>待处理</span>
              <el-icon><Clock /></el-icon>
            </div>
          </template>
          <div class="stat-value pending">{{ pendingCount }}</div>
        </el-card>
      </el-col>

      <el-col :span="6">
        <el-card shadow="hover">
          <template #header>
            <div class="card-header">
              <span>成功率</span>
              <el-icon><CircleCheck /></el-icon>
            </div>
          </template>
          <div class="stat-value success">{{ successRate }}%</div>
        </el-card>
      </el-col>
    </el-row>

    <el-row :gutter="20" class="mt-20">
      <el-col :span="12">
        <el-card>
          <template #header>
            <span>最近任务</span>
          </template>
          <el-table :data="recentTasks" style="width: 100%">
            <el-table-column prop="id" label="ID" width="60" />
            <el-table-column prop="task_type" label="类型" width="120" />
            <el-table-column prop="repo" label="仓库" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="getStatusType(row.status)">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card>
          <template #header>
            <span>Agent 列表</span>
          </template>
          <el-table :data="agents" style="width: 100%">
            <el-table-column prop="id" label="ID" width="60" />
            <el-table-column prop="name" label="名称" />
            <el-table-column prop="provider" label="Provider" width="100" />
            <el-table-column prop="model" label="模型" />
            <el-table-column prop="status" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.status === 'active' ? 'success' : 'info'">{{ row.status }}</el-tag>
              </template>
            </el-table-column>
          </el-table>
        </el-card>
      </el-col>
    </el-row>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import api from '../api'
import { User, List, Clock, CircleCheck, Setting, Connection } from '@element-plus/icons-vue'

const router = useRouter()

const stats = ref({})
const agents = ref([])
const recentTasks = ref([])

const pendingCount = computed(() => {
  return recentTasks.value.filter(t => t.status === 'pending').length
})

const successRate = computed(() => {
  const total = recentTasks.value.length
  if (total === 0) return 0
  const success = recentTasks.value.filter(t => t.status === 'success').length
  return Math.round((success / total) * 100)
})

const welcomeStep = computed(() => {
  if (agents.value.length === 0) return 1
  return 3
})

const getStatusType = (status) => {
  const types = {
    pending: 'warning',
    running: 'info',
    success: 'success',
    failed: 'danger'
  }
  return types[status] || 'info'
}

onMounted(async () => {
  try {
    const [statsData, agentsData, tasksData] = await Promise.all([
      api.get('/stats'),
      api.get('/agents'),
      api.get('/tasks?limit=10')
    ])
    stats.value = statsData
    agents.value = agentsData
    recentTasks.value = tasksData
  } catch (error) {
    console.error('Failed to load dashboard data:', error)
  }
})
</script>

<style scoped>
.dashboard {
  padding: 20px;
}

.welcome-card {
  margin-bottom: 20px;
  background: linear-gradient(135deg, #e8f4fd 0%, #f0f9ff 100%);
}

.welcome-content {
  padding: 10px 20px;
}

.welcome-content h2 {
  margin: 0 0 8px 0;
  color: #303133;
}

.welcome-desc {
  color: #606266;
  margin-bottom: 20px;
}

.welcome-steps {
  max-width: 500px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.stat-value {
  font-size: 36px;
  font-weight: bold;
  text-align: center;
  color: #303133;
}

.stat-value.pending {
  color: #e6a23c;
}

.stat-value.success {
  color: #67c23a;
}

.mt-20 {
  margin-top: 20px;
}
</style>
