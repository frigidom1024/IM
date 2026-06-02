import { defineStore } from 'pinia'
import { ref, watch } from 'vue'
import { messageApi } from '../api/user'
import { wsClient } from '../api/ws'
import { useUserStore } from './user'

export const useChatStore = defineStore('chat', () => {
  const currentChat = ref(null) // { id, name, type }
  const messages = ref([])
  const unreadMap = ref({})

  // 监听 WebSocket 收到的消息
  const offWs = wsClient.on('msg', (msg) => {
    const data = msg.data
    if (!data) return

    const fromId = data.from_id
    const userStore = useUserStore()

    if (currentChat.value && fromId === currentChat.value.id) {
      // 当前聊天窗口的消息，直接显示
      messages.value.push({
        id: Date.now(),
        from_id: fromId,
        to_id: data.to_id,
        content: data.content,
        msg_type: data.type,
        created_at: new Date().toISOString(),
        isMine: false
      })
    } else {
      // 非当前窗口消息，计入未读
      unreadMap.value = { ...unreadMap.value, [fromId]: (unreadMap.value[fromId] || 0) + 1 }
    }
  })

  async function openChat(id, name, type = 'private') {
    currentChat.value = { id, name, type }
    messages.value = []
    unreadMap.value = { ...unreadMap.value, [id]: 0 }

    if (type === 'private') {
      try {
        const res = await messageApi.getPrivate(id)
        const d = res.data
        if (d.code === 0 && d.data) {
          const userStore = useUserStore()
          messages.value = (d.data || []).map(m => ({
            ...m,
            isMine: m.from_id === userStore.myId
          }))
        }
      } catch (e) { console.error('loadMessages:', e) }
    }
  }

  function sendMessage(content) {
    if (!currentChat.value || !content.trim()) return
    const userStore = useUserStore()
    const chat = currentChat.value

    // 本地先显示
    messages.value.push({
      id: Date.now(),
      from_id: userStore.myId,
      to_id: chat.id,
      content,
      msg_type: 1,
      created_at: new Date().toISOString(),
      isMine: true
    })

    // 通过 WebSocket 发送
    wsClient.sendChat(userStore.myId, chat.id, content)
  }

  function closeChat() {
    currentChat.value = null
    messages.value = []
  }

  return { currentChat, messages, unreadMap, openChat, sendMessage, closeChat }
})
