import { defineStore } from 'pinia'
import api from '../api'

export const useAuthStore = defineStore('auth', {
  state: () => ({
    token: localStorage.getItem('token') || null,
    user: JSON.parse(localStorage.getItem('user') || 'null')
  }),

  getters: {
    isAuthenticated: (state) => !!state.token,
    isAdmin: (state) => state.user?.role === 'admin',
    userRole: (state) => state.user?.role || 'viewer',
    mustChangePassword: (state) => !!state.user?.must_change_password
  },

  actions: {
    async login(username, password) {
      const data = await api.post('/auth/login', { username, password })
      const user = {
        ...data.user,
        must_change_password: !!(data.must_change_password ?? data.user?.must_change_password)
      }
      this.token = data.token
      this.user = user
      localStorage.setItem('token', data.token)
      localStorage.setItem('user', JSON.stringify(user))
      return user
    },

    async logout() {
      try {
        await api.post('/auth/logout')
      } catch (e) {
        // Ignore errors
      }
      this.token = null
      this.user = null
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    },

    async fetchUser() {
      try {
        const user = await api.get('/auth/me')
        this.user = user
        localStorage.setItem('user', JSON.stringify(user))
      } catch (e) {
        this.logout()
      }
    },

    async changePassword(oldPassword, newPassword) {
      const data = await api.put('/auth/password', {
        old_password: oldPassword,
        new_password: newPassword
      })
      if (data?.token) {
        this.token = data.token
        localStorage.setItem('token', data.token)
      }
      const user = data?.user
        ? { ...data.user, must_change_password: false }
        : this.user
          ? { ...this.user, must_change_password: false }
          : null
      if (user) {
        this.user = user
        localStorage.setItem('user', JSON.stringify(user))
      }
    },

    hasRole(role) {
      return this.user?.role === role
    }
  }
})
