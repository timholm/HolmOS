const express = require('express');
const http = require('http');
const WebSocket = require('ws');
const { Pool } = require('pg');
const { v4: uuidv4 } = require('uuid');

const app = express();
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

app.use(express.json());

// PostgreSQL configuration
const pool = new Pool({
  host: process.env.DB_HOST || 'postgres.holm.svc.cluster.local',
  port: parseInt(process.env.DB_PORT || '5432'),
  database: process.env.DB_NAME || 'claudepod',
  user: process.env.DB_USER || 'claudepod',
  password: process.env.DB_PASSWORD || 'claudepod',
  max: 10,
  idleTimeoutMillis: 30000,
  connectionTimeoutMillis: 5000,
});

// Claude API configuration
const CLAUDE_API_KEY = process.env.ANTHROPIC_API_KEY || process.env.CLAUDE_API_KEY || '';
const CLAUDE_MODEL = process.env.CLAUDE_MODEL || 'claude-sonnet-4-20250514';
const CLAUDE_API_URL = 'https://api.anthropic.com/v1/messages';

// In-memory fallback for message history
let memoryMessages = [];
const MAX_MEMORY_MESSAGES = 100;

// Database initialization
async function initDatabase() {
  try {
    await pool.query(`
      CREATE TABLE IF NOT EXISTS conversations (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        title VARCHAR(255),
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
      )
    `);

    await pool.query(`
      CREATE TABLE IF NOT EXISTS messages (
        id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
        conversation_id UUID REFERENCES conversations(id) ON DELETE CASCADE,
        role VARCHAR(20) NOT NULL,
        content TEXT NOT NULL,
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
      )
    `);

    await pool.query(`
      CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id)
    `);

    await pool.query(`
      CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at)
    `);

    console.log('Database initialized successfully');
    return true;
  } catch (error) {
    console.error('Database initialization failed:', error.message);
    console.log('Using in-memory storage fallback');
    return false;
  }
}

// Database helper functions
async function createConversation(title = 'New Chat') {
  try {
    const result = await pool.query(
      'INSERT INTO conversations (title) VALUES ($1) RETURNING *',
      [title]
    );
    return result.rows[0];
  } catch (error) {
    console.error('Error creating conversation:', error.message);
    return { id: uuidv4(), title, created_at: new Date(), updated_at: new Date() };
  }
}

async function getConversations() {
  try {
    const result = await pool.query(
      'SELECT * FROM conversations ORDER BY updated_at DESC LIMIT 50'
    );
    return result.rows;
  } catch (error) {
    console.error('Error getting conversations:', error.message);
    return [];
  }
}

async function getMessages(conversationId) {
  try {
    const result = await pool.query(
      'SELECT * FROM messages WHERE conversation_id = $1 ORDER BY created_at ASC',
      [conversationId]
    );
    return result.rows;
  } catch (error) {
    console.error('Error getting messages:', error.message);
    return memoryMessages.filter(m => m.conversation_id === conversationId);
  }
}

async function saveMessage(conversationId, role, content) {
  const message = {
    id: uuidv4(),
    conversation_id: conversationId,
    role,
    content,
    created_at: new Date()
  };

  try {
    const result = await pool.query(
      'INSERT INTO messages (conversation_id, role, content) VALUES ($1, $2, $3) RETURNING *',
      [conversationId, role, content]
    );

    // Update conversation timestamp
    await pool.query(
      'UPDATE conversations SET updated_at = CURRENT_TIMESTAMP WHERE id = $1',
      [conversationId]
    );

    return result.rows[0];
  } catch (error) {
    console.error('Error saving message:', error.message);
    memoryMessages.push(message);
    if (memoryMessages.length > MAX_MEMORY_MESSAGES) {
      memoryMessages = memoryMessages.slice(-MAX_MEMORY_MESSAGES);
    }
    return message;
  }
}

async function deleteConversation(conversationId) {
  try {
    await pool.query('DELETE FROM conversations WHERE id = $1', [conversationId]);
    return true;
  } catch (error) {
    console.error('Error deleting conversation:', error.message);
    memoryMessages = memoryMessages.filter(m => m.conversation_id !== conversationId);
    return true;
  }
}

async function updateConversationTitle(conversationId, title) {
  try {
    await pool.query(
      'UPDATE conversations SET title = $1, updated_at = CURRENT_TIMESTAMP WHERE id = $2',
      [title, conversationId]
    );
    return true;
  } catch (error) {
    console.error('Error updating conversation title:', error.message);
    return false;
  }
}

