import { defineStore } from 'pinia'
import { ref, computed, watch, onScopeDispose } from 'vue'
import { conversationApi } from '../api/user'
import { wsClient } from '../api/ws'
import { useUserStore } from './user'
import { parseConversationID } from '../api/conversation-id.js'

export const useConversationStore = defineStore('conversation', () => {
  const currentChat = ref(null)
  const readyScope = ref('')
  const unreadCounts = ref({}) // Canonical viewer-scoped IDs.
  // Compatibility keys used by the existing contact widgets.
  const unreadMap = computed(() => Object.fromEntries(Object.entries(unreadCounts.value).map(([scope, count]) => {
    const [type, id] = scope.split(':'); return [type === 'group' ? 'g' + id : id, count]
  })))
  const readRequests = new Map()
  const rows = ref([])
  const conversations = computed(() => rows.value.map(item => ({ ...item, unread_count: unreadCounts.value[item.id] || 0 })))
  const conversationError = ref('')
  const draftTimers = new Map()
  const localDrafts = new Map()
  const draftWrites = new Map()
  let conversationRequest = null
  const scopeOf = chat => chat ? `${chat.type}:${chat.id}` : ''
  const currentConversation = computed(() => conversations.value.find(item => item.id === scopeOf(currentChat.value)))
  async function loadConversations() {
    const user = useUserStore(); const token = user.token; const owner = user.myId
    if (!token) return false
    if (conversationRequest?.token === token && conversationRequest?.owner === owner) { conversationRequest.again = true; return conversationRequest.promise }
    const request = { token, owner, again: false, promise: null }
    request.promise = (async () => {
      try {
        let loadedRows; let summary
        do {
          request.again = false; loadedRows = []; let offset = 0
          for (;;) {
            const res = await conversationApi.getList({ limit: 100, offset })
            if (res.code !== 0) throw new Error(res.message)
            if (user.token !== token || user.myId !== owner) return false
            loadedRows.push(...(res.data || [])); if ((res.data || []).length < 100) break
            offset += 100
          }
          const result = await conversationApi.getUnreadCount()
          if (result.code !== 0) throw new Error(result.message)
          if (user.token !== token || user.myId !== owner) return false
          summary = result.data
        } while (request.again)
        if (user.token !== token || user.myId !== owner) return false
        rows.value = loadedRows.map(item => ({ ...item, draft: localDrafts.has(item.id) ? localDrafts.get(item.id) : item.draft }))
        unreadCounts.value = Object.fromEntries((summary?.unread_list || []).filter(item => parseConversationID(item.id)).map(item => [item.id, item.unread_count || 0]))
        if (currentChat.value?.type === 'group' && !loadedRows.some(item => item.id === scopeOf(currentChat.value))) closeChat()
        conversationError.value = ''
        return true
      } catch (error) { if (user.token === token && user.myId === owner) conversationError.value = error.message; return false }
      finally { if (conversationRequest === request) conversationRequest = null }
    })()
    conversationRequest = request
    return request.promise
  }

  function saveDraft(scope, draft) {
    if (!scope) return
    localDrafts.set(scope, draft)
    const item = rows.value.find(item => item.id === scope); if (item) item.draft = draft
    clearTimeout(draftTimers.get(scope))
    const userStore = useUserStore(); const auth = userStore.token; const owner = userStore.myId
    draftTimers.set(scope, setTimeout(async () => {
      draftTimers.delete(scope)
      if (!auth || userStore.token !== auth || userStore.myId !== owner) return
      const previous = draftWrites.get(scope) || Promise.resolve()
      const write = previous.catch(() => {}).then(async () => {
        if (userStore.token !== auth || userStore.myId !== owner) return
        const res = await conversationApi.update(scope, { draft })
        if (res.code !== 0) throw new Error(res.message)
      })
      draftWrites.set(scope, write)
      try { await write }
      catch (error) { if (userStore.token === auth && userStore.myId === owner) conversationError.value = '草稿保存失败：' + error.message }
      finally { if (draftWrites.get(scope) === write) draftWrites.delete(scope) }
    }, 400))
  }

  // ====== 已读处理 ======

  // The scope API does not depend on rounded history IDs or temporary WS IDs.
  // Serialize each scope and repeat if a new message arrives during the request.
  async function markRead(fromId, type = 'private') {
    const key = `${type}:${fromId}`
    const userStore = useUserStore()
    const token = userStore.token
    const userID = userStore.myId
    const existing = readRequests.get(key)
    if (existing && existing.token === token && existing.userID === userID) {
      existing.again = true
      return existing.promise
    }
    const request = { token, userID, again: false, promise: null }
    request.promise = (async () => {
      try {
        do {
          request.again = false
          if (userStore.token !== token || userStore.myId !== userID) return false
          const result = await conversationApi.markRead(`${type}:${fromId}`)
          if (result.code !== 0 || userStore.token !== token || userStore.myId !== userID) return false
        } while (request.again)
        unreadCounts.value = { ...unreadCounts.value, [key]: 0 }
        if (conversationRequest) conversationRequest.again = true
        const item = rows.value.find(item => item.id === `${type}:${fromId}`); if (item) item.unread_count = 0
        return true
      } catch {
        // Keep the unread count on failure; open/reconnect/new messages retry.
        return false
      } finally {
        if (readRequests.get(key) === request) readRequests.delete(key)
      }
    })()
    readRequests.set(key, request)
    return request.promise
  }

  wsClient.on('open', loadConversations)
  wsClient.on('friend_changed', loadConversations)
  wsClient.on('group_changed', loadConversations)


  function onIncoming(data) {
    const key = data.conversation_id
    if (!parseConversationID(key)) return
    unreadCounts.value = { ...unreadCounts.value, [key]: (unreadCounts.value[key] || 0) + 1 }
    if (isVisible(key)) markRead(currentChat.value.id, currentChat.value.type)
    loadConversations()
  }
  function consumeIntro(scope) {
    const item = rows.value.find(item => item.id === scope)
    if (item?.send_policy === 'text_once') { item.send_policy = 'unavailable'; item.intro_remaining = 0 }
  }
  function isVisible(scope) { return scope === scopeOf(currentChat.value) && readyScope.value === scope && (typeof document === 'undefined' || document.visibilityState === 'visible') }
  function closeChat() { currentChat.value = null; readyScope.value = '' }
  const user = useUserStore()
  watch(() => [user.token, user.myId], () => {
    for (const timer of draftTimers.values()) clearTimeout(timer)
    draftTimers.clear(); draftWrites.clear(); localDrafts.clear(); readRequests.clear()
    rows.value = []; unreadCounts.value = {}; conversationError.value = ''; closeChat()
    if (typeof document !== 'undefined') {
    const refresh = () => { if (document.visibilityState === 'visible') loadConversations() }
    document.addEventListener('visibilitychange', refresh)
    onScopeDispose(() => document.removeEventListener('visibilitychange', refresh))
  }
  if (user.token) loadConversations()
  })
  if (typeof document !== 'undefined') {
    const refresh = () => { if (document.visibilityState === 'visible') loadConversations() }
    document.addEventListener('visibilitychange', refresh)
    onScopeDispose(() => document.removeEventListener('visibilitychange', refresh))
  }
  if (user.token) loadConversations()
  return { currentChat, readyScope, unreadCounts, unreadMap, conversations, currentConversation, conversationError, loadConversations, saveDraft, markRead, onIncoming, consumeIntro, closeChat, isVisible }
})
