<template>
  <div class="system-config-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>系统配置</span>
          <el-button type="primary" :loading="saving" @click="saveAll">
            <el-icon><Check /></el-icon>
            保存全部
          </el-button>
        </div>
      </template>

      <el-alert
        v-if="setupHint"
        :title="setupHint"
        type="info"
        :closable="false"
        show-icon
        style="margin-bottom: 16px"
      />

      <el-tabs v-model="activeTab">
        <!-- Tab 1: Gitea 连接 -->
        <el-tab-pane label="Gitea 连接" name="gitea">
          <el-form label-width="140px" class="config-form">
            <el-form-item label="Gitea 地址">
              <el-input v-model="form['gitea.url']" placeholder="http://localhost:3000" />
              <div class="form-tip">
                Gitea 服务的访问地址
                <el-tag v-if="sourceTag('gitea.url')" size="small" :type="sourceTag('gitea.url') === '数据库' ? 'success' : 'info'" style="margin-left: 8px">
                  {{ sourceTag('gitea.url') }}
                </el-tag>
              </div>
            </el-form-item>
            <el-form-item label="管理员 Token">
              <el-input v-model="form['gitea.admin_token']" type="password" show-password placeholder="Gitea 管理员 Token" />
              <div class="form-tip">
                用于自动创建 Agent 账号，需包含 <code>write:admin</code> 权限
                <el-tag v-if="sourceTag('gitea.admin_token')" size="small" :type="sourceTag('gitea.admin_token') === '数据库' ? 'success' : 'info'" style="margin-left: 8px">
                  {{ sourceTag('gitea.admin_token') }}
                </el-tag>
              </div>
            </el-form-item>
            <el-form-item label="Webhook 密钥">
              <el-input v-model="form['gitea.webhook_secret']" type="password" show-password placeholder="Webhook 签名密钥" />
              <div class="form-tip">
                与 Gitea Webhook 设置中的密钥一致
                <el-tag v-if="sourceTag('gitea.webhook_secret')" size="small" :type="sourceTag('gitea.webhook_secret') === '数据库' ? 'success' : 'info'" style="margin-left: 8px">
                  {{ sourceTag('gitea.webhook_secret') }}
                </el-tag>
              </div>
            </el-form-item>
            <el-form-item>
              <el-button :loading="testingGitea" @click="testGitea">测试 Gitea 连接</el-button>
              <span v-if="giteaTestMessage" :class="['test-result', giteaTestOk ? 'ok' : 'error']">{{ giteaTestMessage }}</span>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <!-- Tab 2: LLM 配置 -->
        <el-tab-pane label="LLM 配置" name="llm">
          <el-alert title="配置 LLM Provider 后，Agent 创建时可从已配置的 Provider 中选择" type="info" :closable="false" style="margin-bottom: 16px" />
          <el-form label-width="140px" class="config-form">
            <el-form-item label="Provider 配置">
              <el-input
                v-model="providersJson"
                type="textarea"
                :rows="8"
                placeholder='{"deepseek":{"base_url":"https://api.deepseek.com/v1","api_key":"sk-xxx"}}'
              />
              <div class="form-tip">
                JSON 格式，可配置多个 Provider；字段名使用 <code>base_url</code> 与 <code>api_key</code>
                <el-button type="primary" link size="small" class="help-link" @click="$refs.providerHelp.show()">点击查看配置示例</el-button>
                <span v-if="providerNames.length" class="provider-tags">
                  已识别：{{ providerNames.join('、') }}
                </span>
              </div>
            </el-form-item>
            <el-form-item label="默认 Provider">
              <el-select v-model="form['llm.defaults.provider']" placeholder="选择默认 Provider" style="width: 100%">
                <el-option v-for="(_, name) in providers" :key="name" :label="name" :value="name" />
              </el-select>
            </el-form-item>
            <el-form-item label="默认模型">
              <el-input v-model="form['llm.defaults.model']" placeholder="deepseek-chat" />
            </el-form-item>
            <el-form-item>
              <el-button :loading="testingLLM" @click="testLLM">测试 LLM 连接</el-button>
              <span v-if="llmTestMessage" :class="['test-result', llmTestOk ? 'ok' : 'error']">{{ llmTestMessage }}</span>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <!-- Tab 3: 任务调度 -->
        <el-tab-pane label="任务调度" name="dispatcher">
          <el-alert title="调整任务执行的并发和重试参数；任务超时由 Agent 配置控制" type="info" :closable="false" style="margin-bottom: 16px" />
          <el-form label-width="140px" class="config-form">
            <el-form-item label="最大并发数">
              <el-input-number v-model.number="form['dispatcher.max_concurrent']" :min="1" :max="20" />
              <div class="form-tip">同时执行的 Agent 任务数量（默认 3）</div>
            </el-form-item>
            <el-form-item label="失败重试次数">
              <el-input-number v-model.number="form['dispatcher.retry_count']" :min="0" :max="5" />
              <div class="form-tip">任务失败后自动重试次数（默认 1）</div>
            </el-form-item>
            <el-form-item label="429 退避时间">
              <el-input-number v-model.number="form['dispatcher.rate_limit_backoff']" :min="0" :max="300" :step="5" />
              <div class="form-tip">LLM 返回 429 时等待秒数后再重试；0 表示关闭（默认 0）</div>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <!-- Tab 4: Agent 默认参数 -->
        <el-tab-pane label="Agent 默认参数" name="agents">
          <el-alert title="新建 Agent 时的默认参数，可在 Agent 编辑中单独覆盖" type="info" :closable="false" style="margin-bottom: 16px" />
          <el-form label-width="160px" class="config-form">
            <el-form-item label="默认 Provider">
              <el-select v-model="form['agents.defaults.provider']" placeholder="选择默认 Provider" style="width: 100%">
                <el-option v-for="(_, name) in providers" :key="name" :label="name" :value="name" />
              </el-select>
            </el-form-item>
            <el-form-item label="默认模型">
              <el-input v-model="form['agents.defaults.model']" placeholder="deepseek-chat" />
            </el-form-item>

            <el-divider content-position="left">LLM Token</el-divider>
            <el-form-item label="最大输出 Tokens">
              <el-input-number v-model.number="form['agents.defaults.max_output_tokens']" :min="256" :max="128000" :step="512" />
              <div class="form-tip">每次调用的最大输出 Tokens（单次任务与 Loop 每轮共用）</div>
            </el-form-item>
            <el-form-item label="最大输入 Tokens">
              <el-input-number v-model.number="form['agents.defaults.max_input_tokens']" :min="1024" :max="200000" :step="1024" />
              <div class="form-tip">每次请求送入模型的输入上限（含 tools）；估算为字符数/4，仅供管控</div>
            </el-form-item>
            <el-form-item label="Temperature">
              <el-slider v-model.number="form['agents.defaults.temperature']" :min="0" :max="2" :step="0.1" show-input style="width: 100%" />
            </el-form-item>
            <el-form-item label="单次任务超时">
              <el-input v-model="form['agents.defaults.timeout']" placeholder="5m" style="width: 200px" />
              <div class="form-tip">analyze / review / reply 等单次任务总超时（Go duration，如 5m）</div>
            </el-form-item>

            <el-divider content-position="left">Agent Loop 默认参数</el-divider>
            <el-form-item label="最大迭代轮数">
              <el-input-number v-model.number="form['agents.loop.max_iterations']" :min="1" :max="100" />
            </el-form-item>
            <el-form-item label="Loop 总超时">
              <el-input v-model="form['agents.loop.total_timeout']" placeholder="30m" style="width: 200px" />
              <div class="form-tip">仅 solve / fix_bug 等多轮任务使用</div>
            </el-form-item>
            <el-form-item label="轮次间隔">
              <el-input-number v-model.number="form['agents.loop.iteration_interval']" :min="0" :max="300" :step="1" />
              <div class="form-tip">每轮 Agent Loop 之间的等待秒数；0 表示不等待</div>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <!-- Tab 5: Prompt 模板 -->
        <el-tab-pane label="Prompt 模板" name="prompts">
          <el-alert title="管理内置 Prompt 模板。自定义模板优先级高于内置模板（DB > 内置）。" type="info" :closable="false" style="margin-bottom: 16px" />
          <div style="margin-bottom: 12px">
            <el-button type="primary" size="small" @click="showAddTemplate = true">
              <el-icon><Plus /></el-icon> 新增模板
            </el-button>
          </div>
          <el-table :data="templateList" style="width: 100%">
            <el-table-column prop="name" label="名称" width="160" />
            <el-table-column prop="source" label="来源" width="100">
              <template #default="{ row }">
                <el-tag :type="row.source === 'custom' ? 'success' : 'info'" size="small">
                  {{ row.source === 'custom' ? '自定义' : row.source === 'config' ? '配置文件' : '内置' }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="system_prompt" label="System Prompt">
              <template #default="{ row }">
                <span class="prompt-preview">{{ row.system_prompt?.substring(0, 80) }}...</span>
              </template>
            </el-table-column>
            <el-table-column label="操作" width="150">
              <template #default="{ row }">
                <el-button size="small" type="primary" link @click="viewTemplate(row)">查看</el-button>
                <el-button v-if="row.source === 'custom'" size="small" type="danger" link @click="deleteTemplate(row)">删除</el-button>
              </template>
            </el-table-column>
          </el-table>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 查看模板对话框 -->
    <el-dialog v-model="showViewTemplate" :title="'模板详情：' + (viewingTemplate?.name || '')" width="700px" :close-on-click-modal="false">
      <h4>System Prompt</h4>
      <el-input :model-value="viewingTemplate?.system_prompt" type="textarea" :rows="8" readonly />
      <h4 style="margin-top: 16px">User Template</h4>
      <el-input :model-value="viewingTemplate?.user_template" type="textarea" :rows="4" readonly />
    </el-dialog>

    <!-- 新增模板对话框 -->
    <el-dialog v-model="showAddTemplate" title="新增 Prompt 模板" width="700px" :close-on-click-modal="false">
      <el-form :model="newTemplate" label-width="120px">
        <el-form-item label="模板名称">
          <el-input v-model="newTemplate.name" placeholder="如 my_review" />
          <div class="form-tip">唯一标识，创建后不可修改</div>
        </el-form-item>
        <el-form-item label="System Prompt">
          <el-input v-model="newTemplate.system_prompt" type="textarea" :rows="8" placeholder="Agent 的系统提示词" />
        </el-form-item>
        <el-form-item label="User Template">
          <el-input v-model="newTemplate.user_template" type="textarea" :rows="4" placeholder="用户消息模板（支持 Go template 语法）" />
          <el-button type="primary" link size="small" style="margin-top: 4px" @click="$refs.templateHelp.show()">查看模板变量说明</el-button>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAddTemplate = false">取消</el-button>
        <el-button type="primary" @click="addTemplate">创建</el-button>
      </template>
    </el-dialog>

    <TemplateHelp ref="templateHelp" />
    <ProviderConfigHelp ref="providerHelp" />
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import api from '../api'
import { Check, Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import TemplateHelp from '../components/TemplateHelp.vue'
import ProviderConfigHelp from '../components/ProviderConfigHelp.vue'

const activeTab = ref('gitea')
const form = ref({})
const sources = ref({})
const saving = ref(false)
const testingGitea = ref(false)
const testingLLM = ref(false)
const giteaTestMessage = ref('')
const giteaTestOk = ref(false)
const llmTestMessage = ref('')
const llmTestOk = ref(false)
const providersJson = ref('')
const templateList = ref([])
const showViewTemplate = ref(false)
const viewingTemplate = ref(null)
const showAddTemplate = ref(false)
const newTemplate = ref({ name: '', system_prompt: '', user_template: '' })

const providers = computed(() => {
  try {
    return normalizeProviders(JSON.parse(providersJson.value))
  } catch {
    return {}
  }
})

const providerNames = computed(() => Object.keys(providers.value))

const normalizeProviders = (raw) => {
  const out = {}
  for (const [name, cfg] of Object.entries(raw || {})) {
    if (!cfg || typeof cfg !== 'object') continue
    out[name] = {
      base_url: cfg.base_url || cfg.BaseURL || '',
      api_key: cfg.api_key || cfg.APIKey || ''
    }
  }
  return out
}

const formatProvidersJson = (raw) => JSON.stringify(normalizeProviders(raw), null, 2)

const setupHint = computed(() => {
  const fileCount = Object.values(sources.value).filter(v => v === 'file').length
  if (fileCount === 0) return ''
  return `有 ${fileCount} 项配置来自 config.yaml，保存后将写入数据库。建议先完成 Gitea 连接测试，再配置 LLM，最后到 Agent 管理创建 Agent。`
})

const sourceTag = (key) => {
  const src = sources.value[key]
  if (!src) return ''
  return src === 'db' ? '数据库' : 'config.yaml'
}

const applyConfigData = (data) => {
  const next = { ...data }
  if (next._meta?.sources) {
    sources.value = next._meta.sources
    delete next._meta
  }
  form.value = next
  if (data['llm.providers']) {
    providersJson.value = formatProvidersJson(data['llm.providers'])
  }
}

const loadConfig = async () => {
  const data = await api.get('/config')
  applyConfigData(data)
}

const loadTemplates = async () => {
  try {
    const data = await api.get('/prompt-templates')
    templateList.value = Object.entries(data).map(([key, val]) => ({
      name: key,
      ...val
    }))
  } catch {
    templateList.value = []
  }
}

const viewTemplate = (row) => {
  viewingTemplate.value = row
  showViewTemplate.value = true
}

const addTemplate = async () => {
  if (!newTemplate.value.name || !newTemplate.value.system_prompt) {
    ElMessage.warning('请填写模板名称和 System Prompt')
    return
  }
  try {
    const payload = {}
    payload[newTemplate.value.name] = {
      name: newTemplate.value.name,
      system_prompt: newTemplate.value.system_prompt,
      user_template: newTemplate.value.user_template || ''
    }
    await api.put('/prompt-templates', payload)
    ElMessage.success('模板创建成功')
    showAddTemplate.value = false
    newTemplate.value = { name: '', system_prompt: '', user_template: '' }
    await loadTemplates()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '创建失败')
  }
}

const deleteTemplate = async (row) => {
  try {
    await ElMessageBox.confirm(`确定删除模板"${row.name}"？`, '确认')
    await api.delete(`/prompt-templates/${row.name}`)
    ElMessage.success('删除成功')
    await loadTemplates()
  } catch (error) {
    if (error !== 'cancel') ElMessage.error('删除失败')
  }
}

const saveAll = async () => {
  saving.value = true
  try {
    // Parse providers JSON
    let providersData
    try {
      providersData = normalizeProviders(JSON.parse(providersJson.value))
    } catch {
      ElMessage.error('Provider 配置 JSON 格式错误')
      saving.value = false
      return
    }
    if (Object.keys(providersData).length === 0) {
      ElMessage.error('请至少配置一个 Provider')
      saving.value = false
      return
    }

    // Build entries to save
    const entries = {}
    for (const [key, value] of Object.entries(form.value)) {
      if (key === 'llm.providers') continue // handle separately
      if (value !== null && value !== undefined && value !== '') {
        entries[key] = String(value)
      }
    }
    entries['llm.providers'] = JSON.stringify(providersData)

    const data = await api.put('/config', entries)
    applyConfigData(data)
    ElMessage.success('配置已保存')
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '保存失败')
  } finally {
    saving.value = false
  }
}

