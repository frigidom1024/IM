import { defineStore, storeToRefs } from 'pinia'
import { ref, computed, watch, onScopeDispose } from 'vue'
import { conversationApi } from '../api/user'
import { wsClient } from '../api/ws'
import { useUserStore } from './user'
import { useConversationStore } from './conversation'
import { parseConversationID } from '../api/conversation-id.js'

function compareMessages(a, b) {
  const time = new Date(a.created_at) - new Date(b.created_at)
  if (time) return time
  if (/^[1-9][0-9]*$/.test(String(a.id)) && /^[1-9][0-9]*$/.test(String(b.id))) {
    const left = BigInt(a.id), right = BigInt(b.id)
    return left < right ? -1 : left > right ? 1 : 0
  }
  return 0
}

export const useChatStore = defineStore('chat', () => {
  const conversationStore = useConversationStore()
  const { currentChat, conversations, unreadMap, currentConversation, conversationError, readyScope } = storeToRefs(conversationStore)
  const { loadConversations, saveDraft, markRead, closeChat } = conversationStore
  const messagesCache = ref({})
  const pendingSends = new Map()
  const scopeOf = chat => chat ? `${chat.type}:${chat.id}` : ''
  const keyOf = chat => chat.type === 'group' ? 'g' + chat.id : String(chat.id)
  const sendingCurrent = computed(() => currentMessages.value.some(item => item.send_status === 'sending'))

  const currentMessages = computed(() => {
    if (!currentChat.value) return []
    const key = currentChat.value.type === 'group' ? 'g' + currentChat.value.id : String(currentChat.value.id)
    const msgs = messagesCache.value[key] || []
    return [...msgs].sort(compareMessages)
  })

  // ====== WebSocket 事件监听 ======

  // 实时消息
  wsClient.on('msg', (msg) => {
    const data = msg.data
    if (!data) return
    const target = parseConversationID(data.conversation_id)
    if (!target || !data.id || !/^[1-9][0-9]*$/.test(data.sender_id || '')) return

    const userStore = useUserStore()
    const cacheKey = target.key
    if ((messagesCache.value[cacheKey] || []).some(item => String(item.id) === data.id)) return

    const senderName = userStore.profiles[data.sender_id]?.nickname || userStore.profiles[data.sender_id]?.username || ('用户 ' + data.sender_id)
    userStore.loadProfile(data.sender_id)
    const msgObj = {
      id: data.id,
      conversation_id: data.conversation_id,
      from_id: data.sender_id,
      to_id: target.type === 'private' ? userStore.myId : 0,
      group_id: target.type === 'group' ? target.id : 0,
      content: data.content,
      msg_type: data.type,
      created_at: data.created_at || new Date().toISOString(),
      isMine: false,
      sender_name: senderName,
      _temp: false
    }

    if (!messagesCache.value[cacheKey]) {
      messagesCache.value = { ...messagesCache.value, [cacheKey]: [msgObj] }
    } else {
      messagesCache.value = { ...messagesCache.value, [cacheKey]: [...messagesCache.value[cacheKey], msgObj] }
    }

    conversationStore.onIncoming(data)
  })

  function finishSend(msg, status, reason = '') {
    clearTimeout(msg._sendTimer); msg.send_status = status; msg.send_error = reason
    messagesCache.value = { ...messagesCache.value }
  }
  wsClient.on('msg_reply', reply => {
    const pending = pendingSends.get(reply.seq); if (!pending || reply.data?.conversation_id !== pending.scope) return
    pendingSends.delete(reply.seq)
    pending.msg.id = reply.data?.message_id_string || String(reply.data?.message_id || pending.msg.id)
    pending.msg._temp = false; finishSend(pending.msg, 'sent')
    conversationStore.consumeIntro(pending.scope)
    loadConversations()
  })
  wsClient.on('error', reply => {
    const pending = pendingSends.get(reply?.seq); if (!pending) return
    pendingSends.delete(reply.seq); finishSend(pending.msg, 'failed', reply.data?.message || '发送失败')
    loadConversations()
  })

  async function openChat(id, name, type = 'private') {
    const openingUser = useUserStore(); const openingToken = openingUser.token; const openingOwner = openingUser.myId
    const key = type === 'group' ? 'g' + id : String(id)
    currentChat.value = { id, name, type }; readyScope.value = ''

    try {
      const opened = await conversationApi.open(`${type}:${id}`)
      if (openingUser.token !== openingToken || openingUser.myId !== openingOwner) return
      if (opened.code !== 0) throw new Error(opened.message)
      await loadConversations()
      const res = await conversationApi.getMessages(`${type}:${id}`)
      if (openingUser.token !== openingToken || openingUser.myId !== openingOwner) return
      if (res.code !== 0) throw new Error(res.message || '历史消息加载失败')
      if (res.data) {
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
        merged.sort(compareMessages)
        messagesCache.value = { ...messagesCache.value, [key]: merged }
        // A slow history response must not mark a conversation we have left.
        if (currentChat.value?.id === id && currentChat.value?.type === type) { readyScope.value = `${type}:${id}`; if (conversationStore.isVisible(readyScope.value)) await markRead(id, type) }
      }
    } catch (e) {
      if (openingUser.token === openingToken && openingUser.myId === openingOwner) conversationError.value = e.message
    }
  }

  // Reload active history before acknowledging messages missed while offline.
  function reloadActiveHistory() {
    if (!currentChat.value) return
    const { id, name, type } = currentChat.value
    return openChat(id, name, type)
  }
  wsClient.on('close', () => { readyScope.value = '' })
  wsClient.on('open', reloadActiveHistory)
  if (typeof document !== 'undefined') {
    const refresh = () => { if (document.visibilityState === 'visible') reloadActiveHistory() }
    document.addEventListener('visibilitychange', refresh)
    onScopeDispose(() => document.removeEventListener('visibilitychange', refresh))
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

    // Send to the viewer-scoped conversation.
    dispatchMessage(msgObj, chat)
    loadConversations()
    return true
  }

  function dispatchMessage(msg, chat) {
    msg.send_status = 'sending'; msg.send_error = ''
    pendingSends.set(msg.client_seq, { msg, scope: scopeOf(chat) })
    msg._sendTimer = setTimeout(() => finishSend(msg, 'failed', '未收到发送确认，可重试'), 10000)
    wsClient.sendChat(scopeOf(chat), msg.content, msg.msg_type, msg.client_seq)
  }
  function retryMessage(msg) {
    if (msg.send_status !== 'failed' || !currentChat.value) return
    dispatchMessage(msg, currentChat.value); messagesCache.value = { ...messagesCache.value }
  }
  const user = useUserStore()
  watch(() => [user.token, user.myId], () => {
    for (const pending of pendingSends.values()) clearTimeout(pending.msg._sendTimer)
    pendingSends.clear(); messagesCache.value = {}
  })

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
