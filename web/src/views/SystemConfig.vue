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
            <el-form-item label="Provider 列表">
              <div class="provider-toolbar">
                <el-button type="primary" size="small" @click="openProviderDialog()">
                  <el-icon><Plus /></el-icon> 新增 Provider
                </el-button>
                <el-button size="small" @click="providerEditMode = providerEditMode === 'json' ? 'form' : 'json'">
                  <el-icon v-if="providerEditMode === 'json'"><Document /></el-icon>
                  <el-icon v-else><Edit /></el-icon>
                  {{ providerEditMode === 'json' ? '表单编辑' : 'JSON 编辑' }}
                </el-button>
                <el-tag v-if="sourceTag('llm.providers')" size="small" :type="sourceTag('llm.providers') === '数据库' ? 'success' : 'info'">
                  {{ sourceTag('llm.providers') }}
                </el-tag>
              </div>

              <!-- 表单模式：Provider 表格 -->
              <div v-if="providerEditMode === 'form'" class="provider-table-wrap">
                <el-table :data="providerList" border style="width: 100%" empty-text="暂无 Provider，点击上方按钮添加">
                  <el-table-column prop="name" label="名称" width="140" />
                  <el-table-column label="类型" width="120">
                    <template #default="{ row }">
                      <el-tag size="small" :type="row.type === 'anthropic' ? 'warning' : 'primary'">
                        {{ row.type === 'anthropic' ? 'Anthropic' : 'OpenAI 兼容' }}
                      </el-tag>
                    </template>
                  </el-table-column>
                  <el-table-column prop="base_url" label="Base URL">
                    <template #default="{ row }">
                      <span class="text-muted" v-if="!row.base_url">-</span>
                      <span v-else>{{ row.base_url }}</span>
                    </template>
                  </el-table-column>
                  <el-table-column label="API Key" width="120">
                    <template #default="{ row }">
                      <span v-if="row.api_key" class="api-key-masked">••••••••</span>
                      <span v-else class="text-muted">-</span>
                    </template>
                  </el-table-column>
                  <el-table-column label="操作" width="140" fixed="right">
                    <template #default="{ row, $index }">
                      <el-button size="small" type="primary" link @click="openProviderDialog(row, $index)">编辑</el-button>
                      <el-button size="small" type="danger" link @click="deleteProvider($index)">删除</el-button>
                    </template>
                  </el-table-column>
                </el-table>
              </div>

              <!-- JSON 模式：textarea -->
              <div v-else class="provider-json-wrap">
                <el-input
                  v-model="providersJson"
                  type="textarea"
                  :rows="10"
                  placeholder='{"deepseek":{"base_url":"https://api.deepseek.com/v1","api_key":"sk-xxx"}}'
                  @input="onProvidersJsonInput"
                />
                <div class="form-tip">
                  字段名使用 <code>base_url</code> 与 <code>api_key</code>
                  <el-button type="primary" link size="small" class="help-link" @click="$refs.providerHelp.show()">查看配置示例</el-button>
                  <span v-if="providerNames.length" class="provider-tags">
                    已识别：{{ providerNames.join('、') }}
                  </span>
                </div>
              </div>
            </el-form-item>
            <el-form-item label="默认 Provider">
              <el-select v-model="form['llm.defaults.provider']" placeholder="选择默认 Provider" style="width: 100%">
                <el-option v-for="(_, name) in providers" :key="name" :label="name" :value="name" />
              </el-select>
              <div class="form-tip">
                <el-tag v-if="sourceTag('llm.defaults.provider')" size="small" :type="sourceTag('llm.defaults.provider') === '数据库' ? 'success' : 'info'" style="margin-left: 8px">
                  {{ sourceTag('llm.defaults.provider') }}
                </el-tag>
              </div>
            </el-form-item>
            <el-form-item label="默认模型">
              <el-input v-model="form['llm.defaults.model']" placeholder="deepseek-chat" />
              <div class="form-tip">
                <el-tag v-if="sourceTag('llm.defaults.model')" size="small" :type="sourceTag('llm.defaults.model') === '数据库' ? 'success' : 'info'" style="margin-left: 8px">
                  {{ sourceTag('llm.defaults.model') }}
                </el-tag>
              </div>
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
            <el-form-item label="任务重试次数">
              <el-input-number v-model.number="form['dispatcher.task_retry_count']" :min="0" :max="5" />
              <div class="form-tip">整任务失败后自动重试次数（clone/runner 整次；默认 1）</div>
            </el-form-item>
            <el-form-item label="429 退避时间">
              <el-input-number v-model.number="form['dispatcher.rate_limit_backoff']" :min="0" :max="300" :step="5" />
              <div class="form-tip">LLM 返回 429 时等待秒数后再重试；0 表示关闭（默认 0）</div>
            </el-form-item>
            <el-form-item label="429 重试次数">
              <el-input-number v-model.number="form['llm.rate_limit_retries']" :min="0" :max="10" />
              <div class="form-tip">单次 ChatCompletion 遇 429 后的重试次数（需退避 &gt; 0；默认 1）</div>
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

        <!-- Tab 5: 调试 -->
        <el-tab-pane label="调试" name="debug">
          <el-alert title="调试功能默认关闭。开启后会将 Agent Loop 的 LLM 对话写入数据库，便于排查问题。" type="warning" :closable="false" style="margin-bottom: 16px" />
          <el-form label-width="180px" class="config-form">
            <el-form-item label="记录 Agent 对话">
              <el-switch v-model="form['debug.conversation_log.enabled']" />
              <div class="form-tip">
                开启后，solve / fix_bug 等多轮任务的每轮 LLM 消息与 tool call 将持久化到 <code>task_conversation_logs</code> 表
                <el-tag v-if="sourceTag('debug.conversation_log.enabled')" size="small" :type="sourceTag('debug.conversation_log.enabled') === '数据库' ? 'success' : 'info'" style="margin-left: 8px">
                  {{ sourceTag('debug.conversation_log.enabled') }}
                </el-tag>
              </div>
            </el-form-item>
            <el-form-item label="单条内容最大字符">
              <el-input-number v-model.number="form['debug.conversation_log.max_content_chars']" :min="0" :max="500000" :step="10000" />
              <div class="form-tip">写入数据库前截断 message / tool result 长度；0 表示不截断（默认 100000）</div>
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <!-- Tab 6: Prompt 模板 -->
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

    <!-- Provider 编辑对话框 -->
    <el-dialog
      v-model="providerDialogVisible"
      :title="editingProviderIndex >= 0 ? '编辑 Provider' : '新增 Provider'"
      width="560px"
      :close-on-click-modal="false"
    >
      <el-form :model="providerForm" label-width="120px">
        <el-form-item label="Provider 名称" required>
          <el-input
            v-model="providerForm.name"
            placeholder="如 deepseek、openai、ollama"
            :disabled="editingProviderIndex >= 0"
          />
          <div class="form-tip">唯一标识，创建后不可修改</div>
        </el-form-item>
        <el-form-item label="类型">
          <el-select v-model="providerForm.type" style="width: 100%">
            <el-option label="OpenAI 兼容（DeepSeek、Qwen、Ollama 等）" value="openai_compatible" />
            <el-option label="Anthropic (Claude)" value="anthropic" />
          </el-select>
        </el-form-item>
        <el-form-item label="Base URL">
          <el-input
            v-model="providerForm.base_url"
            placeholder="https://api.deepseek.com/v1"
          />
          <div class="form-tip">Anthropic 可留空</div>
        </el-form-item>
        <el-form-item label="API Key" required>
          <el-input
            v-model="providerForm.api_key"
            type="password"
            show-password
            placeholder="sk-xxx"
          />
        </el-form-item>

        <el-collapse v-model="providerAdvancedOpen" class="provider-advanced">
          <el-collapse-item title="高级配置" name="advanced">
            <el-form-item label="模型发现模式">
              <el-radio-group v-model="providerForm.model_discovery">
                <el-radio value="auto">自动发现（调用 /models API）</el-radio>
                <el-radio value="builtin">使用内置目录</el-radio>
                <el-radio value="custom">自定义列表</el-radio>
              </el-radio-group>
              <div class="form-tip">
                自动发现：尝试调用 Provider 的 /models 接口获取模型列表；失败时回退到内置目录
              </div>
            </el-form-item>
            <el-form-item v-if="providerForm.model_discovery === 'custom'" label="自定义模型">
              <el-input
                v-model="providerForm.models_text"
                type="textarea"
                :rows="4"
                placeholder="每行一个模型 ID，如：&#10;deepseek-chat&#10;deepseek-reasoner"
              />
            </el-form-item>
          </el-collapse-item>
        </el-collapse>
      </el-form>
      <template #footer>
        <el-button @click="providerDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveProvider">保存</el-button>
      </template>
    </el-dialog>

    <TemplateHelp ref="templateHelp" />
    <ProviderConfigHelp ref="providerHelp" />
  </div>
