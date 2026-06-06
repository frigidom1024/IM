import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { messageApi } from '../api/user'
import { wsClient } from '../api/ws'
import { useUserStore } from './user'

export const useChatStore = defineStore('chat', () => {
  const currentChat = ref(null) // { id, name, type }
  const messagesCache = ref({})  // { [userId: string]: Message[] }
  const unreadMap = ref({})      // { [userId: string]: count }

  const currentMessages = computed(() => {
    if (!currentChat.value) return []
    const key = String(currentChat.value.id)
    const msgs = messagesCache.value[key] || []
    return [...msgs].sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
  })

  // 监听 WebSocket 收到的消息
  wsClient.on('msg', (msg) => {
    const data = msg.data
    if (!data) return

    const fromId = String(data.from_id)
    const userStore = useUserStore()

    // 构建消息对象
    const msgObj = {
      id: Date.now(),
      from_id: data.from_id,
      to_id: data.to_id,
      content: data.content,
      msg_type: data.type,
      created_at: new Date().toISOString(),
      isMine: false
    }

    // 缓存消息（通过重新赋值触发 Vue 响应式）
    if (!messagesCache.value[fromId]) {
      messagesCache.value = { ...messagesCache.value, [fromId]: [msgObj] }
    } else {
      messagesCache.value = { ...messagesCache.value, [fromId]: [...messagesCache.value[fromId], msgObj] }
    }

    // 如果是当前聊天窗口的消息，Vue 会自动响应
    if (currentChat.value && String(currentChat.value.id) === fromId) {
      // 当前窗口，什么都不做，Vue 自动更新
    } else {
      // 非当前窗口消息，计入未读
      unreadMap.value[fromId] = (unreadMap.value[fromId] || 0) + 1
    }
  })

  async function openChat(id, name, type = 'private') {
    const key = String(id)
    currentChat.value = { id, name, type }
    unreadMap.value[key] = 0

    // 从 API 获取消息并合并缓存
    if (type === 'private') {
      try {
        const res = await messageApi.getPrivate(id)
        if (res.code === 0 && res.data) {
          const userStore = useUserStore()
          // API 返回的消息
          const apiMsgs = res.data || []
          // 已有缓存的消息（来自 WebSocket）
          const cachedMsgs = messagesCache.value[key] || []
          // 合并去重（按 id 或 created_at + content）
          const merged = [...cachedMsgs]
          for (const msg of apiMsgs) {
            const exists = merged.some(m => m.id === msg.id || (m.created_at === msg.created_at && m.content === msg.content))
            if (!exists) {
              merged.push({
                ...msg,
                isMine: msg.from_id === userStore.myId
              })
            }
          }
          // 按时间升序排列
          merged.sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
          messagesCache.value = { ...messagesCache.value, [key]: merged }
        }
      } catch (e) {
        console.error('loadMessages:', e)
        // 保留缓存，不覆盖
      }
    }
  }

  function sendMessage(content) {
    if (!currentChat.value || !content.trim()) return
    const userStore = useUserStore()
    const chat = currentChat.value
    const key = String(chat.id)

    // 构建消息对象
    const msgObj = {
      id: Date.now(),
      from_id: userStore.myId,
      to_id: chat.id,
      content,
      msg_type: 'text',
      created_at: new Date().toISOString(),
      isMine: true
    }

    // 缓存消息（通过重新赋值触发 Vue 响应式）
    if (!messagesCache.value[key]) {
      messagesCache.value = { ...messagesCache.value, [key]: [msgObj] }
    } else {
      messagesCache.value = { ...messagesCache.value, [key]: [...messagesCache.value[key], msgObj] }
    }

    // 通过 WebSocket 发送
    wsClient.sendChat(userStore.myId, chat.id, content)
  }

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
    clearMessages
  }
})
