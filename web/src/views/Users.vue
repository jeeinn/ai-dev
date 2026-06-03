<template>
  <div class="users-page">
    <el-card>
      <template #header>
        <div class="card-header">
          <span>用户管理</span>
          <el-button type="primary" @click="showCreateDialog = true">
            <el-icon><Plus /></el-icon>
            创建用户
          </el-button>
        </div>
      </template>

      <el-table :data="users" style="width: 100%">
        <el-table-column prop="id" label="ID" width="60" />
        <el-table-column prop="username" label="用户名" />
        <el-table-column prop="display_name" label="显示名称" />
        <el-table-column prop="role" label="角色" width="100">
          <template #default="{ row }">
            <el-tag :type="getRoleType(row.role)">{{ row.role }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="is_active" label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.is_active ? 'success' : 'danger'">{{ row.is_active ? '活跃' : '禁用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" @click="editUser(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="deleteUser(row)" :disabled="row.username === 'admin'">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- Create/Edit Dialog -->
    <el-dialog v-model="showCreateDialog" :title="editingUser ? '编辑用户' : '创建用户'" width="500px">
      <el-form :model="form" label-width="100px">
        <el-form-item label="用户名">
          <el-input v-model="form.username" :disabled="!!editingUser" />
        </el-form-item>
        <el-form-item v-if="!editingUser" label="密码">
          <el-input v-model="form.password" type="password" />
        </el-form-item>
        <el-form-item label="显示名称">
          <el-input v-model="form.display_name" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="form.email" />
        </el-form-item>
        <el-form-item label="角色">
          <el-select v-model="form.role">
            <el-option label="管理员" value="admin" />
            <el-option label="用户" value="user" />
            <el-option label="查看者" value="viewer" />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" @click="saveUser">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import api from '../api'
import { Plus } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'

const users = ref([])
const showCreateDialog = ref(false)
const editingUser = ref(null)

const form = ref({
  username: '',
  password: '',
  display_name: '',
  email: '',
  role: 'user'
})

const getRoleType = (role) => {
  const types = { admin: 'danger', user: 'primary', viewer: 'info' }
  return types[role] || 'info'
}

const loadUsers = async () => {
  users.value = await api.get('/users')
}

const editUser = (user) => {
  editingUser.value = user
  form.value = { ...user, password: '' }
  showCreateDialog.value = true
}

const saveUser = async () => {
  try {
    if (editingUser.value) {
      await api.put(`/users/${editingUser.value.id}`, form.value)
      ElMessage.success('更新成功')
    } else {
      await api.post('/users', form.value)
      ElMessage.success('创建成功')
    }
    showCreateDialog.value = false
    loadUsers()
  } catch (error) {
    ElMessage.error(error.response?.data?.error || '操作失败')
  }
}

const deleteUser = async (user) => {
  try {
    await ElMessageBox.confirm('确定要删除这个用户吗？', '确认')
    await api.delete(`/users/${user.id}`)
    ElMessage.success('删除成功')
    loadUsers()
  } catch (error) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

onMounted(loadUsers)
</script>

<style scoped>
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
</style>
