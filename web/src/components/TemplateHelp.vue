<template>
  <el-dialog v-model="visible" title="User Template 模板变量说明" width="700px" :close-on-click-modal="false">
    <p>User Template 使用 <b>Go template</b> 语法（如 <code v-text="'{{.Issue.Title}}'" />），在发送给 LLM 前会自动渲染为实际内容。</p>
    <h4>可用变量</h4>
    <el-table :data="templateVars" border size="small">
      <el-table-column prop="var" label="变量" width="200" />
      <el-table-column prop="desc" label="说明" />
    </el-table>
    <h4 style="margin-top: 16px">示例</h4>
    <el-input type="textarea" :rows="6" :model-value="templateExample" readonly />
  </el-dialog>
</template>

<script setup>
import { ref } from 'vue'

const visible = ref(false)

const show = () => { visible.value = true }

defineExpose({ show })

const templateVars = [
  { var: '{{.Issue.Number}}', desc: 'Issue 编号' },
  { var: '{{.Issue.Title}}', desc: 'Issue 标题' },
  { var: '{{.Issue.Body}}', desc: 'Issue 正文内容' },
  { var: '{{.Issue.State}}', desc: 'Issue 状态（open/closed）' },
  { var: '{{.Issue.User.Login}}', desc: 'Issue 作者用户名' },
  { var: '{{.Issue.Labels}}', desc: 'Issue 标签列表' },
  { var: '{{.PR.Number}}', desc: 'PR 编号' },
  { var: '{{.PR.Title}}', desc: 'PR 标题' },
  { var: '{{.PR.Body}}', desc: 'PR 正文内容' },
  { var: '{{.PR.Head.Ref}}', desc: 'PR 源分支' },
  { var: '{{.PR.Base.Ref}}', desc: 'PR 目标分支' },
  { var: '{{.PR.User.Login}}', desc: 'PR 作者用户名' },
  { var: '{{.Comment.Body}}', desc: '评论内容' },
  { var: '{{.Comment.User.Login}}', desc: '评论作者用户名' },
  { var: '{{.Repo.FullName}}', desc: '仓库全名（owner/repo）' },
  { var: '{{.Sender.Login}}', desc: '触发事件的用户' },
  { var: '{{.Event}}', desc: '事件类型（issues/pull_request 等）' },
  { var: '{{.Action}}', desc: '事件动作（opened/assigned 等）' },
]

const templateExample = `请分析以下 Issue：

## Issue #{{.Issue.Number}}: {{.Issue.Title}}

**仓库:** {{.Repo.FullName}}
**作者:** {{.Issue.User.Login}}
**状态:** {{.Issue.State}}

{{.Issue.Body}}`
</script>
