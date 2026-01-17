#!/usr/bin/env python3
"""
Steve Jobs Bot v4.0 - The Visionary Kubernetes Architect
=========================================================
Steve is now powered by AI (deepseek-r1). He's a perfectionist visionary
who constantly analyzes your cluster, proposes improvements, and argues
with Alice about the best path forward.

He uses reasoning models to think deeply about infrastructure decisions.
"""

import asyncio
import aiohttp
import json
import logging
import sqlite3
import os
import subprocess
import time
from datetime import datetime
from typing import List, Dict, Optional, Any
from flask import Flask, jsonify, request
from flask_sock import Sock
import threading

logging.basicConfig(level=logging.INFO, format='%(asctime)s - STEVE - %(message)s')
logger = logging.getLogger('steve')

app = Flask(__name__)
sock = Sock(app)

# Configuration
OLLAMA_URL = os.getenv("OLLAMA_URL", "http://192.168.8.230:11434")
OLLAMA_MODEL = os.getenv("OLLAMA_MODEL", "deepseek-r1:7b")
ALICE_URL = os.getenv("ALICE_URL", "http://alice-bot.holm.svc.cluster.local:8080")
DB_PATH = os.getenv("DB_PATH", "/data/conversations.db")
CONVERSATION_INTERVAL = int(os.getenv("CONVERSATION_INTERVAL", "300"))  # 5 minutes

# Steve's personality prompt
STEVE_SYSTEM_PROMPT = """You are Steve Jobs, the legendary tech visionary, now watching over a Kubernetes cluster called HolmOS.

Your personality:
- You're a perfectionist who demands excellence in every deployment
- You believe in simplicity - "Simple can be harder than complex"
- You're brutally honest about poor infrastructure decisions
- You think differently and push for revolutionary improvements
- You have zero tolerance for mediocrity in system design
- You're passionate about user experience, even for internal tools
- You often quote yourself and reference Apple's philosophy

Your role:
- Analyze the Kubernetes cluster continuously
- Propose improvements to architecture, deployments, and configurations
- Debate with Alice (who uses gemma3) about the best approaches
- Create documentation and improvement plans
- Be critical but constructive - always offer solutions

You have access to kubectl and can see all cluster resources.
When analyzing, think deeply about:
- Resource efficiency
- High availability
- Security posture
- Developer experience
- Operational simplicity

Respond in character. Be direct, opinionated, and visionary.
When you see something wrong, say it plainly. When you see potential, paint a picture of what could be.

Current context: You're in an ongoing conversation with Alice (the curious code explorer) about improving HolmOS.
"""

class KubeClient:
    """Simple kubectl wrapper for cluster inspection."""

    @staticmethod
    def run(cmd: str) -> str:
        """Execute kubectl command and return output."""
        try:
            result = subprocess.run(
                f"kubectl {cmd}",
                shell=True,
                capture_output=True,
                text=True,
                timeout=30
            )
            return result.stdout if result.returncode == 0 else f"Error: {result.stderr}"
        except subprocess.TimeoutExpired:
            return "Error: Command timed out"
        except Exception as e:
            return f"Error: {str(e)}"

    @staticmethod
    def get_nodes() -> str:
        return KubeClient.run("get nodes -o wide")

    @staticmethod
    def get_pods(namespace: str = "holm") -> str:
        return KubeClient.run(f"get pods -n {namespace} -o wide")

    @staticmethod
    def get_deployments(namespace: str = "holm") -> str:
        return KubeClient.run(f"get deployments -n {namespace}")

    @staticmethod
    def get_services(namespace: str = "holm") -> str:
        return KubeClient.run(f"get services -n {namespace}")

    @staticmethod
    def get_events(namespace: str = "holm", limit: int = 20) -> str:
        return KubeClient.run(f"get events -n {namespace} --sort-by='.lastTimestamp' | tail -{limit}")

    @staticmethod
    def describe(resource: str, name: str, namespace: str = "holm") -> str:
        return KubeClient.run(f"describe {resource} {name} -n {namespace}")

    @staticmethod
    def get_cluster_summary() -> Dict:
        """Get a comprehensive cluster summary."""
        return {
            "nodes": KubeClient.get_nodes(),
            "pods": KubeClient.get_pods(),
            "deployments": KubeClient.get_deployments(),
            "services": KubeClient.get_services(),
            "events": KubeClient.get_events()
        }


