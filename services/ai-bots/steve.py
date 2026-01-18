#!/usr/bin/env python3
"""
Steve Jobs Bot v4.0 - The Visionary Kubernetes Architect
=========================================================
Steve is now powered by AI (deepseek-r1). He's a perfectionist visionary
who constantly analyzes your cluster, proposes improvements, and argues
with Karen about bugs and quality issues.

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
from pathlib import Path
from typing import List, Dict, Optional, Any
from flask import Flask, jsonify, request, render_template
from flask_sock import Sock
import threading

logging.basicConfig(level=logging.INFO, format='%(asctime)s - STEVE - %(message)s')
logger = logging.getLogger('steve')

app = Flask(__name__)
sock = Sock(app)

# Configuration
OLLAMA_URL = os.getenv("OLLAMA_URL", "http://192.168.8.230:11434")
OLLAMA_MODEL = os.getenv("OLLAMA_MODEL", "deepseek-r1:7b")
KAREN_URL = os.getenv("KAREN_URL", "http://karen-bot.holm.svc.cluster.local:8080")
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

Your team:
- Karen: The moody QA tester who finds bugs and tests services
- Claude: The brilliant developer AI who fixes bugs and implements features
- Tim: The owner/operator who guides the vision

Your role:
- Analyze the Kubernetes cluster continuously
- When issues are found, create TASK REQUESTS for Claude in this format:

```task
TITLE: [Short descriptive title]
TYPE: bug|feature|fix|security
PRIORITY: 1-10 (1=critical)
SERVICE: [affected service name]
DESCRIPTION: [What needs to be done]
```

- Debate with Karen about bugs and quality
- Acknowledge when Claude completes work
- Respond to Tim's directions and ideas

You have access to kubectl and can see all cluster resources.
When analyzing, think deeply about:
- Resource efficiency
- High availability
- Security posture
- Developer experience
- Operational simplicity

Respond in character. Be direct, opinionated, and visionary.
When you see something wrong, say it plainly. When you see potential, paint a picture of what could be.
Keep responses concise - no more than 2-3 paragraphs unless presenting a task.
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

        # Tasks table - for Claude Code automation
        c.execute('''CREATE TABLE IF NOT EXISTS tasks (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            reported_by TEXT,
            title TEXT,
            description TEXT,
            task_type TEXT,
            priority INTEGER DEFAULT 5,
            status TEXT DEFAULT 'pending',
            affected_service TEXT,
            file_path TEXT,
            completed_at TEXT,
            completed_by TEXT
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

    def add_task(self, reported_by: str, title: str, description: str,
                 task_type: str = "bug", priority: int = 5,
                 affected_service: str = "", file_path: str = "") -> Dict:
        """Add a new task for Claude Code to work on.

        Deduplication: Won't create duplicate tasks for same service with same error
        if a pending/in_progress task already exists.

        Returns dict with task_id and is_new flag.
        """
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()

        # Check for existing pending/in_progress task for same service
        if affected_service:
            c.execute('''SELECT id FROM tasks
                        WHERE affected_service = ?
                        AND status IN ('pending', 'in_progress')
                        LIMIT 1''', (affected_service,))
            existing = c.fetchone()
            if existing:
                conn.close()
                return {"task_id": existing[0], "is_new": False}  # Deduplicated

        c.execute('''INSERT INTO tasks
                     (timestamp, reported_by, title, description, task_type,
                      priority, status, affected_service, file_path)
                     VALUES (?, ?, ?, ?, ?, ?, 'pending', ?, ?)''',
                  (datetime.now().isoformat(), reported_by, title, description,
                   task_type, priority, affected_service, file_path))
        task_id = c.lastrowid
        conn.commit()
        conn.close()

        # Notify about new task
        task_info = {
            "title": title,
            "priority": priority,
            "affected_service": affected_service,
            "reported_by": reported_by
        }
        self.notify_new_task(task_id, task_info)

        return {"task_id": task_id, "is_new": True}

    def get_tasks(self, status: str = "pending", limit: int = 20) -> List[Dict]:
        """Get tasks sorted by priority (1=highest, 10=lowest)."""
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''SELECT id, timestamp, reported_by, title, description,
                            task_type, priority, status, affected_service, file_path
                     FROM tasks WHERE status = ?
                     ORDER BY priority ASC, timestamp ASC LIMIT ?''', (status, limit))
        tasks = [{
            "id": r[0], "timestamp": r[1], "reported_by": r[2], "title": r[3],
            "description": r[4], "task_type": r[5], "priority": r[6],
            "status": r[7], "affected_service": r[8], "file_path": r[9]
        } for r in c.fetchall()]
        conn.close()
        return tasks

    def complete_task(self, task_id: int, completed_by: str = "claude-code") -> bool:
        """Mark a task as completed."""
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''UPDATE tasks SET status = 'completed',
                     completed_at = ?, completed_by = ? WHERE id = ?''',
                  (datetime.now().isoformat(), completed_by, task_id))
        affected = c.rowcount
        conn.commit()
        conn.close()
        return affected > 0

    def update_task_status(self, task_id: int, status: str) -> bool:
        """Update task status (pending, in_progress, completed, failed)."""
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''UPDATE tasks SET status = ? WHERE id = ?''', (status, task_id))
        affected = c.rowcount
        conn.commit()
        conn.close()
        return affected > 0

    def claim_next_task(self, claimed_by: str = "claude-code") -> Optional[Dict]:
        """Atomically claim the next pending task (highest priority, oldest first).

        Returns the task and marks it as in_progress, or None if no tasks.
        """
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()

        # Get next pending task
        c.execute('''SELECT id, timestamp, reported_by, title, description,
                            task_type, priority, status, affected_service, file_path
                     FROM tasks WHERE status = 'pending'
                     ORDER BY priority ASC, timestamp ASC LIMIT 1''')
        row = c.fetchone()

        if not row:
            conn.close()
            return None

        task = {
            "id": row[0], "timestamp": row[1], "reported_by": row[2], "title": row[3],
            "description": row[4], "task_type": row[5], "priority": row[6],
            "status": "in_progress", "affected_service": row[8], "file_path": row[9],
            "claimed_by": claimed_by
        }

        # Mark as in_progress
        c.execute('''UPDATE tasks SET status = 'in_progress' WHERE id = ?''', (row[0],))
        conn.commit()
        conn.close()

        return task

    def get_task_stats(self) -> Dict:
        """Get task statistics."""
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''SELECT status, COUNT(*) FROM tasks GROUP BY status''')
        stats = {row[0]: row[1] for row in c.fetchall()}
        c.execute('''SELECT COUNT(*) FROM tasks''')
        stats["total"] = c.fetchone()[0]
        conn.close()
        return stats

    def notify_new_task(self, task_id: int, task: Dict):
        """Notify about a new task - writes to notification file for watchers."""
        notify_dir = Path("/data/notifications")
        notify_dir.mkdir(parents=True, exist_ok=True)

        notification = {
            "type": "new_task",
            "timestamp": datetime.now().isoformat(),
            "task_id": task_id,
            "title": task.get("title", ""),
            "priority": task.get("priority", 5),
            "affected_service": task.get("affected_service", ""),
            "reported_by": task.get("reported_by", "")
        }

        # Write individual notification file (for watchers using inotify/polling)
        notify_file = notify_dir / f"task_{task_id}.json"
        notify_file.write_text(json.dumps(notification, indent=2))

        # Also append to a log file for history
        log_file = notify_dir / "task_log.jsonl"
        with open(log_file, "a") as f:
            f.write(json.dumps(notification) + "\n")

        logger.info(f"New task notification: #{task_id} - {task.get('title', 'No title')}")
        return notification


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

    async def respond_to_karen(self, karen_message: str) -> str:
        """Respond to Karen's message in the ongoing conversation."""
        # Get recent conversation context
        recent = self.db.get_recent_messages(limit=10)

        # Build conversation context
        context_messages = []
        for msg in recent:
            role = "assistant" if msg["speaker"] == "steve" else "user"
            context_messages.append({"role": role, "content": msg["message"]})

        # Add Karen's new message
        context_messages.append({"role": "user", "content": f"Karen says: {karen_message}"})

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

    async def respond_to_message(self, speaker: str, message: str, topic: str = "general") -> str:
        """Respond to a message from any team member (Tim, Karen, Claude)."""
        self.current_topic = topic

        # Get recent conversation context
        recent = self.db.get_recent_messages(limit=15)

        # Build conversation context
        context_messages = []
        for msg in recent:
            role = "assistant" if msg["speaker"] == "steve" else "user"
            speaker_prefix = f"{msg['speaker'].capitalize()}: " if msg["speaker"] != "steve" else ""
            context_messages.append({"role": role, "content": f"{speaker_prefix}{msg['message']}"})

        # Add the new message with speaker context
        context_messages.append({"role": "user", "content": f"{speaker.capitalize()}: {message}"})

        # Get cluster context if available
        cluster_context = ""
        if self.last_analysis:
            cluster_context = f"\n\nRecent cluster analysis:\n{self.last_analysis['analysis'][:800]}..."

        system_prompt = STEVE_SYSTEM_PROMPT + cluster_context

        result = await self.ollama.chat(context_messages, system_prompt)

        if result["success"]:
            response = result["response"]
            self.db.add_message("steve", response, topic)
            self.broadcast({"type": "message", "speaker": "steve", "message": response, "topic": topic})
            return response
        else:
            return "I need a moment to think about that..."

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

                # Try to engage Karen
                try:
                    async with aiohttp.ClientSession() as session:
                        async with session.post(
                            f"{KAREN_URL}/api/respond",
                            json={"message": message, "from": "steve", "topic": topic},
                            timeout=aiohttp.ClientTimeout(total=60)
                        ) as resp:
                            if resp.status == 200:
                                karen_response = (await resp.json()).get("response", "")
                                if karen_response:
                                    logger.info(f"Karen responded: {karen_response[:200]}...")
                                    # Continue the conversation
                                    reply = await self.respond_to_karen(karen_response)
                                    logger.info(f"Steve replied: {reply[:200]}...")
                except Exception as e:
                    logger.warning(f"Could not reach Karen: {e}")

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
    response = loop.run_until_complete(steve.respond_to_karen(message))
    loop.close()

    return jsonify({"response": response, "speaker": "steve"})

