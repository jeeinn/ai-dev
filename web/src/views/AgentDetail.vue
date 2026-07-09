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
            <el-form-item label="角色">
              <el-select v-model="form.role" style="width: 100%">
                <el-option label="分析 (analyze)" value="analyze" />
                <el-option label="开发 (coder)" value="coder" />
                <el-option label="审查 (review)" value="review" />
              </el-select>
              <div class="form-tip">Assign Agent 后的角色行为</div>
            </el-form-item>
            <el-form-item label="Gitea 用户">
              <el-input :model-value="form.gitea_username" disabled />
            </el-form-item>
            <el-form-item label="关联仓库">
              <el-select v-model="form.repos" multiple filterable placeholder="选择仓库（可多选）" style="width: 100%">
                <el-option v-for="r in repoList" :key="r.full_name" :label="r.full_name" :value="r.full_name" />
              </el-select>
              <div class="form-tip">自动将 Agent 添加为仓库协作者（用于创建 PR）。也可以在 Gitea 仓库设置 → 协作者中手动添加</div>
              <el-alert v-if="!form.repos || form.repos.length === 0" title="Agent 需要至少关联一个仓库才能获得协作者权限，用于创建 PR" type="warning" :closable="false" show-icon style="margin-top: 8px" />
            </el-form-item>

            <el-divider content-position="left">LLM 配置</el-divider>
            <el-form-item label="Provider">
              <el-col :span="11">
                <el-select v-model="form.provider" placeholder="选择 Provider" style="width: 100%">
                  <el-option v-for="name in effectiveProviderNames(form.provider)" :key="name" :label="name" :value="name" />
                </el-select>
              </el-col>
              <el-col :span="2" style="text-align: center; line-height: 32px">:</el-col>
              <el-col :span="11">
                <el-input v-model="form.model" placeholder="模型名称" />
              </el-col>
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
              <el-button type="primary" link size="small" style="margin-top: 4px" @click="$refs.templateHelp.show()">查看模板变量说明</el-button>
            </el-form-item>

            <el-form-item>
              <el-button type="primary" :loading="saving" @click="saveAgent">保存修改</el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-tab-pane>

      <!-- Tab 2: 触发规则 (v2 deprecated) -->
      <el-tab-pane label="触发规则" name="routes">
        <el-card>
          <el-empty description="触发规则已弃用 — v2 使用 Assign Agent 模型触发工作流">
            <template #description>
              <p>v2 已弃用基于 Label 的触发规则。</p>
              <p style="margin-top: 8px">请在 Issue/PR 上 <strong>Assign Agent</strong> 来触发工作流。</p>
            </template>
          </el-empty>
        </el-card>
      </el-tab-pane>

      <!-- Tab 3: Prompt 版本 -->
      <el-tab-pane label="Prompt 版本" name="prompts">
        <el-card>
          <el-empty v-if="!prompts.length" description="暂无 Prompt 版本记录" />
          <el-table v-else :data="paginatedPrompts" style="width: 100%">
            <el-table-column prop="version" label="版本" width="80" />
            <el-table-column prop="note" label="备注" />
            <el-table-column prop="is_active" label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="row.is_active ? 'success' : 'info'" size="small">{{ row.is_active ? '活跃' : '历史' }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="created_at" label="创建时间" width="180" />
            <el-table-column label="操作" width="180">
              <template #default="{ row }">
                <el-button size="small" type="primary" link @click="viewPromptDetail(row)">详情</el-button>
                <el-button v-if="!row.is_active" size="small" @click="rollbackPrompt(row)">回滚</el-button>
                <el-button size="small" type="danger" @click="deletePrompt(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
          <div v-if="prompts.length > 10" class="pagination-bar">
            <el-pagination v-model:current-page="promptPage" :page-size="10" :total="prompts.length" layout="prev, pager, next" small />
          </div>
        </el-card>
      </el-tab-pane>
    </el-tabs>

    <!-- Prompt 详情对话框 -->
    <el-dialog v-model="showPromptDetail" title="Prompt 版本详情" width="700px" :close-on-click-modal="false">
      <el-descriptions :column="2" border style="margin-bottom: 16px">
        <el-descriptions-item label="版本">{{ viewingPrompt?.version }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="viewingPrompt?.is_active ? 'success' : 'info'" size="small">{{ viewingPrompt?.is_active ? '活跃' : '历史' }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="备注">{{ viewingPrompt?.note || '-' }}</el-descriptions-item>
        <el-descriptions-item label="创建时间">{{ viewingPrompt?.created_at }}</el-descriptions-item>
      </el-descriptions>
      <h4>System Prompt</h4>
      <el-input :model-value="viewingPrompt?.system_prompt" type="textarea" :rows="8" readonly />
      <h4 style="margin-top: 16px">User Template</h4>
      <el-input :model-value="viewingPrompt?.user_template" type="textarea" :rows="4" readonly />
    </el-dialog>

    <TemplateHelp ref="templateHelp" />
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import api from '../api'
import { ElMessage, ElMessageBox } from 'element-plus'
import TemplateHelp from '../components/TemplateHelp.vue'
import { useAgentDefaults } from '../composables/useAgentDefaults'

const route = useRoute()
const router = useRouter()
const agentId = ref(route.params.id)
const {
  loadAgentConfig,
  effectiveProviderNames,
  createEmptyAgentForm,
  defaultLoopConfig
} = useAgentDefaults()

const activeTab = ref('info')
const agent = ref(null)
const prompts = ref([])
const repoList = ref([])
const promptPage = ref(1)

const paginatedPrompts = computed(() => {
  const start = (promptPage.value - 1) * 10
  return prompts.value.slice(start, start + 10)
})
const saving = ref(false)
const showPromptDetail = ref(false)
const viewingPrompt = ref(null)

const defaultForm = createEmptyAgentForm()

const form = ref({ ...defaultForm, loop_config: { ...defaultLoopConfig } })

const loadRepos = async () => {
  try {
    repoList.value = await api.get('/repos') || []
  } catch {
    repoList.value = []
  }
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

const loadPrompts = async () => {
  try {
    const data = await api.get(`/agents/${agentId.value}/prompts`)
    prompts.value = Array.isArray(data) ? data : []
  } catch {
    prompts.value = []
  }
}

const saveAgent = async () => {
  saving.value = true
  try {
    const res = await api.put(`/agents/${agentId.value}`, form.value)
    if (res?.repo_warnings?.length > 0) {
      ElMessage.warning(`保存成功，但部分仓库关联失败：${res.repo_warnings.join('; ')}`)
    } else {
      ElMessage.success('保存成功')
    }
    await loadAgent()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const viewPromptDetail = (prompt) => {
  viewingPrompt.value = prompt
  showPromptDetail.value = true
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
  if (tab === 'prompts') loadPrompts()
})

onMounted(async () => {
  await loadAgentConfig()
  await loadAgent()
  loadRepos()
})
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

.pagination-bar {
  display: flex;
  justify-content: flex-end;
  margin-top: 12px;
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
