import { test } from 'node:test'
import assert from 'node:assert/strict'
import { createAttachmentDrafts } from '../src/composables/attachment-drafts.js'

const file = name => new File(['attachment'], name, { type: 'application/pdf' })
const meta = { url: '/api/file/one/download', name: 'one.pdf' }

test('upload stays in its conversation until an explicit send; multiple attachments publish once', async () => {
  const published = []
  const drafts = createAttachmentDrafts({ upload: async () => meta })
  await drafts.add('private:2', [file('one.pdf'), file('two.pdf')])
  assert.equal(published.length, 0)
  assert.equal(drafts.list('private:2').length, 2)
  assert.equal(drafts.list('group:2').length, 0)
  assert.ok(drafts.list('private:2').every(item => item.status === 'ready'))
  drafts.send('private:2', (...message) => published.push(message))
  assert.equal(published.length, 2)
  assert.equal(drafts.list('private:2').length, 0)
  drafts.send('private:2', (...message) => published.push(message))
  assert.equal(published.length, 2)
})

test('upload/processing blocks send, preserves progress and resumes failures', async () => {
  let complete, calls = 0, options
  const drafts = createAttachmentDrafts({ upload: async (_file, _type, opts) => {
    options = opts
    if (++calls === 1) { await new Promise(resolve => { complete = resolve }); throw Object.assign(new Error('offline'), { sessionId: 'session-one' }) }
    assert.equal(opts.sessionId, 'session-one')
    return meta
  } })
  const added = drafts.add('private:2', [file('one.pdf')])
  options.onProgress({ loaded: 10, total: 10 })
  assert.equal(drafts.list('private:2')[0].status, 'processing')
  assert.equal(drafts.send('private:2', () => assert.fail('sent before ready')), false)
  complete(); await added
  const item = drafts.list('private:2')[0]
  assert.equal(item.status, 'error')
  assert.equal(item.error, 'offline')
  await drafts.retry('private:2', item.id)
  assert.equal(item.status, 'ready')
})

test('removal aborts upload/session and late completion cannot restore a draft', async () => {
  let complete, options
  const aborted = []
  const drafts = createAttachmentDrafts({ abortSession: id => aborted.push(id), upload: async (_file, _type, opts) => {
    options = opts; opts.onSession('session-one')
    await new Promise(resolve => { complete = resolve }); return meta
  } })
  const added = drafts.add('group:3', [file('one.pdf')])
  drafts.remove('group:3', drafts.list('group:3')[0].id)
  assert.equal(options.signal.aborted, true)
  assert.deepEqual(aborted, ['session-one'])
  complete(); await added
  assert.equal(drafts.list('group:3').length, 0)
})

test('clear on logout cancels every conversation; publication errors preserve pending attachments', async () => {
  const drafts = createAttachmentDrafts({ upload: async () => meta })
  await drafts.add('private:2', [file('one.pdf')])
  await drafts.add('group:3', [file('two.pdf')])
  assert.throws(() => drafts.send('private:2', () => { throw new Error('send failed') }), /send failed/)
  assert.equal(drafts.list('private:2').length, 1)
  drafts.clear()
  assert.equal(drafts.list('private:2').length, 0)
  assert.equal(drafts.list('group:3').length, 0)
})
