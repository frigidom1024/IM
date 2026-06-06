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

  // ====== WebSocket 事件监听 ======

  // 离线通知：登录后服务端推送的未读汇总
  wsClient.on('offline_notify', (msg) => {
    const list = msg.data?.unread_list
    if (!list) return
    const map = {}
    for (const item of list) {
      if (item.from_id > 0) {
        map[String(item.from_id)] = item.count
      }
    }
    unreadMap.value = { ...unreadMap.value, ...map }
  })

  // 实时消息
  wsClient.on('msg', (msg) => {
    const data = msg.data
    if (!data) return

    const fromId = String(data.from_id)
    const userStore = useUserStore()

    const msgObj = {
      id: Date.now(),
      from_id: data.from_id,
      to_id: data.to_id,
      content: data.content,
      msg_type: data.type,
      created_at: new Date().toISOString(),
      isMine: false
    }

    if (!messagesCache.value[fromId]) {
      messagesCache.value = { ...messagesCache.value, [fromId]: [msgObj] }
    } else {
      messagesCache.value = { ...messagesCache.value, [fromId]: [...messagesCache.value[fromId], msgObj] }
    }

    if (currentChat.value && String(currentChat.value.id) === fromId) {
      // 当前聊天窗口，仅清本地未读（真实 ID 由 openChat 加载 API 后发送）
      unreadMap.value[fromId] = 0
    } else {
      unreadMap.value[fromId] = (unreadMap.value[fromId] || 0) + 1
    }
  })

  // ====== 已读处理 ======

  // markRead 发送已读回执
  function markRead(fromId) {
    const msgs = messagesCache.value[String(fromId)]
    if (!msgs || msgs.length === 0) return
    const lastMsg = msgs[msgs.length - 1]
    wsClient.send({
      cmd: 'msg_read',
      seq: Date.now().toString(),
      data: { message_id: lastMsg.id }
    })
  }

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
          const apiMsgs = res.data || []
          const cachedMsgs = messagesCache.value[key] || []
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
          merged.sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
          messagesCache.value = { ...messagesCache.value, [key]: merged }
          // API 消息加载完后重新发已读（确保拿到真实 message_id）
          markRead(id)
        }
      } catch (e) {
        console.error('loadMessages:', e)
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
    clearMessages,
    markRead
  }
})