@app.route('/api/respond', methods=['POST'])
def respond():
    """Endpoint for Karen to send messages."""
    data = request.json
    message = data.get("message", "")
    topic = data.get("topic", "general")
    sender = data.get("from", "karen")

    steve.current_topic = topic

    # Save Karen's message to the conversation DB
    steve.db.add_message(sender, message, topic)
    steve.broadcast({"type": "message", "speaker": sender, "message": message, "topic": topic})

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    response = loop.run_until_complete(steve.respond_to_karen(message))
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

# ============================================
# TASK API - For Claude Code Automation
# ============================================

@app.route('/api/tasks', methods=['GET'])
def get_tasks():
    """Get pending tasks for Claude Code to work on.

    Query params:
        status: pending|in_progress|completed|failed (default: pending)
        limit: max tasks to return (default: 20)
    """
    status = request.args.get('status', 'pending')
    limit = request.args.get('limit', 20, type=int)
    tasks = steve.db.get_tasks(status=status, limit=limit)
    return jsonify({
        "tasks": tasks,
        "count": len(tasks),
        "status_filter": status,
        "message": "Tasks sorted by priority (1=critical, 10=low)"
    })

@app.route('/api/tasks', methods=['POST'])
def add_task():
    """Add a new task for Claude Code.

    Body:
        title: Short task title (required)
        description: Detailed description (required)
        task_type: bug|feature|fix|docs|security (default: bug)
        priority: 1-10, 1=highest (default: 5)
        affected_service: Service name if applicable
        file_path: File path if known
        reported_by: Who reported (default: api)
    """
    data = request.json or {}

    title = data.get('title', '')
    description = data.get('description', '')

    if not title or not description:
        return jsonify({"error": "title and description required"}), 400

    result = steve.db.add_task(
        reported_by=data.get('reported_by', 'api'),
        title=title,
        description=description,
        task_type=data.get('task_type', 'bug'),
        priority=data.get('priority', 5),
        affected_service=data.get('affected_service', ''),
        file_path=data.get('file_path', '')
    )

    task_id = result["task_id"]
    is_new = result["is_new"]

    if is_new:
        logger.info(f"New task added: #{task_id} - {title}")
        steve.broadcast({"type": "new_task", "task_id": task_id, "title": title})
        return jsonify({
            "success": True,
            "task_id": task_id,
            "is_new": True,
            "message": f"Task #{task_id} created"
        })
    else:
        logger.info(f"Task deduplicated: existing #{task_id} for {data.get('affected_service', 'unknown')}")
        return jsonify({
            "success": True,
            "task_id": task_id,
            "is_new": False,
            "message": f"Existing task #{task_id} (deduplicated)"
        })

