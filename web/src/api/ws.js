// WebSocket 连接管理
class WsClient {
  constructor() {
    this.ws = null
    this.handlers = {}
    this.reconnectTimer = null
    this.heartbeatTimer = null
  }

  connect(token) {
    if (this.ws?.readyState === WebSocket.OPEN) return
    this.disconnect()

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:'
    const url = `${proto}//${location.host}/ws?token=${token}`

    this.ws = new WebSocket(url)

    this.ws.onopen = () => {
      console.log('[ws] connected')
      this._emit('open')
      this._startHeartbeat()
    }

    this.ws.onmessage = (e) => {
      try {
        const msg = JSON.parse(e.data)
        this._emit(msg.cmd, msg)
        this._emit('*', msg)
      } catch (err) {
        console.error('[ws] parse error:', err)
      }
    }

    this.ws.onerror = () => this._emit('error')

    this.ws.onclose = () => {
      console.log('[ws] closed')
      this._emit('close')
      this._stopHeartbeat()
      this.reconnectTimer = setTimeout(() => this.connect(token), 3000)
    }
  }

  send(msg) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
    }
  }

  sendChat(fromId, toId, content, msgType = 1) {
    this.send({
      cmd: 'msg',
      seq: Date.now().toString(),
      data: { from_id: fromId, to_id: toId, group_id: 0, type: msgType, content }
    })
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
    this._stopHeartbeat()
    if (this.ws) {
      this.ws.onclose = null
      this.ws.close()
    }
    this.ws = null
  }

  _emit(cmd, data) {
    ;(this.handlers[cmd] || []).forEach(h => h(data))
  }

  _startHeartbeat() {
    this._stopHeartbeat()
    this.heartbeatTimer = setInterval(() => this.ping(), 30000)
  }

  _stopHeartbeat() {
    clearInterval(this.heartbeatTimer)
  }
}

export const wsClient = new WsClient()
