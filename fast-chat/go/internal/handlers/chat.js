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
      // Reuse early connection if available
      if (window.__earlyWT) {
        conn = window.__earlyWT;
        delete window.__earlyWT;
      } else {
        conn = new WebTransport(`https://${location.host}/chat`);
        await conn.ready;
      }
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
  console.log('Received raw message:', event.data);

  let msg;
  try {
    msg = JSON.parse(event.data);
    console.log('Parsed message:', msg);
  } catch (e) {
    console.error('Failed to parse message:', e);
    console.error('Raw data:', event.data);
    setStatus('⚠️ Received malformed message');
    return;
  }

  if (!msg || !msg.type) {
    console.error('Invalid message structure:', msg);
    setStatus('⚠️ Invalid message received');
    return;
  }

  const messagesEl = document.getElementById('messages');
  const userCountEl = document.getElementById('user-count');

  switch (msg.type) {
    case 'error':
      console.error('Server error:', msg.data);
      setStatus(`❌ Error: ${msg.data}`);
      return;
    case 'message':
      const msgDiv = document.createElement('div');
      msgDiv.className = msg.data.isSystem ? 'sys' : 'msg';

      if (!msg.data.isSystem) {
        msgDiv.dataset.id = msg.data.id;

        let html = `<span class="nick">${msg.data.nickname}</span><span class="text">${msg.data.text}</span><time>${msg.data.time}</time>`;
        
        if (msg.data.clientId && pendingMessages.has(msg.data.clientId)) {
          const sentTime = pendingMessages.get(msg.data.clientId);
          const latency = Date.now() - sentTime;
          html += `<span class="latency"> (${latency}ms)</span>`;
          pendingMessages.delete(msg.data.clientId);
        }
        
        msgDiv.innerHTML = html;
      } else {
        msgDiv.textContent = msg.data.text;
      }

      if (msg.action === 'prepend') {
        messagesEl.prepend(msgDiv);
      } else {
        messagesEl.appendChild(msgDiv);
        messagesEl.scrollTop = messagesEl.scrollHeight;
      }
      break;

    case 'history':
      // Handle batch of history messages
      if (!msg.data || !Array.isArray(msg.data)) return;

      // Create document fragment for batch insert
      const fragment = document.createDocumentFragment();
      for (const msgData of msg.data) {
        const msgDiv = document.createElement('div');
        msgDiv.className = 'msg';
        msgDiv.dataset.id = msgData.id;
        
        // Batch write with innerHTML
        msgDiv.innerHTML = `<span class="nick">${msgData.nickname}</span><span class="text">${msgData.text}</span><time>${msgData.time}</time>`;
        
        fragment.appendChild(msgDiv);
      }

      messagesEl.prepend(fragment);
      break;

    case 'usercount':
      userCountEl.textContent = msg.data;
      break;
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

  const clientId = Date.now().toString();
  const message = `SEND|${nickname}|${text}|${clientId}`;

  if (conn instanceof WebTransport) {
    const writer = conn.datagrams.writable.getWriter();
    writer.write(new TextEncoder().encode(message))
      .then(() => writer.releaseLock());
  } else if (conn instanceof WebSocket) {
    conn.send(message);
  }

  pendingMessages.set(clientId, Date.now());

  input.value = '';

  setTimeout(() => pendingMessages.delete(clientId), 5000);
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

// Auto-load history on scroll to top (debounced)
let scrollTimeout;
document.getElementById('messages').onscroll = (e) => {
  if (scrollTimeout) return;
  if (e.target.scrollTop < 100 && !historyLoading) {
    scrollTimeout = setTimeout(() => {
      loadMoreHistory();
      scrollTimeout = null;
    }, 50);
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

// Load first packet of history immediately (no delay!)
// Wait for connection to be established first
const checkConnectionAndLoadHistory = setInterval(() => {
  if (conn) {
    loadMoreHistory();
    clearInterval(checkConnectionAndLoadHistory);
  }
}, 10); // Check every 10ms instead of waiting 100ms

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