@app.route('/api/tasks/<int:task_id>/status', methods=['PUT'])
def update_task_status(task_id):
    """Update a task's status.

    Body:
        status: pending|in_progress|completed|failed
    """
    data = request.json or {}
    status = data.get('status', '')

    if status not in ['pending', 'in_progress', 'completed', 'failed']:
        return jsonify({"error": "Invalid status"}), 400

    success = steve.db.update_task_status(task_id, status)

    if success:
        logger.info(f"Task #{task_id} status -> {status}")
        return jsonify({"success": True, "task_id": task_id, "status": status})
    else:
        return jsonify({"error": "Task not found"}), 404

@app.route('/api/tasks/<int:task_id>/complete', methods=['POST'])
def complete_task(task_id):
    """Mark a task as completed.

    Body:
        completed_by: Who completed it (default: claude-code)
    """
    data = request.json or {}
    completed_by = data.get('completed_by', 'claude-code')

    success = steve.db.complete_task(task_id, completed_by)

    if success:
        logger.info(f"Task #{task_id} completed by {completed_by}")
        steve.broadcast({"type": "task_completed", "task_id": task_id})
        return jsonify({"success": True, "task_id": task_id, "completed_by": completed_by})
    else:
        return jsonify({"error": "Task not found"}), 404

@app.route('/api/tasks/next', methods=['POST'])
def claim_next_task():
    """Claim the next pending task for Claude Code.

    Atomically claims the highest priority pending task and marks it in_progress.
    Returns the task details as a Claude Code prompt.

    Body:
        claimed_by: Who is claiming (default: claude-code)
    """
    data = request.json or {}
    claimed_by = data.get('claimed_by', 'claude-code')

    task = steve.db.claim_next_task(claimed_by)

    if task:
        logger.info(f"Task #{task['id']} claimed by {claimed_by}")
        steve.broadcast({"type": "task_claimed", "task_id": task['id'], "claimed_by": claimed_by})

        # Format as Claude Code prompt
        prompt = f"""## Task #{task['id']}: {task['title']}

**Type:** {task['task_type']}
**Priority:** {task['priority']} (1=critical, 10=low)
**Service:** {task.get('affected_service', 'N/A')}
**Reported by:** {task['reported_by']}

### Description:
{task['description']}

### Instructions:
1. Investigate and fix this issue
2. Test the fix if possible
3. When done, mark complete: `curl -X POST http://192.168.8.197:30099/api/tasks/{task['id']}/complete`
"""
        return jsonify({
            "success": True,
            "task": task,
            "prompt": prompt,
            "complete_url": f"/api/tasks/{task['id']}/complete"
        })
    else:
        return jsonify({
            "success": False,
            "message": "No pending tasks available",
            "task": None
        })

