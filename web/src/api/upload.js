import http from './http.js'

// Only send credentials to the supported private media routes. Old storage
// object names and arbitrary external URLs have no supported download route.
function mediaPath(value) {
  if (typeof value !== 'string' || !/^\/(?:api\/)?file\/[A-Za-z0-9_-]+\/(?:download|thumbnail)$/.test(value)) {
    throw new Error('文件地址无效或已不再支持，请重新上传')
  }
  return value.replace(/^\/api/, '')
}

function data(response) {
  if (response.code !== 0) throw new Error(response.message || '文件操作失败')
  return response.data
}

export const uploadApi = {
  upload(file, type, { onProgress, signal, groupId } = {}) {
    const form = new FormData()
    form.append('file', file)
    form.append('type', type)
    if (groupId) form.append('group_id', groupId)
    return http.post('/upload', form, {
      timeout: 0, signal, onUploadProgress: onProgress
    }).then(data)
  },
  async download(url, { signal } = {}) {
    const path = mediaPath(url)
    try {
      const response = await http.get(path, { responseType: 'arraybuffer', timeout: 0, signal })
      return new Blob([response.data], { type: response.headers['content-type'] || 'application/octet-stream' })
    } catch (error) {
      if (error.response?.data) {
        try {
          const envelope = JSON.parse(new TextDecoder().decode(error.response.data))
          if (envelope.message) error.message = envelope.message
        } catch { /* Preserve transport errors and non-JSON responses. */ }
      }
      throw error
    }
  },
  createSession: (declaration, { signal } = {}) => http.post('/upload/session', declaration, { signal }).then(data),
  getSession: (id, { signal } = {}) => http.get(`/upload/session/${encodeURIComponent(id)}`, { signal }).then(data),
  appendSession: (id, offset, chunk, { signal, onProgress } = {}) => http.patch(`/upload/session/${encodeURIComponent(id)}`, chunk, {
    headers: { 'Content-Type': 'application/octet-stream', 'Upload-Offset': String(offset) },
    timeout: 0, signal, onUploadProgress: onProgress
  }).then(data),
  completeSession: (id, { signal } = {}) => http.post(`/upload/session/${encodeURIComponent(id)}/complete`, null, { timeout: 0, signal }).then(data),
  abortSession: id => http.delete(`/upload/session/${encodeURIComponent(id)}`).then(data),
  async uploadResumable(file, type, { sessionId, groupId, expiresHours, onProgress, onSession, signal } = {}) {
    let session
    try {
      session = sessionId ? await this.getSession(sessionId, { signal }) : await this.createSession({ name: file.name, type, mime: file.type || 'application/octet-stream', size: file.size, group_id: groupId, expires_hours: expiresHours }, { signal })
      if (onSession) onSession(session.id)
      if (session.size !== file.size || !Number.isSafeInteger(session.offset) || session.offset < 0 || session.offset > file.size) throw new Error('上传会话与文件不匹配')
      if (onProgress) onProgress({ loaded: session.offset, total: file.size })
      while (session.offset < file.size) {
        signal?.throwIfAborted()
        const end = Math.min(session.offset + 8 * 1024 * 1024, file.size)
        const next = await this.appendSession(session.id, session.offset, file.slice(session.offset, end), { signal })
        if (next.id !== session.id || next.size !== file.size || next.offset !== end) throw new Error('服务器返回的上传偏移无效')
        session = next
        if (onProgress) onProgress({ loaded: session.offset, total: file.size })
      }
      return await this.completeSession(session.id, { signal })
    } catch (error) {
      if (session?.id) error.sessionId = session.id
      throw error
    }
  }
}
