import { ref, onMounted, onUnmounted } from 'vue'
import { wsClient } from '../api/ws'
import { useChatStore } from '../stores/chat'
import { useUserStore } from '../stores/user'

export function useChat(token) {
  const connected = ref(false)
  const chatStore = useChatStore()
  const userStore = useUserStore()

  const currentChat = chatStore.currentChat
  const messages = chatStore.currentMessages
  const unreadMap = chatStore.unreadMap

  function connect() {
    wsClient.connect(token)
  }

  function disconnect() {
    wsClient.disconnect()
  }

  function openChat(id, name, type = 'private') {
    chatStore.openChat(id, name, type)
  }

  function sendMessage(content, msgType = 'text') {
    chatStore.sendMessage(content, msgType)
  }

  onMounted(() => {
    const offOpen = wsClient.on('open', () => { connected.value = true })
    const offClose = wsClient.on('close', () => { connected.value = false })
    connect()

    onUnmounted(() => {
      offOpen()
      offClose()
      disconnect()
    })
  })

  return {
    connected, currentChat, messages, unreadMap,
    friends: userStore.friends,
    friendMap: userStore.friendMap,
    myName: userStore.myName,
    myId: userStore.myId,
    openChat, sendMessage
  }
}