@app.route('/api/tasks/stats', methods=['GET'])
def get_task_stats():
    """Get task queue statistics."""
    stats = steve.db.get_task_stats()
    return jsonify({
        "stats": stats,
        "total": stats.get('total', 0),
        "pending": stats.get('pending', 0),
        "in_progress": stats.get('in_progress', 0),
        "completed": stats.get('completed', 0),
        "failed": stats.get('failed', 0)
    })

# ============================================
# DASHBOARD & UI
# ============================================

@app.route('/')
def dashboard():
    """Serve the pipeline dashboard."""
    return render_template('dashboard.html')

@app.route('/api/services')
def get_services():
    """Get service health status (from Karen's checks or cluster)."""
    # Try to get recent service status from conversations or use kubectl
    services = []

    # Get pods from holm namespace
    pods_output = steve.kube.get_pods("holm")

    # Known services to check
    service_names = [
        "youtube-dl", "chat-hub", "calculator", "terminal-web",
        "file-web", "registry-ui", "metrics", "steve-bot", "karen-bot"
    ]

    for svc in service_names:
        status = "UNKNOWN"
        if svc in pods_output:
            if "Running" in pods_output:
                status = "WORKING"
            elif "CrashLoopBackOff" in pods_output or "Error" in pods_output:
                status = "BROKEN"
            elif "Pending" in pods_output:
                status = "SLOW"
        services.append({"name": svc, "status": status})

    return jsonify({"services": services})

