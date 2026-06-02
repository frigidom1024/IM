import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { authApi, friendApi, userApi } from '../api/user'
import { wsClient } from '../api/ws'

export const useUserStore = defineStore('user', () => {
  const token = ref(localStorage.getItem('token') || '')
  const user = ref(JSON.parse(localStorage.getItem('user') || 'null'))
  const friends = ref([])
  const friendMap = ref({})

  const isLoggedIn = computed(() => !!token.value)
  const myId = computed(() => user.value?.id || 0)
  const myName = computed(() => user.value?.nickname || user.value?.username || '')

  async function login(username, password) {
    const res = await authApi.login({ username, password })
    // res is the response body: { code, message, data: { token, user } }
    if (res.code !== 0) throw new Error(res.message)
    token.value = res.data.token
    user.value = res.data.user
    localStorage.setItem('token', res.data.token)
    localStorage.setItem('user', JSON.stringify(res.data.user))
    wsClient.connect(res.data.token)
    loadFriends()
  }

  async function register(username, password, nickname) {
    const res = await authApi.register({ username, password, nickname })
    if (res.code !== 0) throw new Error(res.message)
  }

  async function loadFriends() {
    try {
      const res = await friendApi.getList()
      if (res.code === 0) {
        friends.value = res.data || []
        // 获取好友用户信息
        const map = {}
        for (const f of friends.value) {
          try {
            const info = await userApi.getInfo(f.friend_id)
            if (info.code === 0 && info.data) {
              map[f.friend_id] = info.data
            }
          } catch {
            map[f.friend_id] = { id: f.friend_id, nickname: '用户 ' + f.friend_id }
          }
        }
        friendMap.value = map
      }
    } catch (e) { console.error('loadFriends:', e) }
  }

  async function addFriend(friendId) {
    const res = await friendApi.add({ friend_id: friendId })
    if (res.code !== 0) throw new Error(res.message)
    await loadFriends()
  }

  function logout() {
    wsClient.disconnect()
    token.value = ''
    user.value = null
    friends.value = []
    friendMap.value = {}
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  }

  // 自动重连
  if (token.value) {
    wsClient.connect(token.value)
    loadFriends()
  }

  return {
    token, user, friends, friendMap,
    isLoggedIn, myId, myName,
    login, register, loadFriends, addFriend, logout
  }
})
