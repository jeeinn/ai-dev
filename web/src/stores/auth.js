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
    userRole: (state) => state.user?.role || 'viewer'
  },

  actions: {
    async login(username, password) {
      const { token, user } = await api.post('/auth/login', { username, password })
      this.token = token
      this.user = user
      localStorage.setItem('token', token)
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

    hasRole(role) {
      return this.user?.role === role
    }
  }
})