</template>

<script setup>
import { ref, onMounted, computed, watch } from 'vue'
import api from '../api'
import { Check, Plus, Document, Edit } from '@element-plus/icons-vue'
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

// Provider 可视化编辑状态
const providerEditMode = ref('form') // form | json
const providerDialogVisible = ref(false)
const editingProviderIndex = ref(-1)
const providerAdvancedOpen = ref([])
const providerForm = ref({
  name: '',
  type: 'openai_compatible',
  base_url: '',
  api_key: '',
  model_discovery: 'builtin', // auto | builtin | custom
  models_text: ''
})

const providerList = computed(() => {
  const map = providers.value
  return Object.entries(map).map(([name, cfg]) => ({
    name,
    type: cfg.type || 'openai_compatible',
    base_url: cfg.base_url || '',
    api_key: cfg.api_key || ''
  }))
})

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
      api_key: cfg.api_key || cfg.APIKey || '',
      type: cfg.type || 'openai_compatible',
      models: cfg.models || undefined,
      default_params: cfg.default_params || undefined
    }
  }
  return out
}

const formatProvidersJson = (raw) => JSON.stringify(normalizeProviders(raw), null, 2)

// 打开 Provider 编辑对话框
const openProviderDialog = (row = null, index = -1) => {
  editingProviderIndex.value = index
  if (row) {
    providerForm.value = {
      name: row.name,
      type: row.type || 'openai_compatible',
      base_url: row.base_url || '',
      api_key: row.api_key || '',
      model_discovery: 'builtin',
      models_text: ''
    }
    // 检测模型发现模式
    const cfg = providers.value[row.name]
    if (cfg) {
      if (cfg.models && Array.isArray(cfg.models) && cfg.models.length > 0) {
        providerForm.value.model_discovery = 'custom'
        providerForm.value.models_text = cfg.models.map(m => m.id || m).join('\n')
      } else if (cfg.models && Array.isArray(cfg.models) && cfg.models.length === 0) {
        providerForm.value.model_discovery = 'auto'
      } else {
        providerForm.value.model_discovery = 'builtin'
      }
    }
  } else {
    providerForm.value = {
      name: '',
      type: 'openai_compatible',
      base_url: '',
      api_key: '',
      model_discovery: 'builtin',
      models_text: ''
    }
  }
  providerAdvancedOpen.value = []
  providerDialogVisible.value = true
}

