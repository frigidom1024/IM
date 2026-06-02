import http from './http'

export const uploadApi = {
  /**
   * 上传文件
   * @param {File} file - 文件对象
   * @param {string} type - 消息类型 (image/file/voice/video)
   * @returns {Promise<{url, name, size, mime}>} FileMeta
   */
  upload(file, type) {
    const form = new FormData()
    form.append('file', file)
    form.append('type', type)
    return http.post('/upload', form, {
      headers: { 'Content-Type': 'multipart/form-data' }
    }).then(res => res.data)
  }
}