// Call Claude API with streaming
async function callClaudeStream(messages, conversationId, ws) {
  if (!CLAUDE_API_KEY) {
    return 'I apologize, but the Claude API key is not configured. Please set the ANTHROPIC_API_KEY environment variable to enable AI responses.';
  }

  const systemPrompt = `You are Claude, a helpful AI assistant created by Anthropic. You are running inside HolmOS, a Kubernetes-based home operating system. Be helpful, harmless, and honest. When writing code, use proper markdown code blocks with language specification. Format your responses using markdown for better readability.`;

  const apiMessages = messages.map(m => ({
    role: m.role === 'user' ? 'user' : 'assistant',
    content: m.content
  }));

  try {
    const response = await fetch(CLAUDE_API_URL, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'x-api-key': CLAUDE_API_KEY,
        'anthropic-version': '2023-06-01'
      },
      body: JSON.stringify({
        model: CLAUDE_MODEL,
        max_tokens: 4096,
        system: systemPrompt,
        messages: apiMessages,
        stream: true
      })
    });

    if (!response.ok) {
      const errorText = await response.text();
      console.error('Claude API error:', response.status, errorText);
      return `Error calling Claude API: ${response.status}`;
    }

    let fullResponse = '';
    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split('\n');
      buffer = lines.pop() || '';

      for (const line of lines) {
        if (line.startsWith('data: ')) {
          const data = line.slice(6);
          if (data === '[DONE]') continue;

          try {
            const parsed = JSON.parse(data);
            if (parsed.type === 'content_block_delta' && parsed.delta?.text) {
              const chunk = parsed.delta.text;
              fullResponse += chunk;

              // Send streaming chunk to client
              if (ws && ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({
                  type: 'stream',
                  conversationId,
                  chunk,
                  timestamp: new Date().toISOString()
                }));
              }
            }
          } catch (e) {
            // Skip invalid JSON
          }
        }
      }
    }

    return fullResponse;
  } catch (error) {
    console.error('Error calling Claude API:', error);
    return `Error: ${error.message}`;
  }
}

// WebSocket connection handling
const clientConnections = new Map();

wss.on('connection', (ws) => {
  const clientId = uuidv4();
  clientConnections.set(clientId, ws);
  console.log('New WebSocket connection:', clientId);

  ws.on('message', async (data) => {
    try {
      const msg = JSON.parse(data.toString());

      if (msg.type === 'chat') {
        const { conversationId, message } = msg;

        // Save user message
        await saveMessage(conversationId, 'user', message);

        // Get conversation history
        const history = await getMessages(conversationId);

        // Notify client that assistant is typing
        ws.send(JSON.stringify({
          type: 'typing',
          conversationId,
          timestamp: new Date().toISOString()
        }));

        // Call Claude with streaming
        const response = await callClaudeStream(history, conversationId, ws);

        // Save assistant message
        const savedMessage = await saveMessage(conversationId, 'assistant', response);

        // Send completion message
        ws.send(JSON.stringify({
          type: 'complete',
          conversationId,
          message: savedMessage,
          timestamp: new Date().toISOString()
        }));
      }
    } catch (error) {
      console.error('WebSocket message error:', error);
      ws.send(JSON.stringify({
        type: 'error',
        error: error.message,
        timestamp: new Date().toISOString()
      }));
    }
  });

  ws.on('close', () => {
    clientConnections.delete(clientId);
    console.log('WebSocket connection closed:', clientId);
  });

  ws.on('error', (error) => {
    console.error('WebSocket error:', error);
    clientConnections.delete(clientId);
  });
});

// Broadcast to all clients
function broadcast(data) {
  clientConnections.forEach((ws) => {
    if (ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(data));
    }
  });
}