const testGitea = async () => {
  testingGitea.value = true
  giteaTestMessage.value = ''
  try {
    const result = await api.post('/config/test/gitea', {
      'gitea.url': form.value['gitea.url'] || '',
      'gitea.admin_token': form.value['gitea.admin_token'] || ''
    })
    giteaTestOk.value = !!result.ok
    giteaTestMessage.value = result.message
  } catch (error) {
    giteaTestOk.value = false
    giteaTestMessage.value = error.response?.data?.message || error.response?.data?.error || '测试失败'
  } finally {
    testingGitea.value = false
  }
}

const testLLM = async () => {
  testingLLM.value = true
  llmTestMessage.value = ''
  try {
    let providersData
    try {
      providersData = normalizeProviders(JSON.parse(providersJson.value))
    } catch {
      ElMessage.error('Provider 配置 JSON 格式错误')
      testingLLM.value = false
      return
    }
    const result = await api.post('/config/test/llm', {
      'llm.defaults.provider': form.value['llm.defaults.provider'] || '',
      'llm.defaults.model': form.value['llm.defaults.model'] || '',
      'agents.defaults.max_output_tokens': form.value['agents.defaults.max_output_tokens'],
      'llm.providers': providersData
    })
    llmTestOk.value = !!result.ok
    llmTestMessage.value = result.message
  } catch (error) {
    llmTestOk.value = false
    llmTestMessage.value = error.response?.data?.message || error.response?.data?.error || '测试失败'
  } finally {
    testingLLM.value = false
  }
}

onMounted(() => {
  loadConfig()
  loadTemplates()
})
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.config-form {
  max-width: 700px;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.prompt-preview {
  max-width: 400px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 13px;
  color: #606266;
}

.test-result {
  margin-left: 12px;
  font-size: 13px;
}

.test-result.ok {
  color: #67c23a;
}

.test-result.error {
  color: #f56c6c;
}

.form-tip code {
  font-size: 12px;
}

.help-link {
  margin-left: 8px;
  padding: 0;
  vertical-align: baseline;
}

.provider-tags {
  display: block;
  margin-top: 4px;
  color: #67c23a;
}
</style>
