// Performance monitoring
performance.mark('chat-init-start');

let conn = null;
let nickname = null;
let reconnectDelay = 1000;
const pendingMessages = new Map();
let historyLoaded = 0; // Track how many history messages we've loaded
let historyLoading = false;

async function connect() {
  try {
    if ('WebTransport' in window) {
      conn = new WebTransport(`https://${location.host}/chat`);
      await conn.ready;
      handleWebTransport(conn);
      setStatus('Connected (WebTransport)');
      reconnectDelay = 1000;
      return;
    }
  } catch (e) {
    console.log('WebTransport failed, trying WebSocket');
  }

  try {
    conn = new WebSocket(`wss://${location.host}/ws`);
    conn.onopen = () => {
      conn.send(`JOIN|${nickname}`);
      setStatus('Connected (WebSocket)');
      reconnectDelay = 1000;
    };
    conn.onmessage = handleMessage;
    conn.onclose = handleDisconnect;
  } catch (e) {
    handleDisconnect();
  }
}

async function handleWebTransport(transport) {
  try {
    const writer = transport.datagrams.writable.getWriter();
    await writer.write(new TextEncoder().encode(`JOIN|${nickname}`));
    writer.releaseLock();

    const reader = transport.datagrams.readable.getReader();
    while (true) {
      const { value, done } = await reader.read();
      if (done) {
        console.log('WebTransport datagram reader closed');
        break;
      }
      const msg = new TextDecoder().decode(value);
      console.log('WebTransport received:', msg);
      handleMessage({ data: msg });
    }
  } catch (e) {
    console.error('WebTransport error:', e);
  } finally {
    handleDisconnect();
  }
}

function handleMessage(event) {
  const parser = new DOMParser();
  const doc = parser.parseFromString(event.data, 'text/html');
  const fragment = doc.body.firstElementChild;

  if (!fragment) return;

  const target = document.querySelector(fragment.dataset.target);
  const action = fragment.dataset.action || 'replace';

  if (!target) return;

  const msgId = fragment.querySelector('[data-id]')?.dataset.id;
  if (msgId && pendingMessages.has(msgId)) {
    const sentTime = pendingMessages.get(msgId);
    const latency = Date.now() - sentTime;
    const latencySpan = document.createElement('span');
    latencySpan.className = 'latency';
    latencySpan.textContent = ` (${latency}ms)`;
    fragment.querySelector('.msg').appendChild(latencySpan);
    pendingMessages.delete(msgId);
  }

  if (action === 'append') {
    target.append(...fragment.childNodes);
    target.scrollTop = target.scrollHeight;
  } else if (action === 'prepend') {
    target.prepend(...fragment.childNodes);
  } else if (action === 'replace') {
    target.replaceWith(fragment);
  }
}

function handleDisconnect() {
  setStatus(`Disconnected. Reconnecting in ${reconnectDelay/1000}s...`);
  setTimeout(() => {
    reconnectDelay = Math.min(reconnectDelay * 2, 30000);
    connect();
  }, reconnectDelay);
}

function setStatus(msg) {
  document.getElementById('connection-status').textContent = msg;
}

function sendMessage() {
  const input = document.getElementById('msg-input');
  const text = input.value.trim();
  if (!text) return;

  const msgId = Date.now();
  pendingMessages.set(msgId.toString(), Date.now());

  const message = `SEND|${nickname}|${text}`;

  if (conn instanceof WebTransport) {
    const writer = conn.datagrams.writable.getWriter();
    writer.write(new TextEncoder().encode(message))
      .then(() => writer.releaseLock());
  } else if (conn instanceof WebSocket) {
    conn.send(message);
  }

  input.value = '';

  setTimeout(() => pendingMessages.delete(msgId.toString()), 5000);
}

document.getElementById('send-btn').onclick = sendMessage;
document.getElementById('msg-input').onkeypress = (e) => {
  if (e.key === 'Enter') sendMessage();
};

// Load more history (in packets of 10)
function loadMoreHistory() {
  if (historyLoading || !conn) return;
  historyLoading = true;

  // Request next 10 messages (skip 10 initial + already loaded)
  const skip = 10 + historyLoaded;
  const msg = `HISTORY|${skip}|10`;

  if (conn instanceof WebTransport) {
    const writer = conn.datagrams.writable.getWriter();
    writer.write(new TextEncoder().encode(msg))
      .then(() => writer.releaseLock());
  } else if (conn instanceof WebSocket) {
    conn.send(msg);
  }

  historyLoaded += 10;
  setTimeout(() => { historyLoading = false; }, 500);
}

// Auto-load history on scroll to top
document.getElementById('messages').onscroll = (e) => {
  if (e.target.scrollTop < 100 && !historyLoading) {
    loadMoreHistory();
  }
};

// Initialize nickname (with defer, DOM is ready)
// Sticky nickname: try to reuse from sessionStorage, fallback to server-assigned (from DOM)
nickname = sessionStorage.getItem('chatNickname');
if (!nickname) {
  // Get server-assigned nickname from the DOM element
  nickname = document.getElementById('nickname').textContent.trim();
  sessionStorage.setItem('chatNickname', nickname);
} else {
  // Update UI with cached nickname
  document.getElementById('nickname').textContent = nickname;
}

connect();

// Load first packet of history after connection
setTimeout(() => {
  loadMoreHistory();
}, 100);

// Performance monitoring - report when fully initialized
performance.mark('chat-init-end');
performance.measure('chat-init', 'chat-init-start', 'chat-init-end');

// Log performance metrics
window.addEventListener('load', () => {
  const chatInit = performance.getEntriesByName('chat-init')[0];
  const navTiming = performance.getEntriesByType('navigation')[0];

  console.log('[Performance Metrics]');
  console.log(`  Chat Init: ${chatInit.duration.toFixed(2)}ms`);
  console.log(`  DOM Content Loaded: ${navTiming.domContentLoadedEventEnd - navTiming.domContentLoadedEventStart}ms`);
  console.log(`  Total Load Time: ${navTiming.loadEventEnd - navTiming.fetchStart}ms`);
  console.log(`  Time to Interactive: ${navTiming.domInteractive - navTiming.fetchStart}ms`);

  // Log resource timing
  const resources = performance.getEntriesByType('resource');
  resources.forEach(r => {
    if (r.name.includes('chat.')) {
      console.log(`  ${r.name.split('/').pop()}: ${r.duration.toFixed(2)}ms (${r.transferSize} bytes)`);
    }
  });
});