// Chat UI HTML with Catppuccin Mocha theme
const chatHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Claude - HolmOS AI Assistant</title>
  <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/github-dark.min.css">
  <script src="https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js"></script>
  <script src="https://cdnjs.cloudflare.com/ajax/libs/marked/11.1.1/marked.min.js"></script>
  <style>
    :root {
      --base: #1e1e2e;
      --mantle: #181825;
      --crust: #11111b;
      --text: #cdd6f4;
      --subtext0: #a6adc8;
      --subtext1: #bac2de;
      --surface0: #313244;
      --surface1: #45475a;
      --surface2: #585b70;
      --overlay0: #6c7086;
      --overlay1: #7f849c;
      --mauve: #cba6f7;
      --blue: #89b4fa;
      --sapphire: #74c7ec;
      --green: #a6e3a1;
      --red: #f38ba8;
      --peach: #fab387;
      --yellow: #f9e2af;
      --lavender: #b4befe;
    }

    * { margin: 0; padding: 0; box-sizing: border-box; }

    body {
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', sans-serif;
      background: var(--base);
      color: var(--text);
      height: 100vh;
      overflow: hidden;
    }

    .container {
      display: flex;
      height: 100vh;
    }

    /* Sidebar */
    .sidebar {
      width: 280px;
      background: var(--mantle);
      border-right: 1px solid var(--surface0);
      display: flex;
      flex-direction: column;
      flex-shrink: 0;
    }

    .sidebar-header {
      padding: 16px;
      border-bottom: 1px solid var(--surface0);
    }

    .new-chat-btn {
      width: 100%;
      padding: 12px 16px;
      background: var(--mauve);
      color: var(--crust);
      border: none;
      border-radius: 8px;
      font-size: 14px;
      font-weight: 600;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 8px;
      transition: all 0.2s;
    }

    .new-chat-btn:hover {
      background: var(--lavender);
      transform: translateY(-1px);
    }

    .conversations-list {
      flex: 1;
      overflow-y: auto;
      padding: 8px;
    }

    .conversation-item {
      padding: 12px 16px;
      margin: 4px 0;
      border-radius: 8px;
      cursor: pointer;
      transition: all 0.2s;
      display: flex;
      align-items: center;
      gap: 12px;
      position: relative;
    }

    .conversation-item:hover {
      background: var(--surface0);
    }

    .conversation-item.active {
      background: var(--surface1);
    }

    .conversation-item .icon {
      width: 20px;
      height: 20px;
      opacity: 0.7;
    }

    .conversation-item .title {
      flex: 1;
      font-size: 14px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    .conversation-item .delete-btn {
      opacity: 0;
      padding: 4px;
      border: none;
      background: none;
      color: var(--red);
      cursor: pointer;
      border-radius: 4px;
      transition: all 0.2s;
    }

    .conversation-item:hover .delete-btn {
      opacity: 1;
    }

    .conversation-item .delete-btn:hover {
      background: var(--surface2);
    }

    .sidebar-footer {
      padding: 16px;
      border-top: 1px solid var(--surface0);
      font-size: 12px;
      color: var(--overlay0);
      text-align: center;
    }

    .sidebar-footer a {
      color: var(--mauve);
      text-decoration: none;
    }

    /* Main Content */
    .main {
      flex: 1;
      display: flex;
      flex-direction: column;
      min-width: 0;
    }

    .chat-header {
      padding: 16px 24px;
      border-bottom: 1px solid var(--surface0);
      display: flex;
      align-items: center;
      gap: 12px;
      background: var(--mantle);
    }

    .chat-header .logo {
      width: 32px;
      height: 32px;
      background: linear-gradient(135deg, var(--mauve) 0%, var(--blue) 100%);
      border-radius: 8px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: bold;
      color: var(--crust);
      font-size: 16px;
    }

    .chat-header h1 {
      font-size: 18px;
      font-weight: 600;
    }

    .chat-header .model-badge {
      margin-left: auto;
      padding: 4px 12px;
      background: var(--surface0);
      border-radius: 16px;
      font-size: 12px;
      color: var(--subtext0);
    }

    .connection-status {
      display: flex;
      align-items: center;
      gap: 6px;
      font-size: 12px;
      color: var(--overlay0);
    }

    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--green);
    }

    .status-dot.disconnected {
      background: var(--red);
    }

    /* Messages Area */
    .messages-container {
      flex: 1;
      overflow-y: auto;
      padding: 0;
    }

    .messages {
      max-width: 800px;
      margin: 0 auto;
      padding: 24px;
    }

    .message {
      margin-bottom: 24px;
      animation: fadeIn 0.3s ease;
    }

    @keyframes fadeIn {
      from { opacity: 0; transform: translateY(10px); }
      to { opacity: 1; transform: translateY(0); }
    }

    .message-header {
      display: flex;
      align-items: center;
      gap: 12px;
      margin-bottom: 8px;
    }

    .message-avatar {
      width: 32px;
      height: 32px;
      border-radius: 6px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: 600;
      font-size: 14px;
    }

    .message.user .message-avatar {
      background: var(--blue);
      color: var(--crust);
    }

    .message.assistant .message-avatar {
      background: linear-gradient(135deg, var(--mauve) 0%, var(--blue) 100%);
      color: var(--crust);
    }

    .message-sender {
      font-weight: 600;
      font-size: 14px;
    }

    .message-time {
      font-size: 12px;
      color: var(--overlay0);
    }

    .message-content {
      padding-left: 44px;
      line-height: 1.7;
      font-size: 15px;
    }

    .message-content p {
      margin-bottom: 12px;
    }

    .message-content p:last-child {
      margin-bottom: 0;
    }

    .message-content ul, .message-content ol {
      margin: 12px 0;
      padding-left: 24px;
    }

    .message-content li {
      margin-bottom: 6px;
    }

    .message-content code {
      background: var(--surface0);
      padding: 2px 6px;
      border-radius: 4px;
      font-family: 'SF Mono', 'Fira Code', monospace;
      font-size: 13px;
    }

    .message-content pre {
      background: var(--crust);
      border-radius: 8px;
      margin: 16px 0;
      overflow: hidden;
    }

    .message-content pre code {
      display: block;
      padding: 16px;
      background: transparent;
      overflow-x: auto;
      font-size: 13px;
      line-height: 1.5;
    }

    .code-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 8px 16px;
      background: var(--surface0);
      font-size: 12px;
      color: var(--subtext0);
    }

    .copy-btn {
      padding: 4px 8px;
      background: var(--surface1);
      border: none;
      border-radius: 4px;
      color: var(--text);
      font-size: 11px;
      cursor: pointer;
      transition: all 0.2s;
    }

    .copy-btn:hover {
      background: var(--surface2);
    }

    .message-content blockquote {
      border-left: 3px solid var(--mauve);
      padding-left: 16px;
      margin: 12px 0;
      color: var(--subtext1);
    }

    .message-content a {
      color: var(--blue);
      text-decoration: none;
    }

    .message-content a:hover {
      text-decoration: underline;
    }

    .message-content strong {
      color: var(--text);
      font-weight: 600;
    }

    .message-content em {
      color: var(--subtext1);
    }

    .message-content h1, .message-content h2, .message-content h3 {
      margin: 20px 0 12px;
      color: var(--text);
    }

    .message-content h1 { font-size: 24px; }
    .message-content h2 { font-size: 20px; }
    .message-content h3 { font-size: 17px; }

    .message-content hr {
      border: none;
      border-top: 1px solid var(--surface1);
      margin: 20px 0;
    }

    .message-content table {
      width: 100%;
      border-collapse: collapse;
      margin: 16px 0;
    }

    .message-content th, .message-content td {
      padding: 10px 14px;
      border: 1px solid var(--surface1);
      text-align: left;
    }

    .message-content th {
      background: var(--surface0);
      font-weight: 600;
    }

    /* Typing Indicator */
    .typing-indicator {
      display: flex;
      align-items: center;
      gap: 8px;
      padding: 16px 44px;
      color: var(--subtext0);
      font-size: 14px;
    }

    .typing-dots {
      display: flex;
      gap: 4px;
    }

    .typing-dots span {
      width: 8px;
      height: 8px;
      background: var(--mauve);
      border-radius: 50%;
      animation: typing 1.4s infinite ease-in-out both;
    }

    .typing-dots span:nth-child(1) { animation-delay: -0.32s; }
    .typing-dots span:nth-child(2) { animation-delay: -0.16s; }

    @keyframes typing {
      0%, 80%, 100% { transform: scale(0.8); opacity: 0.5; }
      40% { transform: scale(1); opacity: 1; }
    }

    /* Welcome Screen */
    .welcome-screen {
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      padding: 40px;
      text-align: center;
    }

    .welcome-logo {
      width: 80px;
      height: 80px;
      background: linear-gradient(135deg, var(--mauve) 0%, var(--blue) 100%);
      border-radius: 20px;
      display: flex;
      align-items: center;
      justify-content: center;
      font-size: 40px;
      font-weight: bold;
      color: var(--crust);
      margin-bottom: 24px;
    }

    .welcome-screen h2 {
      font-size: 28px;
      margin-bottom: 12px;
      color: var(--text);
    }

    .welcome-screen p {
      font-size: 16px;
      color: var(--subtext0);
      max-width: 500px;
      line-height: 1.6;
      margin-bottom: 32px;
    }

    .suggestions {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 12px;
      max-width: 600px;
    }

    .suggestion {
      padding: 16px;
      background: var(--surface0);
      border: 1px solid var(--surface1);
      border-radius: 12px;
      cursor: pointer;
      transition: all 0.2s;
      text-align: left;
    }

    .suggestion:hover {
      background: var(--surface1);
      border-color: var(--mauve);
      transform: translateY(-2px);
    }

    .suggestion-title {
      font-weight: 600;
      font-size: 14px;
      margin-bottom: 4px;
      color: var(--text);
    }

    .suggestion-desc {
      font-size: 12px;
      color: var(--overlay0);
    }

    /* Input Area */
    .input-area {
      padding: 16px 24px 24px;
      background: var(--base);
    }

    .input-wrapper {
      max-width: 800px;
      margin: 0 auto;
    }

    .input-container {
      display: flex;
      gap: 12px;
      background: var(--surface0);
      border: 1px solid var(--surface1);
      border-radius: 16px;
      padding: 8px 8px 8px 20px;
      transition: all 0.2s;
    }

    .input-container:focus-within {
      border-color: var(--mauve);
      box-shadow: 0 0 0 2px rgba(203, 166, 247, 0.2);
    }

    #messageInput {
      flex: 1;
      background: transparent;
      border: none;
      color: var(--text);
      font-size: 15px;
      resize: none;
      outline: none;
      min-height: 24px;
      max-height: 200px;
      line-height: 1.5;
      font-family: inherit;
    }

    #messageInput::placeholder {
      color: var(--overlay0);
    }

    #sendButton {
      width: 40px;
      height: 40px;
      background: var(--mauve);
      border: none;
      border-radius: 10px;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      transition: all 0.2s;
      flex-shrink: 0;
    }

    #sendButton:hover:not(:disabled) {
      background: var(--lavender);
      transform: scale(1.05);
    }

    #sendButton:disabled {
      background: var(--surface1);
      cursor: not-allowed;
    }

    #sendButton svg {
      width: 20px;
      height: 20px;
      fill: var(--crust);
    }

    .input-hint {
      text-align: center;
      font-size: 12px;
      color: var(--overlay0);
      margin-top: 8px;
    }

    /* Scrollbar */
    ::-webkit-scrollbar {
      width: 8px;
    }

    ::-webkit-scrollbar-track {
      background: var(--mantle);
    }

    ::-webkit-scrollbar-thumb {
      background: var(--surface1);
      border-radius: 4px;
    }

    ::-webkit-scrollbar-thumb:hover {
      background: var(--surface2);
    }

    /* Responsive */
    @media (max-width: 768px) {
      .sidebar {
        display: none;
      }

      .suggestions {
        grid-template-columns: 1fr;
      }

      .messages {
        padding: 16px;
      }

      .message-content {
        padding-left: 0;
      }
    }
  </style>
