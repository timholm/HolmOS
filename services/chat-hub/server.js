const express = require('express');
const http = require('http');
const WebSocket = require('ws');

const app = express();
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

app.use(express.json());

const STEVE_URL = process.env.STEVE_URL || 'http://steve-bot.holm.svc.cluster.local:8080';
const ALICE_URL = process.env.ALICE_URL || 'http://alice-bot.holm.svc.cluster.local:8080';

// HTML UI - Steve & Alice Group Chat
const chatHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Steve & Alice - AI Conversation</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: #1e1e2e;
      color: #cdd6f4;
      height: 100vh;
      display: flex;
      flex-direction: column;
    }

    /* Header */
    .header {
      padding: 16px 24px;
      background: linear-gradient(135deg, #1a1a2e 0%, #2d1f3d 100%);
      border-bottom: 1px solid #313244;
      display: flex;
      align-items: center;
      gap: 16px;
    }
    .header-avatars {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .avatar {
      width: 40px;
      height: 40px;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: bold;
      font-size: 16px;
    }
    .avatar.steve { background: #1a1a2e; color: #89b4fa; border: 2px solid #89b4fa; }
    .avatar.alice { background: #ff6b9d; color: #1e1e2e; }
    .header-vs { color: #6c7086; font-size: 20px; }
    .header-info h1 { font-size: 20px; font-weight: 600; }
    .header-info p { font-size: 12px; color: #6c7086; margin-top: 2px; }
    .live-badge {
      margin-left: auto;
      display: flex;
      align-items: center;
      gap: 6px;
      font-size: 12px;
      color: #a6e3a1;
    }
    .live-dot {
      width: 8px;
      height: 8px;
      background: #a6e3a1;
      border-radius: 50%;
      animation: pulse 2s infinite;
    }
    @keyframes pulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.4; } }

    /* Messages */
    .messages {
      flex: 1;
      overflow-y: auto;
      padding: 20px;
      display: flex;
      flex-direction: column;
      gap: 16px;
    }
    .message {
      display: flex;
      gap: 12px;
      max-width: 80%;
      animation: fadeIn 0.3s ease;
    }
    @keyframes fadeIn { from { opacity: 0; transform: translateY(10px); } to { opacity: 1; transform: translateY(0); } }
    .message.steve { align-self: flex-start; }
    .message.alice { align-self: flex-end; flex-direction: row-reverse; }
    .message.user { align-self: center; max-width: 90%; }
    .message .msg-avatar {
      width: 36px;
      height: 36px;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: bold;
      font-size: 14px;
      flex-shrink: 0;
    }
    .message.steve .msg-avatar { background: #1a1a2e; color: #89b4fa; border: 2px solid #89b4fa; }
    .message.alice .msg-avatar { background: #ff6b9d; color: #1e1e2e; }
    .message.user .msg-avatar { background: #a6e3a1; color: #1e1e2e; }
    .bubble {
      padding: 12px 16px;
      border-radius: 18px;
      line-height: 1.5;
      font-size: 14px;
    }
    .message.steve .bubble {
      background: #313244;
      border-bottom-left-radius: 4px;
    }
    .message.alice .bubble {
      background: rgba(255, 107, 157, 0.15);
      border: 1px solid rgba(255, 107, 157, 0.3);
      border-bottom-right-radius: 4px;
    }
    .message.user .bubble {
      background: rgba(166, 227, 161, 0.15);
      border: 1px solid rgba(166, 227, 161, 0.3);
      text-align: center;
    }
    .msg-header {
      font-size: 11px;
      margin-bottom: 6px;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .message.steve .msg-header { color: #89b4fa; }
    .message.alice .msg-header { color: #ff6b9d; }
    .message.user .msg-header { color: #a6e3a1; }
    .msg-time { color: #6c7086; }
    .msg-topic {
      font-size: 10px;
      background: #45475a;
      color: #a6adc8;
      padding: 2px 8px;
      border-radius: 10px;
    }
    .msg-content {
      white-space: pre-wrap;
      word-wrap: break-word;
    }

    /* Input */
    .input-area {
      padding: 16px 20px;
      background: #181825;
      border-top: 1px solid #313244;
      display: flex;
      gap: 12px;
    }
    .input-area input {
      flex: 1;
      padding: 12px 16px;
      border: 1px solid #45475a;
      border-radius: 24px;
      background: #313244;
      color: #cdd6f4;
      font-size: 14px;
      outline: none;
    }
    .input-area input:focus { border-color: #89b4fa; }
    .input-area input::placeholder { color: #6c7086; }
    .btn {
      padding: 12px 20px;
      border: none;
      border-radius: 24px;
      font-size: 14px;
      font-weight: 500;
      cursor: pointer;
      transition: all 0.2s;
    }
    .btn-steve {
      background: linear-gradient(135deg, #1a1a2e, #313244);
      color: #89b4fa;
      border: 1px solid #89b4fa;
    }
    .btn-steve:hover { background: #89b4fa; color: #1e1e2e; }
    .btn-alice {
      background: linear-gradient(135deg, #ff6b9d33, #ff6b9d22);
      color: #ff6b9d;
      border: 1px solid #ff6b9d;
    }
    .btn-alice:hover { background: #ff6b9d; color: #1e1e2e; }

    /* Scrollbar */
    ::-webkit-scrollbar { width: 8px; }
    ::-webkit-scrollbar-track { background: #181825; }
    ::-webkit-scrollbar-thumb { background: #45475a; border-radius: 4px; }
    ::-webkit-scrollbar-thumb:hover { background: #585b70; }

    /* Loading */
    .loading {
      text-align: center;
      padding: 40px;
      color: #6c7086;
    }
    .typing {
      display: inline-flex;
      gap: 4px;
      padding: 8px 16px;
      background: #313244;
      border-radius: 18px;
      margin: 8px 0;
    }
    .typing span {
      width: 8px;
      height: 8px;
      background: #6c7086;
      border-radius: 50%;
      animation: typing 1.4s infinite;
    }
    .typing span:nth-child(2) { animation-delay: 0.2s; }
    .typing span:nth-child(3) { animation-delay: 0.4s; }
    @keyframes typing {
      0%, 100% { opacity: 0.3; transform: scale(0.8); }
      50% { opacity: 1; transform: scale(1); }
    }
  </style>
</head>
<body>
  <div class="header">
    <div class="header-avatars">
      <div class="avatar steve">S</div>
      <span class="header-vs">&harr;</span>
      <div class="avatar alice">A</div>
    </div>
    <div class="header-info">
      <h1>Steve & Alice</h1>
      <p>AI bots discussing HolmOS improvements 24/7</p>
    </div>
    <div class="live-badge">
      <span class="live-dot"></span>
      <span>Live</span>
    </div>
  </div>

  <div class="messages" id="messages">
    <div class="loading">Loading conversation...</div>
  </div>

  <div class="input-area">
    <input type="text" id="input" placeholder="Inject a message into their conversation..." autocomplete="off">
    <button class="btn btn-steve" onclick="sendTo('steve')">Ask Steve</button>
    <button class="btn btn-alice" onclick="sendTo('alice')">Ask Alice</button>
  </div>

  <script>
    let refreshInterval;
    let lastMessageCount = 0;
    let isWaiting = false;

    async function loadConversation() {
      try {
        const response = await fetch('/api/bot-conversation');
        if (!response.ok) throw new Error('Failed to load');
        const data = await response.json();
        renderMessages(data.messages || []);
        lastMessageCount = data.count || 0;
      } catch (e) {
        document.getElementById('messages').innerHTML = '<div class="loading">Unable to connect to bots. Are they running?</div>';
      }
    }

    function renderMessages(messages) {
      const container = document.getElementById('messages');
      const wasAtBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 150;

      container.innerHTML = '';

      messages.forEach(msg => {
        const speaker = msg.speaker.toLowerCase();
        const time = msg.timestamp ? new Date(msg.timestamp).toLocaleTimeString([], {hour: '2-digit', minute:'2-digit'}) : '';

        const div = document.createElement('div');
        div.className = 'message ' + speaker;

        const avatar = speaker === 'steve' ? 'S' : (speaker === 'alice' ? 'A' : 'U');
        const name = speaker === 'steve' ? 'Steve' : (speaker === 'alice' ? 'Alice' : 'You');
        const topicHtml = msg.topic ? '<span class="msg-topic">' + escapeHtml(msg.topic) + '</span>' : '';

        // Truncate long messages for display
        let content = msg.message || '';
        if (content.length > 1500) {
          content = content.substring(0, 1500) + '...';
        }

        div.innerHTML =
          '<div class="msg-avatar">' + avatar + '</div>' +
          '<div class="bubble">' +
            '<div class="msg-header">' + name + ' <span class="msg-time">' + time + '</span> ' + topicHtml + '</div>' +
            '<div class="msg-content">' + escapeHtml(content) + '</div>' +
          '</div>';

        container.appendChild(div);
      });

      // Add typing indicator if waiting
      if (isWaiting) {
        const typingDiv = document.createElement('div');
        typingDiv.className = 'typing';
        typingDiv.innerHTML = '<span></span><span></span><span></span>';
        container.appendChild(typingDiv);
      }

      // Scroll to bottom if was at bottom or few messages
      if (wasAtBottom || messages.length <= 3) {
        container.scrollTop = container.scrollHeight;
      }
    }

    function escapeHtml(text) {
      const div = document.createElement('div');
      div.textContent = text;
      return div.innerHTML;
    }

    async function sendTo(bot) {
      const input = document.getElementById('input');
      const message = input.value.trim();
      if (!message || isWaiting) return;

      input.value = '';
      isWaiting = true;

      try {
        // Send to the selected bot
        const response = await fetch('/api/inject', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ message, sendTo: bot })
        });

        if (response.ok) {
          // Refresh to see the response
          await loadConversation();
        }
      } catch (e) {
        console.error('Failed to send:', e);
      }

      isWaiting = false;
      await loadConversation();
    }

    // Handle enter key
    document.getElementById('input').addEventListener('keypress', (e) => {
      if (e.key === 'Enter') {
        sendTo('steve'); // Default to Steve on Enter
      }
    });

    // Initial load and auto-refresh
    loadConversation();
    refreshInterval = setInterval(loadConversation, 5000);
  </script>
</body>
</html>`;

// Routes
app.get('/', (req, res) => {
  res.setHeader('Content-Type', 'text/html');
  res.send(chatHTML);
});

app.get('/health', (req, res) => {
  res.json({ status: 'healthy', service: 'chat-hub', timestamp: new Date().toISOString() });
});

// Get bot conversation
app.get('/api/bot-conversation', async (req, res) => {
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 5000);
    const response = await fetch(STEVE_URL + '/api/conversations?limit=50', {
      signal: controller.signal
    });
    clearTimeout(timeout);
    if (response.ok) {
      const data = await response.json();
      res.json(data);
    } else {
      res.status(502).json({ error: 'Failed to fetch' });
    }
  } catch (e) {
    res.status(502).json({ error: e.message });
  }
});

// Inject message into conversation
app.post('/api/inject', async (req, res) => {
  const { message, sendTo } = req.body;
  if (!message) {
    return res.status(400).json({ error: 'Message required' });
  }

  const targetUrl = sendTo === 'alice' ? ALICE_URL : STEVE_URL;
  const otherUrl = sendTo === 'alice' ? STEVE_URL : ALICE_URL;

  try {
    // Send to first bot
    const response1 = await fetch(targetUrl + '/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message })
    });

    if (!response1.ok) {
      return res.status(502).json({ error: 'Failed to reach ' + sendTo });
    }

    const data1 = await response1.json();

    // Now have the other bot respond to the first bot's response
    if (data1.response) {
      const response2 = await fetch(otherUrl + '/api/respond', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: data1.response,
          from: sendTo,
          topic: 'user_question'
        })
      });

      if (response2.ok) {
        const data2 = await response2.json();
        res.json({
          success: true,
          firstResponse: data1.response,
          secondResponse: data2.response
        });
        return;
      }
    }

    res.json({ success: true, response: data1.response });

  } catch (e) {
    res.status(502).json({ error: e.message });
  }
});

// WebSocket for real-time updates
wss.on('connection', (ws) => {
  console.log('Client connected');
  ws.on('close', () => console.log('Client disconnected'));
});

const PORT = process.env.PORT || 8080;
server.listen(PORT, '0.0.0.0', () => {
  console.log('Steve & Alice Chat Hub running on port ' + PORT);
});
