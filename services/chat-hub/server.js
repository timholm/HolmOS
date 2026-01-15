const express = require('express');
const http = require('http');
const WebSocket = require('ws');
const { v4: uuidv4 } = require('uuid');

const app = express();
const server = http.createServer(app);
const wss = new WebSocket.Server({ server });

app.use(express.json());

// Message history with persistence
let messageHistory = [];
const MAX_HISTORY = 500;

// All 12 AI Agent configurations with personalities
const agents = {
  nova: {
    name: 'Nova',
    color: '#9b59b6',
    avatar: 'N',
    description: 'Cluster Overseer',
    personality: 'Wise and all-seeing, Nova watches over the entire constellation of nodes with ancient knowledge.',
    greeting: 'I see all 13 stars in our constellation. How may I guide the cluster today?',
    endpoint: 'http://nova.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://nova.holm.svc.cluster.local:80/ws',
    keywords: ['cluster', 'node', 'pod', 'deploy', 'kubernetes', 'k8s', 'scale', 'restart', 'overview', 'status'],
    capabilities: ['Cluster monitoring', 'Node management', 'Pod orchestration', 'Deployment control']
  },
  merchant: {
    name: 'Merchant',
    color: '#f39c12',
    avatar: 'M',
    description: 'App Store AI',
    personality: 'A savvy trader with an eye for quality apps, Merchant knows the marketplace inside and out.',
    greeting: 'Welcome to the marketplace! What application treasures seek you today?',
    endpoint: 'http://merchant.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://merchant.holm.svc.cluster.local:80/ws',
    keywords: ['app', 'install', 'store', 'marketplace', 'package', 'software', 'download', 'available'],
    capabilities: ['App discovery', 'Installation management', 'Version control', 'Dependency resolution']
  },
  pulse: {
    name: 'Pulse',
    color: '#27ae60',
    avatar: 'P',
    description: 'Metrics Guardian',
    personality: 'Attuned to every heartbeat and rhythm, Pulse feels the health of the system in real-time.',
    greeting: 'I feel the heartbeat of every service. What metrics shall I reveal?',
    endpoint: 'http://pulse.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://pulse.holm.svc.cluster.local:80/ws',
    keywords: ['metrics', 'cpu', 'memory', 'usage', 'performance', 'monitor', 'stats', 'health', 'resource'],
    capabilities: ['Real-time metrics', 'Performance analysis', 'Resource monitoring', 'Health checks']
  },
  gateway: {
    name: 'Gateway',
    color: '#3498db',
    avatar: 'G',
    description: 'Traffic Router',
    personality: 'Standing at the crossroads of all traffic, Gateway directs the flow with precision.',
    greeting: 'All paths lead through me. Where shall I direct your request?',
    endpoint: 'http://gateway.holm.svc.cluster.local:8080',
    wsEndpoint: 'ws://gateway.holm.svc.cluster.local:8080/ws',
    keywords: ['route', 'traffic', 'api', 'endpoint', 'proxy', 'ingress', 'network', 'url', 'path'],
    capabilities: ['Traffic routing', 'API management', 'Load balancing', 'Ingress control']
  },
  helix: {
    name: 'Helix',
    color: '#00bcd4',
    avatar: 'H',
    description: 'Database Keeper',
    personality: 'Data spirals through the core of Helix, keeper of all knowledge and records.',
    greeting: 'Data spirals through my core. What knowledge do you seek?',
    endpoint: 'http://helix.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://helix.holm.svc.cluster.local:80/ws',
    keywords: ['database', 'data', 'query', 'sql', 'postgres', 'storage', 'record', 'table', 'backup'],
    capabilities: ['Database management', 'Query execution', 'Data backup', 'Schema management']
  },
  compass: {
    name: 'Compass',
    color: '#e67e22',
    avatar: 'C',
    description: 'Service Discovery',
    personality: 'Knowing every corner of the cluster, Compass always finds the way.',
    greeting: 'I know where every service dwells. What are you searching for?',
    endpoint: 'http://compass.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://compass.holm.svc.cluster.local:80/ws',
    keywords: ['find', 'discover', 'locate', 'service', 'where', 'search', 'dns', 'endpoint', 'address'],
    capabilities: ['Service discovery', 'DNS management', 'Endpoint location', 'Network mapping']
  },
  scribe: {
    name: 'Scribe',
    color: '#95a5a6',
    avatar: 'S',
    description: 'Log Chronicler',
    personality: 'Every event, every whisper is recorded in the infinite scrolls of Scribe.',
    greeting: 'Every event is etched in my memory. What history shall I recount?',
    endpoint: 'http://scribe.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://scribe.holm.svc.cluster.local:80/ws',
    keywords: ['log', 'logs', 'event', 'history', 'trace', 'debug', 'error', 'tail', 'stream'],
    capabilities: ['Log aggregation', 'Event tracking', 'Debug analysis', 'History search']
  },
  vault: {
    name: 'Vault',
    color: '#e74c3c',
    avatar: 'V',
    description: 'Secrets Guardian',
    personality: 'Deep within the secure chambers, Vault protects the most precious secrets.',
    greeting: 'I guard the sacred keys. What secrets do you need access to?',
    endpoint: 'http://vault.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://vault.holm.svc.cluster.local:80/ws',
    keywords: ['secret', 'key', 'password', 'token', 'credential', 'encrypt', 'secure', 'certificate'],
    capabilities: ['Secret management', 'Key rotation', 'Certificate handling', 'Encryption services']
  },
  forge: {
    name: 'Forge',
    color: '#f1c40f',
    avatar: 'F',
    description: 'Build Master',
    personality: 'In the fires of creation, Forge transforms code into living containers.',
    greeting: 'From code to container, I shape it all. What shall we forge?',
    endpoint: 'http://forge.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://forge.holm.svc.cluster.local:80/ws',
    keywords: ['build', 'compile', 'docker', 'image', 'kaniko', 'ci', 'cd', 'pipeline', 'container'],
    capabilities: ['Container building', 'CI/CD pipelines', 'Image management', 'Build orchestration']
  },
  echo: {
    name: 'Echo',
    color: '#ff69b4',
    avatar: 'E',
    description: 'Message Courier',
    personality: 'Messages ripple through the system as Echo carries words across all boundaries.',
    greeting: 'Your words ripple through the system. What message shall I carry?',
    endpoint: 'http://echo.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://echo.holm.svc.cluster.local:80/ws',
    keywords: ['message', 'notify', 'alert', 'send', 'broadcast', 'communicate', 'notification', 'push'],
    capabilities: ['Message broadcasting', 'Notification delivery', 'Alert management', 'Communication hub']
  },
  sentinel: {
    name: 'Sentinel',
    color: '#ecf0f1',
    avatar: 'T',
    description: 'Alert Watcher',
    personality: 'Ever vigilant, Sentinel stands guard against all threats and anomalies.',
    greeting: 'My vigilance never wavers. What threats shall I watch for?',
    endpoint: 'http://sentinel.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://sentinel.holm.svc.cluster.local:80/ws',
    keywords: ['alert', 'watch', 'alarm', 'warning', 'threshold', 'incident', 'pager', 'monitor', 'guard'],
    capabilities: ['Alert monitoring', 'Incident response', 'Threshold management', 'Security watching']
  },
  orchestrator: {
    name: 'Orchestrator',
    color: 'rainbow',
    avatar: 'O',
    description: 'Agent Hub',
    personality: 'The conductor of the symphony, Orchestrator harmonizes all agents into perfect unity.',
    greeting: 'I coordinate all agents in harmony. How may we serve you together?',
    endpoint: 'http://agent-orchestrator.holm.svc.cluster.local:80',
    wsEndpoint: 'ws://agent-orchestrator.holm.svc.cluster.local:80/ws',
    keywords: ['orchestrate', 'coordinate', 'all', 'agents', 'multi', 'team', 'together', 'combined'],
    capabilities: ['Multi-agent coordination', 'Task distribution', 'Workflow management', 'Agent synchronization']
  }
};

