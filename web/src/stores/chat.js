import { defineStore } from 'pinia'
import { ref, computed, watch } from 'vue'
import { messageApi, conversationApi } from '../api/user'
import { wsClient } from '../api/ws'
import { useUserStore } from './user'

export const useChatStore = defineStore('chat', () => {
  const currentChat = ref(null) // { id, name, type }
  const messagesCache = ref({})  // { [userId: string]: Message[] }
  const unreadMap = ref({})      // { [userId: string]: count }
  const readRequests = new Map()
  const conversations = ref([])
  const conversationError = ref('')
  const pendingSends = new Map()
  const draftTimers = new Map()
  const localDrafts = new Map()
  const draftWrites = new Map()
  let conversationRequest = null
  const scopeOf = chat => chat ? `${chat.type}:${chat.id}` : ''
  const keyOf = chat => chat.type === 'group' ? 'g' + chat.id : String(chat.id)
  const currentConversation = computed(() => conversations.value.find(item => item.id === scopeOf(currentChat.value)))
  const sendingCurrent = computed(() => currentMessages.value.some(item => item.send_status === 'sending'))

  async function loadConversations() {
    const user = useUserStore(); const token = user.token
    if (!token) return false
    if (conversationRequest?.token === token) { conversationRequest.again = true; return conversationRequest.promise }
    const request = { token, again: false, promise: null }
    request.promise = (async () => {
      try {
        let rows
        do {
          request.again = false; rows = []; let offset = 0
          for (;;) {
            const res = await conversationApi.getList({ limit: 100, offset })
            if (res.code !== 0) throw new Error(res.message)
            if (user.token !== token) return false
            rows.push(...(res.data || [])); if ((res.data || []).length < 100) break
            offset += 100
          }
        } while (request.again)
        if (user.token !== token) return false
        conversations.value = rows.map(item => ({ ...item, draft: localDrafts.has(item.id) ? localDrafts.get(item.id) : item.draft }))
        const unread = {}
        for (const item of rows) { const [type, id] = item.id.split(':'); unread[type === 'group' ? 'g' + id : id] = item.unread_count || 0 }
        unreadMap.value = unread
        if (currentChat.value?.type === 'group' && !rows.some(item => item.id === scopeOf(currentChat.value))) closeChat()
        conversationError.value = ''
        return true
      } catch (error) { if (user.token === token) conversationError.value = error.message; return false }
      finally { if (conversationRequest === request) conversationRequest = null }
    })()
    conversationRequest = request
    return request.promise
  }

  function saveDraft(scope, draft) {
    if (!scope) return
    localDrafts.set(scope, draft)
    const item = conversations.value.find(item => item.id === scope); if (item) item.draft = draft
    clearTimeout(draftTimers.get(scope))
    const auth = useUserStore().token
    draftTimers.set(scope, setTimeout(async () => {
      draftTimers.delete(scope)
      if (!auth || useUserStore().token !== auth) return
      const previous = draftWrites.get(scope) || Promise.resolve()
      const write = previous.catch(() => {}).then(async () => {
        if (useUserStore().token !== auth) return
        const res = await conversationApi.update(scope, { draft })
        if (res.code !== 0) throw new Error(res.message)
      })
      draftWrites.set(scope, write)
      try { await write }
      catch (error) { if (useUserStore().token === auth) conversationError.value = '草稿保存失败：' + error.message }
      finally { if (draftWrites.get(scope) === write) draftWrites.delete(scope) }
    }, 400))
  }

  const currentMessages = computed(() => {
    if (!currentChat.value) return []
    const key = currentChat.value.type === 'group' ? 'g' + currentChat.value.id : String(currentChat.value.id)
    const msgs = messagesCache.value[key] || []
    return [...msgs].sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
  })

  // ====== WebSocket 事件监听 ======

  // 离线通知：登录后服务端推送的未读汇总
  wsClient.on('offline_notify', (msg) => {
    const list = msg.data?.unread_list
    if (!list) return
    const map = {}
    for (const item of list) {
      const key = item.group_id > 0 ? 'g' + item.group_id : String(item.from_id)
      map[key] = item.count || item.unread_count || 0
    }
    unreadMap.value = map
    if (currentChat.value) markRead(currentChat.value.id, currentChat.value.type)
  })

  wsClient.on('open', () => {
    loadConversations()
    if (currentChat.value) markRead(currentChat.value.id, currentChat.value.type)
  })
  wsClient.on('friend_changed', loadConversations)
  wsClient.on('group_changed', loadConversations)

  // 实时消息
  wsClient.on('msg', (msg) => {
    const data = msg.data
    if (!data) return

    const userStore = useUserStore()
    const isGroup = data.group_id > 0
    const cacheKey = isGroup ? 'g' + data.group_id : String(data.from_id)
    const chatKey = currentChat.value ? (currentChat.value.type === 'group' ? 'g' + currentChat.value.id : String(currentChat.value.id)) : ''

    const senderName = userStore.profiles[data.from_id]?.nickname || userStore.profiles[data.from_id]?.username || ('用户 ' + data.from_id)
    userStore.loadProfile(data.from_id)
    const msgObj = {
      id: data.id || crypto.randomUUID(),
      from_id: data.from_id,
      to_id: data.to_id,
      group_id: data.group_id,
      content: data.content,
      msg_type: data.type,
      created_at: data.created_at || new Date().toISOString(),
      isMine: false,
      sender_name: senderName,
      _temp: true
    }

    if (!messagesCache.value[cacheKey]) {
      messagesCache.value = { ...messagesCache.value, [cacheKey]: [msgObj] }
    } else {
      messagesCache.value = { ...messagesCache.value, [cacheKey]: [...messagesCache.value[cacheKey], msgObj] }
    }

    unreadMap.value[cacheKey] = (unreadMap.value[cacheKey] || 0) + 1
    if (currentChat.value && chatKey === cacheKey) markRead(currentChat.value.id, currentChat.value.type)
    loadConversations()
  })

  function finishSend(msg, status, reason = '') {
    clearTimeout(msg._sendTimer); msg.send_status = status; msg.send_error = reason
    messagesCache.value = { ...messagesCache.value }
  }
  wsClient.on('msg_reply', reply => {
    const pending = pendingSends.get(reply.seq); if (!pending) return
    pendingSends.delete(reply.seq)
    pending.msg.id = reply.data?.message_id_string || String(reply.data?.message_id || pending.msg.id)
    pending.msg._temp = false; finishSend(pending.msg, 'sent')
    const item = conversations.value.find(item => item.id === pending.scope)
    if (item?.send_policy === 'text_once') { item.send_policy = 'unavailable'; item.intro_remaining = 0 }
    loadConversations()
  })
  wsClient.on('error', reply => {
    const pending = pendingSends.get(reply?.seq); if (!pending) return
    pendingSends.delete(reply.seq); finishSend(pending.msg, 'failed', reply.data?.message || '发送失败')
    loadConversations()
  })

  // ====== 已读处理 ======

  // The scope API does not depend on rounded history IDs or temporary WS IDs.
  // Serialize each scope and repeat if a new message arrives during the request.
  async function markRead(fromId, type = 'private') {
    const key = type === 'group' ? 'g' + fromId : String(fromId)
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
          const result = await messageApi.markRead(type === 'group' ? { group_id: fromId } : { from_id: fromId })
          if (result.code !== 0 || userStore.token !== token || userStore.myId !== userID) return false
        } while (request.again)
        unreadMap.value = { ...unreadMap.value, [key]: 0 }
        if (conversationRequest) conversationRequest.again = true
        const item = conversations.value.find(item => item.id === `${type}:${fromId}`); if (item) item.unread_count = 0
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

  async function openChat(id, name, type = 'private') {
    const openingToken = useUserStore().token
    const key = type === 'group' ? 'g' + id : String(id)
    currentChat.value = { id, name, type }

    try {
      const opened = await conversationApi.open(`${type}:${id}`)
      if (useUserStore().token !== openingToken) return
      if (opened.code !== 0) throw new Error(opened.message)
      await loadConversations()
      const res = type === 'group' ? await messageApi.getGroup(id) : await messageApi.getPrivate(id)
      if (useUserStore().token !== openingToken) return
      if (res.code === 0 && res.data) {
        const userStore = useUserStore()
        const apiMsgs = res.data || []
        const cachedMsgs = messagesCache.value[key] || []
        const merged = [...cachedMsgs]
        for (const msg of apiMsgs) {
          const messageID = String(msg.id_string || msg.id)
          const exists = merged.some(m => String(m.id) === messageID)
          if (!exists) {
            merged.push({
              ...msg,
              id: messageID,
              isMine: msg.from_id === userStore.myId,
              sender_name: msg.sender_name || userStore.profiles[msg.from_id]?.nickname || userStore.profiles[msg.from_id]?.username || ('用户 ' + msg.from_id)
            })
          }
        }
        merged.sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
        messagesCache.value = { ...messagesCache.value, [key]: merged }
        // A slow history response must not mark a conversation we have left.
        if (currentChat.value?.id === id && currentChat.value?.type === type) await markRead(id, type)
      }
    } catch (e) {
      if (useUserStore().token === openingToken) conversationError.value = e.message
    }
  }

  function sendMessage(content, msgType = 'text', targetChat = currentChat.value) {
    if (!targetChat) return
    if (msgType === 'text' && typeof content === 'string' && !content.trim()) return
    const userStore = useUserStore()
    const chat = targetChat
    const policy = conversations.value.find(item => item.id === scopeOf(chat))?.send_policy
    if (chat.type !== 'group' && policy !== 'unrestricted' && (policy !== 'text_once' || msgType !== 'text' || (messagesCache.value[keyOf(chat)] || []).some(m => m.send_status === 'sending'))) return false
    const key = chat.type === 'group' ? 'g' + chat.id : String(chat.id)

    // 构建消息对象
    const seq = crypto.randomUUID()
    const msgObj = {
      id: crypto.randomUUID(),
      from_id: userStore.myId,
      to_id: chat.id,
      content: msgType === 'text' ? content : JSON.stringify(content),
      msg_type: msgType,
      created_at: new Date().toISOString(),
      isMine: true,
      _temp: true
      , client_seq: seq, send_status: 'sending'
    }

    // 缓存消息（通过重新赋值触发 Vue 响应式）
    if (!messagesCache.value[key]) {
      messagesCache.value = { ...messagesCache.value, [key]: [msgObj] }
    } else {
      messagesCache.value = { ...messagesCache.value, [key]: [...messagesCache.value[key], msgObj] }
    }

    // 通过 WebSocket 发送（群聊传 groupId）
    const groupId = chat.type === 'group' ? chat.id : 0
    dispatchMessage(msgObj, chat, groupId)
    loadConversations()
    return true
  }

  function dispatchMessage(msg, chat, groupId = chat.type === 'group' ? chat.id : 0) {
    msg.send_status = 'sending'; msg.send_error = ''
    pendingSends.set(msg.client_seq, { msg, scope: scopeOf(chat) })
    msg._sendTimer = setTimeout(() => finishSend(msg, 'failed', '未收到发送确认，可重试'), 10000)
    wsClient.sendChat(useUserStore().myId, chat.id, msg.content, msg.msg_type, groupId, msg.client_seq)
  }
  function retryMessage(msg) {
    if (msg.send_status !== 'failed' || !currentChat.value) return
    dispatchMessage(msg, currentChat.value); messagesCache.value = { ...messagesCache.value }
  }
  const user = useUserStore()
  watch(() => user.token, token => {
    for (const pending of pendingSends.values()) clearTimeout(pending.msg._sendTimer)
    pendingSends.clear(); for (const timer of draftTimers.values()) clearTimeout(timer)
    draftTimers.clear(); draftWrites.clear(); localDrafts.clear(); conversations.value = []; unreadMap.value = {}; messagesCache.value = {}; currentChat.value = null; readRequests.clear()
    if (token) loadConversations()
  })
  if (user.token) loadConversations()

  function closeChat() {
    currentChat.value = null
  }

  function getUnreadCount(id) {
    return unreadMap.value[String(id)] || 0
  }

  function clearMessages(id) {
    delete messagesCache.value[String(id)]
  }

  return {
    currentChat,
    messagesCache,
    unreadMap,
    currentMessages,
    openChat,
    sendMessage,
    closeChat,
    getUnreadCount,
    clearMessages,
    markRead
    , conversations, currentConversation, conversationError, loadConversations, sendingCurrent, retryMessage, saveDraft
  }
})
