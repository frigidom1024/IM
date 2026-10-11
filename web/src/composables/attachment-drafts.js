// Uploads belong to a conversation draft; only an explicit send publishes them.
export function createAttachmentDrafts({ upload, abortSession = () => {}, onChange = () => {} }) {
  const drafts = new Map()
  let nextID = 0
  const live = item => drafts.get(item.scope)?.includes(item)
  const list = scope => drafts.get(scope) || []

  async function start(item) {
    item.controller = new AbortController()
    item.status = 'uploading'
    item.error = ''
    onChange()
    try {
      const meta = await upload(item.file, item.type, {
        signal: item.controller.signal, sessionId: item.sessionId,
        onSession(id) {
          item.sessionId = id
          if (!live(item)) Promise.resolve(abortSession(id)).catch(() => {})
        },
        onProgress({ loaded, total }) {
          if (!live(item)) return
          item.progress = total ? Math.min(100, Math.round(loaded / total * 100)) : 0
          item.status = loaded >= item.file.size ? 'processing' : 'uploading'
          onChange()
        }
      })
      if (!live(item)) return
      item.meta = meta
      item.progress = 100
      item.status = 'ready'
    } catch (error) {
      if (!live(item)) return
      item.sessionId = error.sessionId || item.sessionId
      item.status = 'error'
      item.error = error.message || '上传失败，请重试'
    } finally {
      if (live(item)) onChange()
    }
  }

  function remove(scope, id) {
    const items = list(scope)
    const item = items.find(candidate => candidate.id === id)
    if (!item) return
    const remaining = items.filter(candidate => candidate !== item)
    if (remaining.length) drafts.set(scope, remaining)
    else drafts.delete(scope)
    item.controller.abort()
    if (item.preview) URL.revokeObjectURL(item.preview)
    if (item.sessionId && item.status !== 'ready') Promise.resolve(abortSession(item.sessionId)).catch(() => {})
    onChange()
  }

  return {
    list,
    add(scope, files) {
      const tasks = []
      for (const file of files) {
        const type = file.type.startsWith('image/') ? 'image' : file.type.startsWith('audio/') ? 'voice' : file.type.startsWith('video/') ? 'video' : 'file'
        const item = { id: ++nextID, scope, file, type, status: 'uploading', progress: 0, error: '', preview: type === 'image' ? URL.createObjectURL(file) : '' }
        drafts.set(scope, [...list(scope), item])
        tasks.push(start(item))
      }
      return Promise.all(tasks)
    },
    remove,
    retry(scope, id) {
      const item = list(scope).find(candidate => candidate.id === id)
      if (item?.status === 'error') return start(item)
    },
    send(scope, publish) {
      const items = [...list(scope)]
      if (items.some(item => item.status !== 'ready')) return false
      for (const item of items) {
        publish(item.meta, item.type)
        remove(scope, item.id)
      }
      return true
    },
    clear() {
      for (const [scope, items] of [...drafts]) for (const item of [...items]) remove(scope, item.id)
    }
  }
}
