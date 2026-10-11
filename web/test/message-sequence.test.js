import { test } from 'node:test'
import assert from 'node:assert/strict'
import { wsClient } from '../src/api/ws.js'

test('messages sent in one batch use distinct idempotency keys', () => {
  const messages = []
  const originalSend = wsClient.send
  const originalNow = Date.now
  try {
    Date.now = () => 12345
    wsClient.send = message => messages.push(message)
    wsClient.sendChat(1, 2, 'caption')
    wsClient.sendChat(1, 2, '{"name":"first.pdf"}', 'file')
    wsClient.sendChat(1, 2, '{"name":"second.pdf"}', 'file')
    assert.equal(new Set(messages.map(message => message.seq)).size, 3)
  } finally {
    wsClient.send = originalSend
    Date.now = originalNow
  }
})