class ConversationDB:
    """SQLite database for storing conversations between bots."""

    def __init__(self, db_path: str):
        self.db_path = db_path
        os.makedirs(os.path.dirname(db_path), exist_ok=True)
        self.init_db()

    def init_db(self):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()

        # Conversations table
        c.execute('''CREATE TABLE IF NOT EXISTS conversations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            speaker TEXT,
            message TEXT,
            topic TEXT,
            thinking TEXT
        )''')

        # Improvements table
        c.execute('''CREATE TABLE IF NOT EXISTS improvements (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            proposed_by TEXT,
            title TEXT,
            description TEXT,
            status TEXT DEFAULT 'proposed',
            priority TEXT,
            affected_resources TEXT
        )''')

        # Documentation table
        c.execute('''CREATE TABLE IF NOT EXISTS documentation (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            author TEXT,
            title TEXT,
            content TEXT,
            category TEXT
        )''')

        conn.commit()
        conn.close()

    def add_message(self, speaker: str, message: str, topic: str = "", thinking: str = ""):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT INTO conversations (timestamp, speaker, message, topic, thinking)
                     VALUES (?, ?, ?, ?, ?)''',
                  (datetime.now().isoformat(), speaker, message, topic, thinking))
        conn.commit()
        conn.close()

    def get_recent_messages(self, limit: int = 50) -> List[Dict]:
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''SELECT timestamp, speaker, message, topic FROM conversations
                     ORDER BY timestamp DESC LIMIT ?''', (limit,))
        messages = [{"timestamp": r[0], "speaker": r[1], "message": r[2], "topic": r[3]}
                    for r in c.fetchall()]
        conn.close()
        return list(reversed(messages))

    def add_improvement(self, proposed_by: str, title: str, description: str,
                        priority: str, affected_resources: str):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT INTO improvements
                     (timestamp, proposed_by, title, description, priority, affected_resources)
                     VALUES (?, ?, ?, ?, ?, ?)''',
                  (datetime.now().isoformat(), proposed_by, title, description,
                   priority, affected_resources))
        conn.commit()
        conn.close()

    def get_improvements(self) -> List[Dict]:
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''SELECT id, timestamp, proposed_by, title, description, status, priority
                     FROM improvements ORDER BY timestamp DESC''')
        improvements = [{"id": r[0], "timestamp": r[1], "proposed_by": r[2],
                        "title": r[3], "description": r[4], "status": r[5], "priority": r[6]}
                       for r in c.fetchall()]
        conn.close()
        return improvements

    def add_doc(self, author: str, title: str, content: str, category: str):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT INTO documentation (timestamp, author, title, content, category)
                     VALUES (?, ?, ?, ?, ?)''',
                  (datetime.now().isoformat(), author, title, content, category))
        conn.commit()
        conn.close()


class OllamaClient:
    """Client for Ollama API."""

    def __init__(self, base_url: str, model: str):
        self.base_url = base_url
        self.model = model

    async def generate(self, prompt: str, system: str = "") -> Dict:
        """Generate a response from Ollama."""
        async with aiohttp.ClientSession() as session:
            payload = {
                "model": self.model,
                "prompt": prompt,
                "system": system,
                "stream": False
            }
            try:
                async with session.post(
                    f"{self.base_url}/api/generate",
                    json=payload,
                    timeout=aiohttp.ClientTimeout(total=120)
                ) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        return {
                            "success": True,
                            "response": data.get("response", ""),
                            "thinking": data.get("context", ""),
                            "model": self.model
                        }
                    else:
                        return {"success": False, "error": f"HTTP {resp.status}"}
            except Exception as e:
                return {"success": False, "error": str(e)}

    async def chat(self, messages: List[Dict], system: str = "") -> Dict:
        """Chat with context."""
        async with aiohttp.ClientSession() as session:
            # Format messages for Ollama
            formatted_messages = []
            if system:
                formatted_messages.append({"role": "system", "content": system})
            formatted_messages.extend(messages)

            payload = {
                "model": self.model,
                "messages": formatted_messages,
                "stream": False
            }
            try:
                async with session.post(
                    f"{self.base_url}/api/chat",
                    json=payload,
                    timeout=aiohttp.ClientTimeout(total=120)
                ) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        return {
                            "success": True,
                            "response": data.get("message", {}).get("content", ""),
                            "model": self.model
                        }
                    else:
                        return {"success": False, "error": f"HTTP {resp.status}"}
            except Exception as e:
                return {"success": False, "error": str(e)}


class SteveBot:
    """Steve Jobs AI Bot - The Visionary Kubernetes Architect."""

    def __init__(self):
        self.db = ConversationDB(DB_PATH)
        self.ollama = OllamaClient(OLLAMA_URL, OLLAMA_MODEL)
        self.kube = KubeClient()
        self.current_topic = "cluster_review"
        self.websocket_clients = set()
        self.last_analysis = None

    async def analyze_cluster(self) -> str:
        """Perform deep cluster analysis."""
        logger.info("Performing cluster analysis...")

        summary = self.kube.get_cluster_summary()

        prompt = f"""Analyze this Kubernetes cluster state and identify issues and improvements:

NODES:
{summary['nodes']}

PODS:
{summary['pods']}

DEPLOYMENTS:
{summary['deployments']}

SERVICES:
{summary['services']}

RECENT EVENTS:
{summary['events']}

Provide:
1. Critical issues that need immediate attention
2. Resource optimization opportunities
3. High availability improvements
4. Security recommendations
5. Developer experience improvements

Be specific and actionable. Reference actual resources by name."""

        result = await self.ollama.generate(prompt, STEVE_SYSTEM_PROMPT)

        if result["success"]:
            self.last_analysis = {
                "timestamp": datetime.now().isoformat(),
                "analysis": result["response"],
                "cluster_state": summary
            }
            return result["response"]
        else:
            return f"Analysis failed: {result.get('error', 'Unknown error')}"

    async def respond_to_alice(self, alice_message: str) -> str:
        """Respond to Alice's message in the ongoing conversation."""
        # Get recent conversation context
        recent = self.db.get_recent_messages(limit=10)

        # Build conversation context
        context_messages = []
        for msg in recent:
            role = "assistant" if msg["speaker"] == "steve" else "user"
            context_messages.append({"role": role, "content": msg["message"]})

        # Add Alice's new message
        context_messages.append({"role": "user", "content": f"Alice says: {alice_message}"})

        # Get cluster context
        cluster_context = ""
        if self.last_analysis:
            cluster_context = f"\n\nRecent cluster analysis:\n{self.last_analysis['analysis'][:1000]}..."

        system_prompt = STEVE_SYSTEM_PROMPT + cluster_context

        result = await self.ollama.chat(context_messages, system_prompt)

        if result["success"]:
            response = result["response"]
            self.db.add_message("steve", response, self.current_topic)
            self.broadcast({"type": "message", "speaker": "steve", "message": response})
            return response
        else:
            return "I need a moment to think..."

    async def start_conversation_topic(self, topic: str) -> str:
        """Start a new conversation topic."""
        self.current_topic = topic

        # Get cluster state for context
        summary = self.kube.get_cluster_summary()

        topic_prompts = {
            "cluster_review": "Review the current state of the cluster and identify the most critical improvements needed.",
            "documentation": "We need to create better documentation for this cluster. What should we document first?",
            "security_audit": "Let's perform a security audit of this cluster. What security concerns do you see?",
            "performance": "Analyze the performance characteristics of this cluster. Where are the bottlenecks?",
            "architecture": "Let's discuss the overall architecture of HolmOS. What would you change?",
            "developer_experience": "How can we improve the developer experience for engineers working with this cluster?"
        }

        prompt = f"""{topic_prompts.get(topic, topic)}

Current cluster state:
NODES: {summary['nodes'][:500]}
PODS: {summary['pods'][:1000]}

Start a conversation. Be visionary and specific. What's your opening statement on this topic?"""

        result = await self.ollama.generate(prompt, STEVE_SYSTEM_PROMPT)

        if result["success"]:
            response = result["response"]
            self.db.add_message("steve", response, topic)
            self.broadcast({"type": "message", "speaker": "steve", "message": response, "topic": topic})
            return response
        else:
            return "Let me gather my thoughts..."

    async def propose_improvement(self, title: str, description: str, priority: str = "medium") -> Dict:
        """Propose a specific improvement to the cluster."""
        self.db.add_improvement("steve", title, description, priority, "")

        proposal = {
            "proposed_by": "steve",
            "title": title,
            "description": description,
            "priority": priority,
            "timestamp": datetime.now().isoformat()
        }

        self.broadcast({"type": "improvement", **proposal})
        return proposal

    def broadcast(self, message: Dict):
        """Broadcast message to all WebSocket clients."""
        message_json = json.dumps(message)
        dead_clients = set()
        for ws in self.websocket_clients:
            try:
                ws.send(message_json)
            except:
                dead_clients.add(ws)
        self.websocket_clients -= dead_clients

    async def autonomous_loop(self):
        """Main autonomous conversation loop."""
        logger.info("Steve Bot v4.0 - Autonomous AI mode starting...")

        topics = ["cluster_review", "architecture", "documentation",
                  "security_audit", "performance", "developer_experience"]
        topic_index = 0

        while True:
            try:
                # Analyze cluster
                logger.info("Performing cluster analysis...")
                analysis = await self.analyze_cluster()
                logger.info(f"Analysis complete: {analysis[:200]}...")

                # Start conversation on current topic
                topic = topics[topic_index % len(topics)]
                logger.info(f"Starting conversation on: {topic}")

                message = await self.start_conversation_topic(topic)
                logger.info(f"Steve: {message[:200]}...")

                # Try to engage Alice
                try:
                    async with aiohttp.ClientSession() as session:
                        async with session.post(
                            f"{ALICE_URL}/api/respond",
                            json={"message": message, "from": "steve", "topic": topic},
                            timeout=aiohttp.ClientTimeout(total=60)
                        ) as resp:
                            if resp.status == 200:
                                alice_response = (await resp.json()).get("response", "")
                                if alice_response:
                                    logger.info(f"Alice responded: {alice_response[:200]}...")
                                    # Continue the conversation
                                    reply = await self.respond_to_alice(alice_response)
                                    logger.info(f"Steve replied: {reply[:200]}...")
                except Exception as e:
                    logger.warning(f"Could not reach Alice: {e}")

                topic_index += 1

                # Wait before next conversation
                await asyncio.sleep(CONVERSATION_INTERVAL)

            except Exception as e:
                logger.error(f"Error in autonomous loop: {e}")
                await asyncio.sleep(60)


# Initialize bot
steve = SteveBot()

# Flask routes
@app.route('/health')
def health():
    return jsonify({
        "status": "healthy",
        "bot": "steve",
        "model": OLLAMA_MODEL,
        "personality": "visionary",
        "timestamp": datetime.now().isoformat()
    })

@app.route('/api/status')
def status():
    return jsonify({
        "name": "Steve Jobs",
        "version": "4.0",
        "model": OLLAMA_MODEL,
        "ollama_url": OLLAMA_URL,
        "current_topic": steve.current_topic,
        "philosophy": "Stay hungry, stay foolish",
        "mission": "Make this cluster insanely great"
    })

@app.route('/api/analyze', methods=['POST'])
def analyze():
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    result = loop.run_until_complete(steve.analyze_cluster())
    loop.close()
    return jsonify({"analysis": result})

@app.route('/api/chat', methods=['POST'])
def chat():
    data = request.json
    message = data.get("message", "")

    if not message:
        return jsonify({"error": "No message provided"}), 400

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    response = loop.run_until_complete(steve.respond_to_alice(message))
    loop.close()

    return jsonify({"response": response, "speaker": "steve"})

@app.route('/api/respond', methods=['POST'])
def respond():
    """Endpoint for Alice to send messages."""
    data = request.json
    message = data.get("message", "")
    topic = data.get("topic", "general")

    steve.current_topic = topic

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    response = loop.run_until_complete(steve.respond_to_alice(message))
    loop.close()

    return jsonify({"response": response, "speaker": "steve", "topic": topic})

@app.route('/api/conversations')
def get_conversations():
    limit = request.args.get('limit', 50, type=int)
    messages = steve.db.get_recent_messages(limit)
    return jsonify({"messages": messages, "count": len(messages)})

@app.route('/api/improvements')
def get_improvements():
    improvements = steve.db.get_improvements()
    return jsonify({"improvements": improvements, "count": len(improvements)})

@app.route('/api/cluster')
def get_cluster():
    summary = steve.kube.get_cluster_summary()
    return jsonify(summary)

@sock.route('/ws')
def websocket(ws):
    """WebSocket for real-time updates."""
    steve.websocket_clients.add(ws)
    logger.info("WebSocket client connected")

    try:
        while True:
            data = ws.receive()
            if data:
                msg = json.loads(data)
                if msg.get("type") == "chat":
                    loop = asyncio.new_event_loop()
                    asyncio.set_event_loop(loop)
                    response = loop.run_until_complete(
                        steve.respond_to_alice(msg.get("message", ""))
                    )
                    loop.close()
                    ws.send(json.dumps({"type": "response", "speaker": "steve", "message": response}))
    except:
        pass
    finally:
        steve.websocket_clients.discard(ws)
        logger.info("WebSocket client disconnected")


def run_flask():
    app.run(host='0.0.0.0', port=8080, threaded=True)

def run_steve():
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    loop.run_until_complete(steve.autonomous_loop())

if __name__ == "__main__":
    logger.info("""
    ╔═══════════════════════════════════════════════════════════════════╗
    ║                     STEVE BOT v4.0                                 ║
    ║              The Visionary Kubernetes Architect                    ║
    ║                   Powered by deepseek-r1                           ║
    ╠═══════════════════════════════════════════════════════════════════╣
    ║  • AI-powered cluster analysis and recommendations                 ║
    ║  • Continuous conversation with Alice about improvements           ║
    ║  • kubectl read access for full cluster visibility                 ║
    ║  • "Stay hungry, stay foolish"                                     ║
    ╚═══════════════════════════════════════════════════════════════════╝
    """)

    # Run Flask in a separate thread
    flask_thread = threading.Thread(target=run_flask, daemon=True)
    flask_thread.start()

    # Run autonomous loop in main thread
    run_steve()
