import { parseConversationID } from './conversation-id.js'

// WebSocket 连接管理
class WsClient {
  constructor() {
    this.ws = null
    this.handlers = {}
    this.reconnectTimer = null
    this.heartbeatTimer = null
    this.pendingToken = null
    this.messageQueue = []
    this.reconnectAttempts = 0
    this.maxReconnectAttempts = 10
    this.reconnectDelay = 1000
    this.heartbeatTimeout = null
  }

  connect(token) {
    this.pendingToken = token
    if (this.ws?.readyState === WebSocket.OPEN) return
    if (this._connecting) return // 防止并发重连
    this._connecting = true
    this.disconnect()

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${proto}//${location.host}/ws?token=${token}`

    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      console.log('[ws] connected')
      this._connecting = false
      this.reconnectAttempts = 0
      this.reconnectDelay = 1000
      this._emit('open')
      this._flushQueue()
      this._startHeartbeat()
    }

    this.ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        console.log('[ws] <<', msg.cmd, msg.seq || '', typeof msg.data === 'object' ? JSON.stringify(msg.data ?? null).slice(0, 200) : msg.data)
        this._emit(msg.cmd, msg)
        this._emit('*', msg)
      } catch (err) {
        console.error('[ws] parse error:', err, e.data.slice(0, 200))
      }
    }

    this.ws.onerror = () => {
      console.warn('[ws] connection error')
      this._emit('error')
    }

    this.ws.onclose = (e) => {
      console.log('[ws] closed:', e.code, e.reason)
      this._connecting = false
      this._emit('close')
      this._stopHeartbeat()
      this._scheduleReconnect()
    }
  }

  send(msg) {
    console.log('[ws] >>', msg.cmd, msg.seq || '', JSON.stringify(msg.data ?? null).slice(0, 200))
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
      return true
    }
    // 离线时入队，最多保留 20 条
    if (this.messageQueue.length < 20) {
      this.messageQueue.push(msg)
    }
    return false
  }

  sendChat(conversationID, content, msgType = 'text', seq = crypto.randomUUID()) {
    if (!parseConversationID(conversationID)) throw new Error('无效的会话标识')
    this.send({
      cmd: 'msg',
      seq,
      data: { conversation_id: conversationID, type: msgType, content }
    })
    return seq
  }

  ping() {
    this.send({ cmd: 'ping', seq: '' })
  }

  on(cmd, handler) {
    if (!this.handlers[cmd]) this.handlers[cmd] = []
    this.handlers[cmd].push(handler)
    return () => this.off(cmd, handler)
  }

  off(cmd, handler) {
    if (!this.handlers[cmd]) return
    this.handlers[cmd] = this.handlers[cmd].filter(h => h !== handler)
  }

  disconnect() {
    clearTimeout(this.reconnectTimer)
    clearTimeout(this.heartbeatTimeout)
    this._stopHeartbeat()
    if (this.ws) {
      this.ws.onclose = null
      this.ws.close()
    }
    this.ws = null
    this.messageQueue = []
  }

  // 清理所有状态（退出登录时调用）
  cleanup() {
    this.disconnect()
    this.reconnectAttempts = this.maxReconnectAttempts // 阻止重连
    this.pendingToken = null
  }

  _flushQueue() {
    if (this.messageQueue.length === 0) return
    const queue = this.messageQueue.splice(0)
    queue.forEach(msg => this.send(msg))
  }

  _scheduleReconnect() {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.warn('[ws] max reconnect attempts reached')
      this._emit('reconnect_failed')
      return
    }
    this.reconnectAttempts++
    const delay = Math.min(this.reconnectDelay, 30000)
    this.reconnectDelay *= 1.5 // 指数退避: 1s → 1.5s → 2.25s → ... → max 30s
    console.log(`[ws] reconnect in ${delay}ms (attempt ${this.reconnectAttempts})`)
    this.reconnectTimer = setTimeout(() => {
      if (this.pendingToken) this.connect(this.pendingToken)
    }, delay)
  }

  _startHeartbeat() {
    this._stopHeartbeat()

    // 收到任何消息视为连接正常，重置超时
    this._offHeartbeatMsg = this.on('*', () => {
      clearTimeout(this.heartbeatTimeout)
      this.heartbeatTimeout = null
    })

    this.heartbeatTimer = setInterval(() => {
      this.ping()
      this.heartbeatTimeout = setTimeout(() => {
        console.warn('[ws] heartbeat timeout, reconnecting...')
        this.ws?.close()
      }, 8000)
    }, 30000)
  }

  _stopHeartbeat() {
    clearInterval(this.heartbeatTimer)
    clearTimeout(this.heartbeatTimeout)
    this.heartbeatTimeout = null
    if (this._offHeartbeatMsg) {
      this._offHeartbeatMsg()
      this._offHeartbeatMsg = null
    }
  }

  _emit(cmd, data) {
    ;(this.handlers[cmd] || []).forEach(h => h(data))
  }
}

export const wsClient = new WsClient()
