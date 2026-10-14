import { defineStore } from 'pinia'
import { ref, computed, onScopeDispose } from 'vue'
import { authApi, friendApi, userApi, groupApi } from '../api/user'
import { wsClient } from '../api/ws'

export const useUserStore = defineStore('user', () => {
  const token = ref(localStorage.getItem('token') || '')
  const user = ref(JSON.parse(localStorage.getItem('user') || 'null'))
  const friends = ref([])
  const friendMap = ref({})
  const groups = ref([])
  const profiles = ref({})
  const incomingFriends = ref([])
  const myApplications = ref([])
  const relationError = ref('')
  const friendNoticeCount = computed(() => incomingFriends.value.filter(item => item.unread).length)
  const pendingGroupCount = computed(() => groups.value.reduce((n, group) => n + (group.pending_count || 0), 0))

  async function loadProfile(id) {
    if (profiles.value[id]) return profiles.value[id]
    const tokenAtStart = token.value
    try {
      const result = await userApi.getInfo(id)
      if (result.code === 0 && token.value === tokenAtStart) profiles.value = { ...profiles.value, [id]: result.data }
    } catch {}
    return profiles.value[id]
  }

  async function loadNotices() {
    const auth = token.value
    try {
      const res = await friendApi.getIncoming()
      if (res.code === 0 && token.value === auth) incomingFriends.value = res.data || []
    } catch (error) { relationError.value = error.message }
  }
  async function loadApplications() {
    const auth = token.value
    try {
      const rows = []; let offset = 0
      for (;;) {
        const res = await groupApi.getMyApplications({ limit: 100, offset })
        if (res.code !== 0 || token.value !== auth) return
        rows.push(...(res.data || [])); if ((res.data || []).length < 100) break
        offset += 100
      }
      if (token.value === auth) myApplications.value = rows
    } catch (error) { relationError.value = error.message }
  }
  async function refreshRelations() {
    if (!token.value) return
    relationError.value = ''
    await Promise.all([loadFriends(), loadGroups(), loadNotices(), loadApplications()])
  }
  wsClient.on('friend_changed', () => { if (token.value) { loadFriends(); loadNotices() } })
  wsClient.on('group_changed', () => { if (token.value) { loadGroups(); loadApplications() } })
  wsClient.on('open', refreshRelations)
  if (typeof document !== 'undefined') {
    const refresh = () => { if (document.visibilityState === 'visible') refreshRelations() }
    document.addEventListener('visibilitychange', refresh)
    onScopeDispose(() => document.removeEventListener('visibilitychange', refresh))
  }

  const isLoggedIn = computed(() => !!token.value)
  const myId = computed(() => user.value?.id || 0)
  const myName = computed(() => user.value?.nickname || user.value?.username || '')

  async function login(username, password) {
    const res = await authApi.login({ username, password })
    if (res.code !== 0) throw new Error(res.message)
    token.value = res.data.token
    user.value = res.data.user
    localStorage.setItem('token', res.data.token)
    localStorage.setItem('user', JSON.stringify(res.data.user))
    wsClient.connect(res.data.token)
    refreshRelations()
  }

  async function externalAuth(externalToken) {
    const http = (await import('../api/http')).default
    const res = await http.post('/auth', null, {
      headers: { Authorization: externalToken }
    })
    if (res.code !== 0) throw new Error(res.message)
    token.value = res.data.token
    user.value = res.data.user
    localStorage.setItem('token', res.data.token)
    localStorage.setItem('user', JSON.stringify(res.data.user))
    wsClient.connect(res.data.token)
    refreshRelations()
  }

  async function register(username, password, nickname) {
    const res = await authApi.register({ username, password, nickname })
    if (res.code !== 0) throw new Error(res.message)
  }

  async function loadFriends() {
    const auth = token.value
    try {
      const res = await friendApi.getList()
      if (res.code === 0 && token.value === auth) {
        friends.value = res.data || []
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
        if (token.value === auth) { friendMap.value = map; profiles.value = { ...profiles.value, ...map } }
      }
    } catch (e) { if (token.value === auth) relationError.value = e.message }
  }

  // ====== 群组 ======

  async function loadGroups() {
    const auth = token.value
    try {
      const res = await groupApi.getList()
      if (res.code === 0 && token.value === auth) groups.value = res.data || []
    } catch (e) { if (token.value === auth) relationError.value = e.message }
  }

  async function createGroup(name, maxMembers = 200) {
    const res = await groupApi.create({ name, max_members: maxMembers })
    if (res.code !== 0) throw new Error(res.message)
    await loadGroups()
    return res.data
  }

  async function joinGroup(groupId) {
    const res = await groupApi.join(groupId)
    if (res.code !== 0) throw new Error(res.message)
    await loadApplications()
    return res.data
  }

  async function leaveGroup(groupId) {
    const res = await groupApi.leave(groupId)
    if (res.code !== 0) throw new Error(res.message)
    await loadGroups()
  }

  async function getGroupMembers(groupId) {
    const res = await groupApi.getMembers(groupId)
    if (res.code !== 0) throw new Error(res.message)
    return res.data || []
  }

  async function removeMember(groupId, userId) {
    const res = await groupApi.removeMember(groupId, userId)
    if (res.code !== 0) throw new Error(res.message)
  }

  async function addFriend(friendId) {
    const res = await friendApi.add({ friend_id: friendId })
    if (res.code !== 0) throw new Error(res.message)
    await Promise.all([loadFriends(), loadNotices()])
  }

  async function removeFriend(id) {
    const res = await friendApi.remove(id)
    if (res.code !== 0) throw new Error(res.message)
    await Promise.all([loadFriends(), loadNotices()])
  }
  async function readFriendNotice(id) {
    const res = await friendApi.readNotice(id)
    if (res.code !== 0) throw new Error(res.message)
    await loadNotices()
  }

  function logout() {
    wsClient.cleanup()
    token.value = ''
    user.value = null
    friends.value = []
    friendMap.value = {}
    groups.value = []
    profiles.value = {}; incomingFriends.value = []; myApplications.value = []; relationError.value = ''
    localStorage.removeItem('token')
    localStorage.removeItem('user')
  }

  if (token.value) {
    wsClient.connect(token.value)
    refreshRelations()
  }

  return {
    token, user, friends, friendMap, groups,
    isLoggedIn, myId, myName,
    login, register, externalAuth, loadFriends, addFriend, logout,
    loadGroups, createGroup, joinGroup, leaveGroup,
    getGroupMembers, removeMember
    , profiles, loadProfile, incomingFriends, myApplications, friendNoticeCount, pendingGroupCount,
    loadNotices, loadApplications, refreshRelations, removeFriend, readFriendNotice, relationError
  }
})
