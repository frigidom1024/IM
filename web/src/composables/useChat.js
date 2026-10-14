import { ref, onMounted, onUnmounted } from 'vue'
import { storeToRefs } from 'pinia'
import { wsClient } from '../api/ws'
import { useChatStore } from '../stores/chat'
import { useUserStore } from '../stores/user'
import { useConversationStore } from '../stores/conversation'

export function useChat(token) {
  const connected = ref(false)
  const chatStore = useChatStore()
  const userStore = useUserStore()

  const { currentChat, currentMessages: messages } = storeToRefs(chatStore)
  const { unreadMap, conversations, currentConversation, conversationError } = storeToRefs(useConversationStore())
  const { friends, friendMap, myName, myId } = storeToRefs(userStore)

  function connect() {
    wsClient.connect(token)
  }

  function disconnect() {
    wsClient.disconnect()
  }

  function openChat(id, name, type = 'private') {
    return chatStore.openChat(id, name, type)
  }

  function sendMessage(content, msgType = 'text') {
    return chatStore.sendMessage(content, msgType)
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
    conversations, currentConversation, conversationError,
    friends, friendMap, myName, myId,
    openChat, sendMessage, connect, disconnect
  }
}
