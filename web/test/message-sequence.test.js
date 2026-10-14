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
    wsClient.sendChat('private:2', 'caption')
    wsClient.sendChat('private:2', '{"name":"first.pdf"}', 'file')
    wsClient.sendChat('private:2', '{"name":"second.pdf"}', 'file')
    assert.equal(new Set(messages.map(message => message.seq)).size, 3)
    assert.deepEqual(messages[0].data, { conversation_id: 'private:2', type: 'text', content: 'caption' })
    wsClient.sendChat('group:3', 'hello', 'text', 'retry-seq')
    assert.equal(messages.at(-1).seq, 'retry-seq')
    assert.equal(messages.at(-1).data.conversation_id, 'group:3')
    assert.throws(() => wsClient.sendChat('private:01', 'bad'), /无效/)
    assert.throws(() => wsClient.sendChat('group:9223372036854775808', 'bad'), /无效/)
  } finally {
    wsClient.send = originalSend
    Date.now = originalNow
  }
})
