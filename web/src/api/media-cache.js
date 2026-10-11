import { uploadApi } from './upload.js'

// Keep one cache per mounted view/user. clear() is required on logout/token
// changes and unmount; retain() releases media no longer shown by that view.
export function createMediaCache({ onChange = () => {} } = {}) {
  const urls = new Map()
  const pending = new Map()
  function release(path) {
    const url = urls.get(path)
    if (url) URL.revokeObjectURL(url)
    urls.delete(path)
    const operation = pending.get(path)
    pending.delete(path)
    operation?.controller.abort()
  }
  return {
    get: path => urls.get(path) || '',
    load(path) {
      if (urls.has(path)) return Promise.resolve(urls.get(path))
      if (pending.has(path)) return pending.get(path).promise
      const operation = { controller: new AbortController() }
      operation.promise = uploadApi.download(path, { signal: operation.controller.signal }).then(blob => {
        // A request can finish just as the user logs out or leaves the chat.
        if (pending.get(path) !== operation) throw new DOMException('Media loading cancelled', 'AbortError')
        const url = URL.createObjectURL(blob)
        urls.set(path, url)
        onChange()
        return url
      }).finally(() => {
        if (pending.get(path) === operation) pending.delete(path)
      })
      pending.set(path, operation)
      return operation.promise
    },
    retain(paths) {
      const allowed = new Set(paths)
      for (const path of new Set([...urls.keys(), ...pending.keys()])) if (!allowed.has(path)) release(path)
      onChange()
    },
    clear() {
      for (const path of new Set([...urls.keys(), ...pending.keys()])) release(path)
      onChange()
    }
  }
}