</head>
<body>
  <div class="container">
    <div class="sidebar">
      <div class="sidebar-header">
        <button class="new-chat-btn" onclick="newChat()">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M12 5v14M5 12h14"/>
          </svg>
          New Chat
        </button>
      </div>
      <div class="conversations-list" id="conversationsList"></div>
      <div class="sidebar-footer">
        Powered by <a href="https://anthropic.com" target="_blank">Claude</a>
      </div>
    </div>

    <div class="main">
      <div class="chat-header">
        <div class="logo">C</div>
        <h1>Claude</h1>
        <div class="model-badge" id="modelBadge">claude-sonnet-4-20250514</div>
        <div class="connection-status">
          <span class="status-dot" id="statusDot"></span>
          <span id="statusText">Connected</span>
        </div>
      </div>

      <div class="messages-container" id="messagesContainer">
        <div class="welcome-screen" id="welcomeScreen">
          <div class="welcome-logo">C</div>
          <h2>How can I help you today?</h2>
          <p>I'm Claude, an AI assistant by Anthropic. I can help you write, analyze, code, and more. Start a conversation to get started.</p>
          <div class="suggestions">
            <div class="suggestion" onclick="sendSuggestion('Help me write a Python script to process JSON files')">
              <div class="suggestion-title">Write Code</div>
              <div class="suggestion-desc">Help me write a Python script to process JSON files</div>
            </div>
            <div class="suggestion" onclick="sendSuggestion('Explain how Kubernetes networking works')">
              <div class="suggestion-title">Explain Concepts</div>
              <div class="suggestion-desc">Explain how Kubernetes networking works</div>
            </div>
            <div class="suggestion" onclick="sendSuggestion('Review my code and suggest improvements')">
              <div class="suggestion-title">Code Review</div>
              <div class="suggestion-desc">Review my code and suggest improvements</div>
            </div>
            <div class="suggestion" onclick="sendSuggestion('Help me debug this error message')">
              <div class="suggestion-title">Debug Issues</div>
              <div class="suggestion-desc">Help me debug this error message</div>
            </div>
          </div>
        </div>
        <div class="messages" id="messages" style="display: none;"></div>
      </div>

      <div class="input-area">
        <div class="input-wrapper">
          <div class="input-container">
            <textarea id="messageInput" placeholder="Message Claude..." rows="1"></textarea>
            <button id="sendButton" onclick="sendMessage()">
              <svg viewBox="0 0 24 24">
                <path d="M2.01 21L23 12 2.01 3 2 10l15 2-15 2z"/>
              </svg>
            </button>
          </div>
          <div class="input-hint">Press Enter to send, Shift+Enter for new line</div>
        </div>
      </div>
    </div>
  </div>

  <script>
    // Configure marked for markdown rendering
    marked.setOptions({
      highlight: function(code, lang) {
        if (lang && hljs.getLanguage(lang)) {
          try {
            return hljs.highlight(code, { language: lang }).value;
          } catch (e) {}
        }
        return hljs.highlightAuto(code).value;
      },
      breaks: true,
      gfm: true
    });

    let ws = null;
    let conversations = [];
    let currentConversation = null;
    let currentMessages = [];
    let streamingContent = '';
    let isStreaming = false;

    // Connect WebSocket
    function connect() {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      ws = new WebSocket(protocol + '//' + window.location.host + '/ws');

      ws.onopen = () => {
        document.getElementById('statusDot').classList.remove('disconnected');
        document.getElementById('statusText').textContent = 'Connected';
        loadConversations();
      };

      ws.onclose = () => {
        document.getElementById('statusDot').classList.add('disconnected');
        document.getElementById('statusText').textContent = 'Reconnecting...';
        setTimeout(connect, 3000);
      };

      ws.onmessage = (event) => {
        const data = JSON.parse(event.data);
        handleMessage(data);
      };

      ws.onerror = (error) => {
        console.error('WebSocket error:', error);
      };
    }

    function handleMessage(data) {
      switch (data.type) {
        case 'typing':
          showTypingIndicator();
          break;
        case 'stream':
          handleStreamChunk(data.chunk);
          break;
        case 'complete':
          finishStreaming(data.message);
          break;
        case 'error':
          hideTypingIndicator();
          addErrorMessage(data.error);
          break;
      }
    }

    function showTypingIndicator() {
      hideTypingIndicator();
      const container = document.getElementById('messages');
      const typing = document.createElement('div');
      typing.id = 'typingIndicator';
      typing.className = 'typing-indicator';
      typing.innerHTML = '<div class="typing-dots"><span></span><span></span><span></span></div><span>Claude is thinking...</span>';
      container.appendChild(typing);
      container.scrollTop = container.scrollHeight;
    }

    function hideTypingIndicator() {
      const indicator = document.getElementById('typingIndicator');
      if (indicator) indicator.remove();
    }

    function handleStreamChunk(chunk) {
      if (!isStreaming) {
        isStreaming = true;
        hideTypingIndicator();
        createStreamingMessage();
      }

      streamingContent += chunk;
      updateStreamingMessage();
    }

    function createStreamingMessage() {
      const container = document.getElementById('messages');
      const msgDiv = document.createElement('div');
      msgDiv.id = 'streamingMessage';
      msgDiv.className = 'message assistant';
      msgDiv.innerHTML = \`
        <div class="message-header">
          <div class="message-avatar">C</div>
          <span class="message-sender">Claude</span>
          <span class="message-time">\${formatTime(new Date())}</span>
        </div>
        <div class="message-content" id="streamingContent"></div>
      \`;
      container.appendChild(msgDiv);
    }

    function updateStreamingMessage() {
      const content = document.getElementById('streamingContent');
      if (content) {
        content.innerHTML = renderMarkdown(streamingContent);
        highlightCodeBlocks();
        const container = document.getElementById('messagesContainer');
        container.scrollTop = container.scrollHeight;
      }
    }

    function finishStreaming(message) {
      isStreaming = false;
      const streamingMsg = document.getElementById('streamingMessage');
      if (streamingMsg) {
        streamingMsg.id = '';
      }
      streamingContent = '';
      currentMessages.push(message);
      enableInput();
    }

    function renderMarkdown(text) {
      let html = marked.parse(text);

      // Add copy button to code blocks
      html = html.replace(/<pre><code class="language-([\\w-]+)">/g, (match, lang) => {
        return \`<pre><div class="code-header"><span>\${lang}</span><button class="copy-btn" onclick="copyCode(this)">Copy</button></div><code class="language-\${lang}">\`;
      });

      html = html.replace(/<pre><code>/g, () => {
        return '<pre><div class="code-header"><span>code</span><button class="copy-btn" onclick="copyCode(this)">Copy</button></div><code>';
      });

      return html;
    }

    function highlightCodeBlocks() {
      document.querySelectorAll('pre code').forEach((block) => {
        hljs.highlightElement(block);
      });
    }

    function copyCode(button) {
      const codeBlock = button.closest('pre').querySelector('code');
      navigator.clipboard.writeText(codeBlock.textContent).then(() => {
        button.textContent = 'Copied!';
        setTimeout(() => { button.textContent = 'Copy'; }, 2000);
      });
    }

    function addErrorMessage(error) {
      const container = document.getElementById('messages');
      const msgDiv = document.createElement('div');
      msgDiv.className = 'message assistant';
      msgDiv.innerHTML = \`
        <div class="message-header">
          <div class="message-avatar" style="background: var(--red)">!</div>
          <span class="message-sender">Error</span>
          <span class="message-time">\${formatTime(new Date())}</span>
        </div>
        <div class="message-content" style="color: var(--red);">\${escapeHtml(error)}</div>
      \`;
      container.appendChild(msgDiv);
      container.scrollTop = container.scrollHeight;
      enableInput();
    }

    async function loadConversations() {
      try {
        const response = await fetch('/api/conversations');
        conversations = await response.json();
        renderConversationsList();
      } catch (error) {
        console.error('Error loading conversations:', error);
      }
    }

    function renderConversationsList() {
      const container = document.getElementById('conversationsList');
      container.innerHTML = '';

      conversations.forEach(conv => {
        const item = document.createElement('div');
        item.className = 'conversation-item' + (currentConversation?.id === conv.id ? ' active' : '');
        item.innerHTML = \`
          <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"></path>
          </svg>
          <span class="title">\${escapeHtml(conv.title || 'New Chat')}</span>
          <button class="delete-btn" onclick="event.stopPropagation(); deleteConversation('\${conv.id}')">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M3 6h18M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>
            </svg>
          </button>
        \`;
        item.onclick = () => selectConversation(conv);
        container.appendChild(item);
      });
    }

    async function selectConversation(conv) {
      currentConversation = conv;
      renderConversationsList();

      document.getElementById('welcomeScreen').style.display = 'none';
      document.getElementById('messages').style.display = 'block';

      try {
        const response = await fetch('/api/conversations/' + conv.id + '/messages');
        currentMessages = await response.json();
        renderMessages();
      } catch (error) {
        console.error('Error loading messages:', error);
      }
    }

    function renderMessages() {
      const container = document.getElementById('messages');
      container.innerHTML = '';

      currentMessages.forEach(msg => {
        addMessageToUI(msg);
      });

      const messagesContainer = document.getElementById('messagesContainer');
      messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }

    function addMessageToUI(msg) {
      const container = document.getElementById('messages');
      const msgDiv = document.createElement('div');
      msgDiv.className = 'message ' + msg.role;

      const avatar = msg.role === 'user' ? 'U' : 'C';
      const sender = msg.role === 'user' ? 'You' : 'Claude';
      const content = msg.role === 'assistant' ? renderMarkdown(msg.content) : escapeHtml(msg.content);

      msgDiv.innerHTML = \`
        <div class="message-header">
          <div class="message-avatar">\${avatar}</div>
          <span class="message-sender">\${sender}</span>
          <span class="message-time">\${formatTime(new Date(msg.created_at))}</span>
        </div>
        <div class="message-content">\${content}</div>
      \`;

      container.appendChild(msgDiv);

      if (msg.role === 'assistant') {
        highlightCodeBlocks();
      }
    }

    async function newChat() {
      try {
        const response = await fetch('/api/conversations', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ title: 'New Chat' })
        });
        const conv = await response.json();
        conversations.unshift(conv);
        selectConversation(conv);
      } catch (error) {
        console.error('Error creating conversation:', error);
      }
    }

    async function deleteConversation(id) {
      try {
        await fetch('/api/conversations/' + id, { method: 'DELETE' });
        conversations = conversations.filter(c => c.id !== id);
        renderConversationsList();

        if (currentConversation?.id === id) {
          currentConversation = null;
          currentMessages = [];
          document.getElementById('welcomeScreen').style.display = 'flex';
          document.getElementById('messages').style.display = 'none';
        }
      } catch (error) {
        console.error('Error deleting conversation:', error);
      }
    }

    async function sendMessage() {
      const input = document.getElementById('messageInput');
      const message = input.value.trim();

      if (!message || isStreaming) return;

      if (!currentConversation) {
        await newChat();
      }

      document.getElementById('welcomeScreen').style.display = 'none';
      document.getElementById('messages').style.display = 'block';

      // Add user message to UI
      const userMsg = {
        role: 'user',
        content: message,
        created_at: new Date().toISOString()
      };
      currentMessages.push(userMsg);
      addMessageToUI(userMsg);

      input.value = '';
      adjustTextareaHeight();
      disableInput();

      // Send via WebSocket
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({
          type: 'chat',
          conversationId: currentConversation.id,
          message: message
        }));
      } else {
        addErrorMessage('Not connected to server. Please wait...');
        enableInput();
      }

      const messagesContainer = document.getElementById('messagesContainer');
      messagesContainer.scrollTop = messagesContainer.scrollHeight;
    }

    function sendSuggestion(text) {
      document.getElementById('messageInput').value = text;
      sendMessage();
    }

    function disableInput() {
      document.getElementById('messageInput').disabled = true;
      document.getElementById('sendButton').disabled = true;
    }

    function enableInput() {
      document.getElementById('messageInput').disabled = false;
      document.getElementById('sendButton').disabled = false;
      document.getElementById('messageInput').focus();
    }

    function formatTime(date) {
      return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
    }

    function escapeHtml(text) {
      const div = document.createElement('div');
      div.textContent = text;
      return div.innerHTML;
    }

    function adjustTextareaHeight() {
      const textarea = document.getElementById('messageInput');
      textarea.style.height = 'auto';
      textarea.style.height = Math.min(textarea.scrollHeight, 200) + 'px';
    }

    // Event listeners
    document.getElementById('messageInput').addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault();
        sendMessage();
      }
    });

    document.getElementById('messageInput').addEventListener('input', adjustTextareaHeight);

    // Fetch model info
    async function loadModelInfo() {
      try {
        const response = await fetch('/api/model');
        const data = await response.json();
        document.getElementById('modelBadge').textContent = data.model || 'claude-sonnet-4-20250514';
      } catch (error) {
        console.error('Error loading model info:', error);
      }
    }

    // Initialize
    connect();
    loadModelInfo();
  </script>
</body>
</html>`;

// Routes
app.get('/', (req, res) => {
  res.setHeader('Content-Type', 'text/html');
  res.send(chatHTML);
});

app.get('/health', (req, res) => {
  res.json({
    status: 'healthy',
    service: 'claude-pod',
    timestamp: new Date().toISOString(),
    connections: clientConnections.size,
    database: pool ? 'connected' : 'fallback',
    model: CLAUDE_MODEL
  });
});

app.get('/api/health', (req, res) => {
  res.json({
    status: 'healthy',
    service: 'claude-pod',
    timestamp: new Date().toISOString()
  });
});

app.get('/api/model', (req, res) => {
  res.json({
    model: CLAUDE_MODEL,
    configured: !!CLAUDE_API_KEY
  });
});

// Conversation endpoints
app.get('/api/conversations', async (req, res) => {
  const conversations = await getConversations();
  res.json(conversations);
});

app.post('/api/conversations', async (req, res) => {
  const { title } = req.body;
  const conversation = await createConversation(title || 'New Chat');
  res.json(conversation);
});

app.get('/api/conversations/:id/messages', async (req, res) => {
  const { id } = req.params;
  const messages = await getMessages(id);
  res.json(messages);
});

app.delete('/api/conversations/:id', async (req, res) => {
  const { id } = req.params;
  await deleteConversation(id);
  res.json({ success: true });
});

app.put('/api/conversations/:id', async (req, res) => {
  const { id } = req.params;
  const { title } = req.body;
  await updateConversationTitle(id, title);
  res.json({ success: true });
});

// Chat endpoint (HTTP fallback)
app.post('/api/chat', async (req, res) => {
  const { conversationId, message } = req.body;

  if (!message) {
    return res.status(400).json({ error: 'Message required' });
  }

  let convId = conversationId;
  if (!convId) {
    const conv = await createConversation('New Chat');
    convId = conv.id;
  }

  await saveMessage(convId, 'user', message);
  const history = await getMessages(convId);

  // Non-streaming response for HTTP
  const response = await callClaudeStream(history, convId, null);
  const savedMessage = await saveMessage(convId, 'assistant', response);

  res.json({
    conversationId: convId,
    message: savedMessage
  });
});

// Start server
const PORT = process.env.PORT || 30001;

async function start() {
  const dbInitialized = await initDatabase();

  server.listen(PORT, '0.0.0.0', () => {
    console.log('Claude Pod AI Chat Service running on port ' + PORT);
    console.log('Database:', dbInitialized ? 'PostgreSQL' : 'In-memory fallback');
    console.log('Model:', CLAUDE_MODEL);
    console.log('API Key:', CLAUDE_API_KEY ? 'Configured' : 'Not configured');
    console.log('WebSocket server ready');
    console.log('Health endpoint: /health');
  });
}

start();
