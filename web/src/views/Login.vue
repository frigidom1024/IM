<template>
  <div class="auth-overlay">
    <div class="auth-card">
      <div class="auth-logo">go<span>chat</span></div>

      <div class="auth-tabs">
        <button :class="['auth-tab', { active: mode === 'login' }]" @click="mode = 'login'">登录</button>
        <button :class="['auth-tab', { active: mode === 'register' }]" @click="mode = 'register'">注册</button>
      </div>

      <div class="field">
        <label>用户名</label>
        <input v-model="form.username" placeholder="username" @keydown.enter="submit">
      </div>

      <div class="field">
        <label>密码</label>
        <input v-model="form.password" type="password" placeholder="password" @keydown.enter="submit">
      </div>

      <div v-if="mode === 'register'" class="field">
        <label>昵称</label>
        <input v-model="form.nickname" placeholder="nickname" @keydown.enter="submit">
      </div>

      <button class="auth-btn" @click="submit" :disabled="loading">
        {{ loading ? '...' : (mode === 'login' ? '登录' : '注册') }}
      </button>

      <div class="auth-error">{{ error }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive } from 'vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'

const router = useRouter()
const userStore = useUserStore()

const mode = ref('login')
const loading = ref(false)
const error = ref('')
const form = reactive({ username: '', password: '', nickname: '' })

async function submit() {
  error.value = ''
  if (!form.username || !form.password) {
    error.value = '请输入用户名和密码'
    return
  }

  loading.value = true
  try {
    if (mode.value === 'register') {
      await userStore.register(form.username, form.password, form.nickname || form.username)
      mode.value = 'login'
      error.value = '注册成功，请登录'
    } else {
      console.log('开始登录...')
      await userStore.login(form.username, form.password)
      console.log('登录成功，跳转...', userStore.token, userStore.user)
      window.location.href = '/'
    }
  } catch (e) {
    console.error('登录失败:', e)
    error.value = e.message
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.auth-overlay {
  position: fixed;
  inset: 0;
  background: var(--bg);
  display: flex;
  align-items: center;
  justify-content: center;
}

.auth-card {
  width: 340px;
}

.auth-logo {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 28px;
  font-weight: 600;
  letter-spacing: -1px;
  margin-bottom: 36px;
}
.auth-logo span { color: var(--accent); }

.auth-tabs {
  display: flex;
  border-bottom: 1px solid var(--border);
  margin-bottom: 24px;
}

.auth-tab {
  flex: 1;
  padding: 10px 0;
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  color: var(--text-3);
  font-family: 'IBM Plex Mono', monospace;
  font-size: 13px;
  cursor: pointer;
  transition: all .2s;
}
.auth-tab.active {
  color: var(--text);
  border-bottom-color: var(--accent);
}

.field { margin-bottom: 14px; }

.field label {
  display: block;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 11px;
  color: var(--text-3);
  margin-bottom: 5px;
  text-transform: uppercase;
  letter-spacing: .5px;
}

.field input {
  width: 100%;
  padding: 10px 12px;
  background: var(--bg-2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  color: var(--text);
  font-size: 14px;
  transition: border-color .2s;
}
.field input:focus { outline: none; border-color: var(--accent); }

.auth-btn {
  width: 100%;
  padding: 11px;
  margin-top: 6px;
  background: var(--accent);
  border: none;
  border-radius: var(--radius);
  color: #fff;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
  transition: background .2s;
}
.auth-btn:hover { background: var(--accent-2); }
.auth-btn:disabled { opacity: .6; cursor: not-allowed; }

.auth-error {
  color: var(--accent);
  font-size: 13px;
  margin-top: 12px;
  min-height: 20px;
}
</style>