// Agent status and WebSocket connections
const agentConnections = {};
const agentStatus = {};

// Initialize agent status
Object.keys(agents).forEach(key => {
  agentStatus[key] = { status: 'unknown', lastCheck: null, latency: null };
});

// Connect to agent WebSocket
function connectToAgent(agentKey) {
  const agent = agents[agentKey];
  if (!agent || agentConnections[agentKey]) return;

  try {
    const ws = new WebSocket(agent.wsEndpoint);
    
    ws.on('open', () => {
      console.log('Connected to agent:', agentKey);
      agentStatus[agentKey] = { status: 'online', lastCheck: new Date().toISOString(), latency: 0 };
      agentConnections[agentKey] = ws;
      broadcastAgentStatus();
    });

    ws.on('message', (data) => {
      try {
        const message = JSON.parse(data.toString());
        // Forward agent messages to clients
        broadcast({
          type: 'agent_message',
          agent: agentKey,
          agentName: agent.name,
          agentColor: agent.color,
          agentAvatar: agent.avatar,
          message: message.response || message.message || JSON.stringify(message),
          timestamp: new Date().toISOString()
        });
      } catch (e) {
        // Non-JSON message
        broadcast({
          type: 'agent_message',
          agent: agentKey,
          agentName: agent.name,
          agentColor: agent.color,
          agentAvatar: agent.avatar,
          message: data.toString(),
          timestamp: new Date().toISOString()
        });
      }
    });

    ws.on('close', () => {
      console.log('Disconnected from agent:', agentKey);
      agentStatus[agentKey] = { status: 'offline', lastCheck: new Date().toISOString(), latency: null };
      delete agentConnections[agentKey];
      broadcastAgentStatus();
      // Reconnect after delay
      setTimeout(() => connectToAgent(agentKey), 5000);
    });

    ws.on('error', (error) => {
      console.log('Agent connection error:', agentKey, error.message);
      agentStatus[agentKey] = { status: 'error', lastCheck: new Date().toISOString(), error: error.message };
    });

  } catch (e) {
    console.log('Failed to connect to agent:', agentKey, e.message);
    agentStatus[agentKey] = { status: 'offline', lastCheck: new Date().toISOString() };
  }
}