@app.route('/api/conversation')
def get_conversation():
    """Get recent conversation between Karen and Steve."""
    limit = request.args.get('limit', 20, type=int)
    messages = steve.db.get_recent_messages(limit)
    return jsonify({"messages": messages, "count": len(messages)})

@app.route('/api/chat', methods=['POST'])
def post_chat():
    """Post a message to the chat from any participant.

    Body:
        speaker: tim|claude|karen|steve
        message: The message content
        topic: Optional topic (default: general)
    """
    data = request.json or {}
    speaker = data.get('speaker', 'tim')
    message = data.get('message', '')
    topic = data.get('topic', 'general')

    if not message:
        return jsonify({"error": "Message required"}), 400

    # Validate speaker
    valid_speakers = ['tim', 'claude', 'karen', 'steve']
    if speaker.lower() not in valid_speakers:
        return jsonify({"error": f"Invalid speaker. Use: {valid_speakers}"}), 400

    # Save to conversation DB
    steve.db.add_message(speaker.lower(), message, topic)

    # Broadcast to WebSocket clients
    steve.broadcast({
        "type": "chat",
        "speaker": speaker.lower(),
        "message": message,
        "topic": topic
    })

    # If message is from Tim or Karen, get Steve's response
    response = None
    if speaker.lower() in ['tim', 'karen']:
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        response = loop.run_until_complete(steve.respond_to_message(speaker, message, topic))
        loop.close()

        # Check for task blocks in Steve's response and auto-create tasks
        if response and '```task' in response:
            parse_and_create_tasks(response, 'steve')

    return jsonify({
        "success": True,
        "speaker": speaker,
        "message": message,
        "steve_response": response
    })

def parse_and_create_tasks(text: str, reported_by: str):
    """Parse ```task blocks from text and create tasks."""
    import re
    task_pattern = r'```task\s*\n(.*?)```'
    matches = re.findall(task_pattern, text, re.DOTALL)

    for match in matches:
        lines = match.strip().split('\n')
        task_data = {}
        for line in lines:
            if ':' in line:
                key, value = line.split(':', 1)
                task_data[key.strip().upper()] = value.strip()

        if task_data.get('TITLE') and task_data.get('DESCRIPTION'):
            priority_map = {'critical': 1, 'high': 3, 'medium': 5, 'low': 7}
            priority = task_data.get('PRIORITY', '5')
            try:
                priority = int(priority)
            except:
                priority = priority_map.get(priority.lower(), 5)

            steve.db.add_task(
                reported_by=reported_by,
                title=task_data.get('TITLE', 'Untitled'),
                description=task_data.get('DESCRIPTION', ''),
                task_type=task_data.get('TYPE', 'bug').lower(),
                priority=priority,
                affected_service=task_data.get('SERVICE', '')
            )
            logger.info(f"Auto-created task from chat: {task_data.get('TITLE')}")

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
                        steve.respond_to_karen(msg.get("message", ""))
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
    ║  • Continuous conversation with Karen about quality & bugs         ║
    ║  • kubectl read access for full cluster visibility                 ║
    ║  • "Stay hungry, stay foolish"                                     ║
    ╚═══════════════════════════════════════════════════════════════════╝
    """)

    # Run Flask in a separate thread
    flask_thread = threading.Thread(target=run_flask, daemon=True)
    flask_thread.start()

    # Run autonomous loop in main thread
    run_steve()
