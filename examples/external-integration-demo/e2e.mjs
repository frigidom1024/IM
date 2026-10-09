const API_URL = process.env.GOCHAT_API_URL || 'http://localhost:18080';
const EXTERNAL_URL = process.env.EXTERNAL_APP_URL || 'http://localhost:18090';
const WS_URL = process.env.GOCHAT_WS_URL || 'ws://localhost:17000/ws';
const TIMEOUT_MS = Number(process.env.E2E_TIMEOUT_MS || 30000);

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

async function fetchJSON(url, options = {}) {
  const response = await fetch(url, options);
  const text = await response.text();
  let body;
  try {
    body = text ? JSON.parse(text) : null;
  } catch {
    throw new Error(`${url} 返回了非 JSON 响应：${text.slice(0, 200)}`);
  }
  return { response, body };
}

async function waitForHTTP(url) {
  const deadline = Date.now() + TIMEOUT_MS;
  let lastError;
  while (Date.now() < deadline) {
    try {
      const { response } = await fetchJSON(url);
      if (response.ok) return;
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 500));
  }
  throw new Error(`等待服务就绪超时：${url}${lastError ? ` (${lastError.message})` : ''}`);
}

async function externalUser(token) {
  const { response, body } = await fetchJSON(`${EXTERNAL_URL}/api/user/info`, {
    headers: { Authorization: `Bearer ${token}` }
  });
  assert(response.ok, `外部用户 API 认证失败：${response.status}`);
  return body;
}

async function gochat(path, token, options = {}) {
  const { response, body } = await fetchJSON(`${API_URL}${path}`, {
    ...options,
    headers: {
      ...(options.body ? { 'Content-Type': 'application/json' } : {}),
      Authorization: `Bearer ${token}`,
      ...(options.headers || {})
    }
  });
  assert(response.ok, `GoChat 请求失败：${path} HTTP ${response.status}`);
  assert(body && body.code === 0, `GoChat 请求失败：${path} ${JSON.stringify(body)}`);
  return body;
}

class SocketClient {
  constructor(token, label) {
    this.label = label;
    this.messages = [];
    this.waiters = [];
    this.socket = new WebSocket(`${WS_URL}?token=${encodeURIComponent(token)}`);
    this.opened = new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`${label} WebSocket 连接超时`)), 10000);
      this.socket.addEventListener('open', () => {
        clearTimeout(timer);
        resolve();
      });
      this.socket.addEventListener('error', () => {
        clearTimeout(timer);
        reject(new Error(`${label} WebSocket 连接失败`));
      });
    });
    this.socket.addEventListener('message', (event) => {
      const message = JSON.parse(String(event.data));
      this.messages.push(message);
      for (const waiter of [...this.waiters]) {
        if (waiter.predicate(message)) {
          clearTimeout(waiter.timer);
          this.waiters = this.waiters.filter((item) => item !== waiter);
          waiter.resolve(message);
        }
      }
    });
  }

  async waitFor(predicate, timeout = 10000) {
    const existing = this.messages.find(predicate);
    if (existing) return existing;
    return new Promise((resolve, reject) => {
      const waiter = {
        predicate,
        resolve,
        reject,
        timer: setTimeout(() => {
          this.waiters = this.waiters.filter((item) => item !== waiter);
          reject(new Error(`${this.label} 等待消息超时`));
        }, timeout)
      };
      this.waiters.push(waiter);
    });
  }

  close() {
    this.socket.close();
  }
}

async function authenticate(externalToken) {
  const { body } = await fetchJSON(`${API_URL}/api/auth`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${externalToken}` }
  });
  assert(body.code === 0, `外部认证失败：${JSON.stringify(body)}`);
  assert(body.data?.token, '外部认证未返回 GoChat token');
  assert(body.data?.user?.id, '外部认证未返回 GoChat 用户 ID');
  return body.data;
}

async function main() {
  console.log(`检查外部应用：${EXTERNAL_URL}`);
  await waitForHTTP(`${EXTERNAL_URL}/healthz`);
  console.log(`检查 GoChat API：${API_URL}`);
  await waitForHTTP(`${API_URL}/healthz`);

  const externalAlice = await externalUser('demo-alice');
  const externalBob = await externalUser('demo-bob');
  assert(externalAlice.id === 'external-alice', 'Alice 外部用户映射不正确');
  assert(externalBob.id === 'external-bob', 'Bob 外部用户映射不正确');
  console.log('✓ 外部应用用户信息 API 可用');

  const alice = await authenticate('demo-alice');
  const bob = await authenticate('demo-bob');
  const aliceAgain = await authenticate('demo-alice');
  assert(alice.user.id === aliceAgain.user.id, '同一外部用户重复认证没有复用 GoChat 用户');
  assert(alice.user.name === 'Alice', 'Alice 昵称未从外部系统同步');
  assert(bob.user.name === 'Bob', 'Bob 昵称未从外部系统同步');
  console.log(`✓ 外部认证成功：Alice=${alice.user.id}, Bob=${bob.user.id}`);

  const aliceFriends = await gochat('/api/friend/list', alice.token);
  if (!aliceFriends.data.some((friend) => friend.friend_id === bob.user.id)) {
    await gochat('/api/friend/add', alice.token, {
      method: 'POST',
      body: JSON.stringify({ friend_id: bob.user.id, remark: 'E2E Bob' })
    });
  }
  const friendsAfterAdd = await gochat('/api/friend/list', alice.token);
  assert(friendsAfterAdd.data.some((friend) => friend.friend_id === bob.user.id), '好友关系创建失败');
  console.log('✓ 外部认证用户可以调用受保护的好友 API');

  const aliceSocket = new SocketClient(alice.token, 'Alice');
  const bobSocket = new SocketClient(bob.token, 'Bob');
  await Promise.all([aliceSocket.opened, bobSocket.opened]);
  console.log('✓ 两个外部用户均已建立 WebSocket 连接');

  const seq = `e2e-${Date.now()}`;
  const content = `integration message ${seq}`;
  aliceSocket.socket.send(JSON.stringify({
    cmd: 'msg',
    seq,
    data: {
      from_id: alice.user.id,
      to_id: bob.user.id,
      group_id: 0,
      type: 'text',
      content
    }
  }));

  const ack = await aliceSocket.waitFor((message) => message.cmd === 'msg_reply' && message.seq === seq);
  assert(ack.data?.status === 'success', `消息 ACK 失败：${JSON.stringify(ack)}`);
  const delivered = await bobSocket.waitFor((message) => message.cmd === 'msg' && message.data?.content === content);
  assert(delivered.data.from_id === alice.user.id, '消息发送者 ID 不正确');
  console.log('✓ WebSocket 私聊消息已投递');

  const history = await gochat(`/api/message/private/${alice.user.id}`, bob.token);
  assert(history.data.some((message) => message.content === content), '消息未持久化到历史消息接口');
  console.log('✓ 消息可通过历史消息 API 查询');

  aliceSocket.close();
  bobSocket.close();
  console.log('\nE2E integration test passed.');
}

main().catch((error) => {
  console.error(`\nE2E integration test failed: ${error.message}`);
  process.exitCode = 1;
});