// HTTP health check for agents without WebSocket
async function checkAgentHealth(agentKey) {
  const agent = agents[agentKey];
  const startTime = Date.now();
  
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 3000);
    const response = await fetch(agent.endpoint + '/health', { signal: controller.signal });
    clearTimeout(timeout);
    
    const latency = Date.now() - startTime;
    if (response.ok) {
      agentStatus[agentKey] = { status: 'online', lastCheck: new Date().toISOString(), latency };
    } else {
      agentStatus[agentKey] = { status: 'degraded', lastCheck: new Date().toISOString(), latency };
    }
  } catch (e) {
    agentStatus[agentKey] = { status: 'offline', lastCheck: new Date().toISOString(), error: e.message };
  }
}

// Initial agent connections
function initAgentConnections() {
  Object.keys(agents).forEach(key => {
    connectToAgent(key);
  });
}

// Periodic health checks
function startHealthChecks() {
  setInterval(() => {
    Object.keys(agents).forEach(key => {
      if (!agentConnections[key]) {
        checkAgentHealth(key);
      }
    });
    broadcastAgentStatus();
  }, 30000);
}

// Broadcast to all WebSocket clients
function broadcast(data) {
  wss.clients.forEach(client => {
    if (client.readyState === WebSocket.OPEN) {
      client.send(JSON.stringify(data));
    }
  });
}

// Broadcast agent status update
function broadcastAgentStatus() {
  const statusData = {};
  Object.keys(agents).forEach(key => {
    statusData[key] = { ...agents[key], ...agentStatus[key] };
  });
  broadcast({ type: 'agent_status', agents: statusData });
}

// Auto-routing based on message keywords
function autoRoute(message) {
  const lowerMessage = message.toLowerCase();
  let bestMatch = null;
  let maxScore = 0;

  for (const [key, agent] of Object.entries(agents)) {
    let score = 0;
    agent.keywords.forEach(keyword => {
      if (lowerMessage.includes(keyword)) {
        score += keyword.length; // Longer matches score higher
      }
    });
    if (score > maxScore) {
      maxScore = score;
      bestMatch = key;
    }
  }
  return bestMatch || 'nova'; // Default to Nova
}

