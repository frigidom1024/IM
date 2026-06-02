import axios from 'axios'

const http = axios.create({
  baseURL: '/api',
  timeout: 10000
})

// 请求拦截器：自动带上token
http.interceptors.request.use(config => {
  const token = localStorage.getItem('token')
  console.log('请求拦截器 - Token:', token)
  if (token) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// 响应拦截器：401跳转登录
http.interceptors.response.use(
  res => res.data,
  err => {
    if (err.response?.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      location.href = '/login'
    }
    return Promise.reject(err)
  }
)

export default http
