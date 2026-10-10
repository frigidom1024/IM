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
    unreadMap.value = { ...unreadMap.value, ...map }
  })

  // 实时消息
  wsClient.on('msg', (msg) => {
    const data = msg.data
    if (!data) return

    const userStore = useUserStore()
    const isGroup = data.group_id > 0
    const cacheKey = isGroup ? 'g' + data.group_id : String(data.from_id)
    const chatKey = currentChat.value ? (currentChat.value.type === 'group' ? 'g' + currentChat.value.id : String(currentChat.value.id)) : ''

    const senderName = userStore.friendMap[data.from_id]?.nickname || userStore.friendMap[data.from_id]?.username || ''
    const msgObj = {
      id: Date.now(),
      from_id: data.from_id,
      to_id: data.to_id,
      group_id: data.group_id,
      content: data.content,
      msg_type: data.type,
      created_at: new Date().toISOString(),
      isMine: false,
      sender_name: senderName,
      _temp: true
    }

    if (!messagesCache.value[cacheKey]) {
      messagesCache.value = { ...messagesCache.value, [cacheKey]: [msgObj] }
    } else {
      messagesCache.value = { ...messagesCache.value, [cacheKey]: [...messagesCache.value[cacheKey], msgObj] }
    }

    if (currentChat.value && chatKey === cacheKey) {
      unreadMap.value[cacheKey] = 0
    } else {
      unreadMap.value[cacheKey] = (unreadMap.value[cacheKey] || 0) + 1
    }
  })

  // ====== 已读处理 ======

  // markRead 发送已读回执
  // 取最后一条对方发送的真实消息（非临时、非自己发送）
  // 服务端根据此消息的 from_id 清除未读计数
  function markRead(fromId, type = 'private') {
    const key = type === 'group' ? 'g' + fromId : String(fromId)
    const msgs = messagesCache.value[key]
    if (!msgs || msgs.length === 0) return
    const realMsg = [...msgs].reverse().find(m => !m._temp && !m.isMine)
    if (!realMsg) {
      console.log('[markRead] no received msg found, skip')
      return
    }
    console.log('[markRead] sending id:', realMsg.id)
    wsClient.send({
      cmd: 'msg_read',
      seq: Date.now().toString(),
      data: { message_id: realMsg.id }
    })
  }

  async function openChat(id, name, type = 'private') {
    const key = type === 'group' ? 'g' + id : String(id)
    currentChat.value = { id, name, type }
    unreadMap.value[key] = 0

    try {
      const res = type === 'group' ? await messageApi.getGroup(id) : await messageApi.getPrivate(id)
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
              isMine: msg.from_id === userStore.myId,
              sender_name: msg.sender_name || userStore.friendMap[msg.from_id]?.nickname || userStore.friendMap[msg.from_id]?.username || ''
            })
          }
        }
        merged.sort((a, b) => new Date(a.created_at) - new Date(b.created_at))
        messagesCache.value = { ...messagesCache.value, [key]: merged }
        markRead(id, type)
      }
    } catch (e) {
      console.error('loadMessages:', e)
    }
  }

  function sendMessage(content, msgType = 'text') {
    if (!currentChat.value) return
    if (msgType === 'text' && typeof content === 'string' && !content.trim()) return
    const userStore = useUserStore()
    const chat = currentChat.value
    const key = chat.type === 'group' ? 'g' + chat.id : String(chat.id)

    // 构建消息对象
    const msgObj = {
      id: Date.now(),
      from_id: userStore.myId,
      to_id: chat.id,
      content: msgType === 'text' ? content : JSON.stringify(content),
      msg_type: msgType,
      created_at: new Date().toISOString(),
      isMine: true,
      _temp: true
    }

    // 缓存消息（通过重新赋值触发 Vue 响应式）
    if (!messagesCache.value[key]) {
      messagesCache.value = { ...messagesCache.value, [key]: [msgObj] }
    } else {
      messagesCache.value = { ...messagesCache.value, [key]: [...messagesCache.value[key], msgObj] }
    }

    // 通过 WebSocket 发送（群聊传 groupId）
    const groupId = chat.type === 'group' ? chat.id : 0
    wsClient.sendChat(userStore.myId, chat.id, msgObj.content, msgType, groupId)
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