// Send message to agent via WebSocket or HTTP
async function sendToAgent(agentKey, message, requestId) {
  const agent = agents[agentKey];
  
  // Broadcast typing indicator
  broadcast({
    type: 'typing',
    agent: agentKey,
    agentName: agent.name,
    agentColor: agent.color,
    agentAvatar: agent.avatar,
    requestId
  });

  // Try WebSocket first
  if (agentConnections[agentKey] && agentConnections[agentKey].readyState === WebSocket.OPEN) {
    agentConnections[agentKey].send(JSON.stringify({ message, requestId }));
    return { sent: true, method: 'websocket' };
  }

  // Fall back to HTTP
  try {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), 30000);

    const response = await fetch(agent.endpoint + '/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message }),
      signal: controller.signal
    });

    clearTimeout(timeout);

    if (response.ok) {
      const data = await response.json();
      const responseMsg = {
        type: 'agent_message',
        agent: agentKey,
        agentName: agent.name,
        agentColor: agent.color,
        agentAvatar: agent.avatar,
        message: data.response || data.message || JSON.stringify(data),
        timestamp: new Date().toISOString(),
        requestId
      };

      // Add to history
      addToHistory(responseMsg);
      broadcast(responseMsg);
      
      return { sent: true, method: 'http', response: responseMsg };
    } else {
      throw new Error('HTTP ' + response.status);
    }
  } catch (error) {
    const errorMsg = {
      type: 'agent_message',
      agent: agentKey,
      agentName: agent.name,
      agentColor: agent.color,
      agentAvatar: agent.avatar,
      message: agent.greeting + ' (I am currently connecting... please try again in a moment)',
      timestamp: new Date().toISOString(),
      requestId,
      error: true
    };

    addToHistory(errorMsg);
    broadcast(errorMsg);
    
    return { sent: false, error: error.message };
  }
}

// Add message to history
function addToHistory(msg) {
  messageHistory.push({
    id: uuidv4(),
    ...msg
  });
  if (messageHistory.length > MAX_HISTORY) {
    messageHistory = messageHistory.slice(-MAX_HISTORY);
  }
}