// 保存 Provider
const saveProvider = () => {
  const name = providerForm.value.name.trim()
  if (!name) {
    ElMessage.warning('请填写 Provider 名称')
    return
  }
  if (!providerForm.value.api_key.trim()) {
    ElMessage.warning('请填写 API Key')
    return
  }

  // 检查名称重复（新增时）
  if (editingProviderIndex.value < 0 && providers.value[name]) {
    ElMessage.warning('Provider 名称已存在')
    return
  }

  const current = JSON.parse(providersJson.value || '{}')
  const entry = {
    base_url: providerForm.value.base_url.trim(),
    api_key: providerForm.value.api_key.trim(),
    type: providerForm.value.type
  }

  // 根据模型发现模式设置 models 字段
  switch (providerForm.value.model_discovery) {
    case 'auto':
      entry.models = []
      break
    case 'custom':
      const ids = providerForm.value.models_text
        .split('\n')
        .map(s => s.trim())
        .filter(s => s)
      entry.models = ids.map(id => ({ id, name: id }))
      break
    // builtin: 不设置 models 字段
  }

  current[name] = entry
  providersJson.value = formatProvidersJson(current)
  providerDialogVisible.value = false
  ElMessage.success(editingProviderIndex.value >= 0 ? '已更新 Provider' : '已添加 Provider')
}

// 删除 Provider
const deleteProvider = async (index) => {
  const row = providerList.value[index]
  try {
    await ElMessageBox.confirm(`确定删除 Provider "${row.name}"？`, '确认', { type: 'warning' })
    const current = JSON.parse(providersJson.value || '{}')
    delete current[row.name]
    providersJson.value = formatProvidersJson(current)
    ElMessage.success('已删除')
  } catch {
    // cancel
  }
}

// JSON 输入时同步（防止格式错误时丢失数据）
const onProvidersJsonInput = () => {
  // 无需额外处理，providers computed 会自动解析
}

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
  }
  if (next._meta) {
    delete next._meta
  }
  if (next['debug.conversation_log.enabled'] === undefined) {
    next['debug.conversation_log.enabled'] = false
  }
  if (next['debug.conversation_log.max_content_chars'] === undefined) {
    next['debug.conversation_log.max_content_chars'] = 100000
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
      if (value === null || value === undefined) continue
      if (value === '' && typeof value !== 'boolean') continue
      entries[key] = String(value)
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

.provider-toolbar {
  margin-bottom: 12px;
  display: flex;
  gap: 8px;
  align-items: center;
}

.provider-table-wrap {
  margin-top: 8px;
}

.provider-json-wrap {
  margin-top: 8px;
}

.api-key-masked {
  font-family: monospace;
  letter-spacing: 2px;
}

.provider-advanced {
  margin-top: 8px;
}
</style>
