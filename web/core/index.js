/**
 * GoChat Core — 前端核心处理库
 *
 * 外部系统可以 import 这些模块自行构建聊天页面，
 * 不必使用内置的 ChatWidget 组件。
 *
 * 使用方式:
 *   import { wsClient, useChat, uploadApi } from '@gochat/core'
 *
 *   // 自己构建 UI，调用这些 API
 *   wsClient.connect(token)
 *   useChat()  // 获取聊天状态
 *   uploadApi.upload(file, 'image')
 */

export { wsClient } from '../src/api/ws'
export { uploadApi } from '../src/api/upload'
export { useChat } from '../src/composables/useChat'
export { useChatStore } from '../src/stores/chat'
export { useUserStore } from '../src/stores/user'

export { default as http } from '../src/api/http'

export { createMediaCache } from '../src/api/media-cache'
export { conversationApi, groupApi, userApi, friendApi, authApi } from '../src/api/user'

export { useConversationStore } from '../src/stores/conversation'