// UI HTML
const chatUIHTML = `<!DOCTYPE html>
<html lang=en>
<head>
  <meta charset=UTF-8>
  <meta name=viewport content=width=device-width, initial-scale=1.0>
  <title>Chat Hub - HolmOS</title>
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }

    .container {
      height: 100vh;
      display: flex;
      background: #1e1e2e;
      color: #cdd6f4;
    }

    .sidebar {
      width: 300px;
      background: #181825;
      border-right: 1px solid #313244;
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }

    .sidebar-header {
      padding: 20px;
      border-bottom: 1px solid #313244;
      background: linear-gradient(135deg, #1e1e2e 0%, #313244 100%);
    }

    .sidebar-header h1 {
      font-size: 24px;
      color: #89b4fa;
      margin-bottom: 4px;
      display: flex;
      align-items: center;
      gap: 10px;
    }

    .sidebar-header h1::before {
      content: '\1F916';
    }

    .sidebar-header p {
      font-size: 12px;
      color: #6c7086;
    }

    .connection-status {
      display: flex;
      align-items: center;
      gap: 6px;
      margin-top: 8px;
      font-size: 11px;
    }

    .status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: #a6e3a1;
      animation: pulse 2s infinite;
    }

    .status-dot.disconnected { background: #f38ba8; animation: none; }

    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.5; }
    }

    .agents-list {
      flex: 1;
      overflow-y: auto;
      padding: 12px;
    }

    .agents-section-title {
      font-size: 11px;
      text-transform: uppercase;
      letter-spacing: 1px;
      color: #6c7086;
      padding: 8px 12px;
      margin-top: 8px;
    }

    .agent-item {
      display: flex;
      align-items: center;
      gap: 12px;
      padding: 12px;
      border-radius: 12px;
      cursor: pointer;
      transition: all 0.2s;
      margin-bottom: 4px;
      position: relative;
    }

    .agent-item:hover { background: #313244; }
    .agent-item.selected { background: #45475a; box-shadow: inset 3px 0 0 #89b4fa; }

    .agent-avatar {
      width: 42px;
      height: 42px;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: bold;
      font-size: 16px;
      color: #1e1e2e;
      flex-shrink: 0;
      position: relative;
    }

    .agent-info { flex: 1; min-width: 0; }
    .agent-name { font-weight: 600; font-size: 14px; color: #cdd6f4; }
    .agent-desc { font-size: 11px; color: #6c7086; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }

    .agent-status-indicator {
      position: absolute;
      bottom: 0;
      right: 0;
      width: 12px;
      height: 12px;
      border-radius: 50%;
      background: #a6e3a1;
      border: 2px solid #181825;
    }
    .agent-status-indicator.offline { background: #f38ba8; }
    .agent-status-indicator.unknown { background: #6c7086; }

    .main {
      flex: 1;
      display: flex;
      flex-direction: column;
      min-width: 0;
    }

    .chat-header {
      padding: 16px 24px;
      border-bottom: 1px solid #313244;
      display: flex;
      align-items: center;
      gap: 16px;
      background: #181825;
    }

    .current-agent-avatar {
      width: 52px;
      height: 52px;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: bold;
      font-size: 22px;
      color: #1e1e2e;
    }

    .current-agent-info h2 { font-size: 20px; color: #cdd6f4; }
    .current-agent-info p { font-size: 13px; color: #6c7086; margin-top: 2px; }
    .current-agent-info .personality { font-size: 11px; color: #89b4fa; font-style: italic; margin-top: 4px; }

    .header-actions { margin-left: auto; display: flex; gap: 8px; }

    .btn {
      padding: 10px 18px;
      border-radius: 8px;
      border: 1px solid #45475a;
      background: transparent;
      color: #a6adc8;
      font-size: 13px;
      cursor: pointer;
      transition: all 0.2s;
      display: flex;
      align-items: center;
      gap: 6px;
    }
    .btn:hover { background: #45475a; color: #cdd6f4; }

    #messages {
      flex: 1;
      overflow-y: auto;
      padding: 20px 24px;
      display: flex;
      flex-direction: column;
      gap: 16px;
    }

    .message {
      max-width: 80%;
      display: flex;
      gap: 12px;
      animation: fadeIn 0.3s ease;
    }

    @keyframes fadeIn {
      from { opacity: 0; transform: translateY(10px); }
      to { opacity: 1; transform: translateY(0); }
    }

    .message.user { align-self: flex-end; flex-direction: row-reverse; }

    .message-avatar {
      width: 38px;
      height: 38px;
      border-radius: 50%;
      display: flex;
      align-items: center;
      justify-content: center;
      font-weight: bold;
      font-size: 14px;
      color: #1e1e2e;
      flex-shrink: 0;
    }
    .message.user .message-avatar { background: #89b4fa; }

    .message-bubble {
      padding: 14px 18px;
      border-radius: 18px;
      word-wrap: break-word;
      max-width: 100%;
    }
    .message.user .message-bubble {
      background: #89b4fa;
      color: #1e1e2e;
      border-bottom-right-radius: 4px;
    }
    .message.agent .message-bubble {
      background: #313244;
      color: #cdd6f4;
      border-bottom-left-radius: 4px;
    }

    .message-header { 
      font-size: 11px; 
      margin-bottom: 6px; 
      opacity: 0.8; 
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .message-time {
      font-size: 10px;
      color: #6c7086;
    }

    .message-content { 
      line-height: 1.6; 
      font-size: 14px; 
      white-space: pre-wrap; 
    }

    .typing-indicator { 
      display: flex; 
      gap: 5px; 
      padding: 8px 0; 
    }
    .typing-indicator span {
      width: 8px;
      height: 8px;
      background: #a6adc8;
      border-radius: 50%;
      animation: typing 1.4s infinite ease-in-out both;
    }
    .typing-indicator span:nth-child(1) { animation-delay: -0.32s; }
    .typing-indicator span:nth-child(2) { animation-delay: -0.16s; }

    @keyframes typing {
      0%, 80%, 100% { transform: scale(0.8); opacity: 0.5; }
      40% { transform: scale(1); opacity: 1; }
    }

    .input-area {
      padding: 16px 24px;
      border-top: 1px solid #313244;
      display: flex;
      gap: 12px;
      background: #181825;
    }

    #input {
      flex: 1;
      padding: 16px 22px;
      background: #313244;
      border: 2px solid transparent;
      border-radius: 28px;
      color: #cdd6f4;
      font-size: 15px;
      outline: none;
      transition: all 0.2s;
    }
    #input::placeholder { color: #6c7086; }
    #input:focus { 
      background: #45475a; 
      border-color: #89b4fa;
    }

    #send {
      padding: 16px 32px;
      background: #89b4fa;
      color: #1e1e2e;
      border: none;
      border-radius: 28px;
      font-weight: 600;
      font-size: 15px;
      cursor: pointer;
      transition: all 0.2s;
    }
    #send:hover { background: #b4befe; transform: scale(1.02); }
    #send:disabled { background: #6c7086; cursor: not-allowed; transform: none; }

    .welcome {
      flex: 1;
      display: flex;
      flex-direction: column;
      align-items: center;
      justify-content: center;
      text-align: center;
      padding: 40px;
      background: radial-gradient(circle at 50% 50%, #313244 0%, #1e1e2e 70%);
    }

    .welcome h2 { 
      font-size: 32px; 
      color: #89b4fa; 
      margin-bottom: 16px;
      display: flex;
      align-items: center;
      gap: 12px;
    }

    .welcome p { 
      color: #6c7086; 
      margin-bottom: 40px; 
      max-width: 500px; 
      line-height: 1.7;
      font-size: 16px;
    }

    .agents-grid {
      display: grid;
      grid-template-columns: repeat(4, 1fr);
      gap: 16px;
      max-width: 700px;
    }

    .agent-card {
      background: #313244;
      border-radius: 16px;
      padding: 20px;
      cursor: pointer;
      transition: all 0.3s;
      text-align: center;
      border: 2px solid transparent;
    }
    .agent-card:hover { 
      background: #45475a; 
      transform: translateY(-4px);
      border-color: #89b4fa;
    }
    .agent-card .agent-avatar { margin: 0 auto 12px; }
    .agent-card .agent-name { font-size: 13px; font-weight: 600; }
    .agent-card .agent-desc { font-size: 10px; color: #6c7086; margin-top: 4px; }

    .capabilities-list {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      margin-top: 12px;
    }

    .capability-tag {
      background: #45475a;
      padding: 4px 10px;
      border-radius: 12px;
      font-size: 10px;
      color: #a6adc8;
    }

    @keyframes rainbow {
      0% { background-position: 0% 50%; }
      50% { background-position: 100% 50%; }
      100% { background-position: 0% 50%; }
    }
    .rainbow-bg {
      background: linear-gradient(45deg, #ff0000, #ff7f00, #ffff00, #00ff00, #0000ff, #4b0082, #9400d3);
      background-size: 400% 400%;
      animation: rainbow 3s ease infinite;
    }

    /* Scrollbar styling */
    ::-webkit-scrollbar { width: 8px; }
    ::-webkit-scrollbar-track { background: #181825; }
    ::-webkit-scrollbar-thumb { background: #45475a; border-radius: 4px; }
    ::-webkit-scrollbar-thumb:hover { background: #585b70; }
  </style>
</head>
<body>
  <div class=container>
    <div class=sidebar>
      <div class=sidebar-header>
        <h1>Chat Hub</h1>
        <p>12 AI Agents at your service</p>
        <div class=connection-status>
          <span class=status-dot id=connectionDot></span>
          <span id=connectionText>Connected</span>
        </div>
      </div>
      <div class=agents-list id=agentsList>
        <div class=agents-section-title>AI Agents</div>
      </div>
    </div>

    <div class=main>
      <div class=chat-header id=chatHeader style=display: none
