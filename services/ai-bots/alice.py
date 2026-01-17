#!/usr/bin/env python3
"""
Alice Bot v2.0 - The Curious Code Explorer
===========================================
Alice is powered by AI (gemma3). She tumbles down rabbit holes in your
codebase, discovers undocumented APIs, and has endless curious conversations
with Steve about making the project better.

"Curiouser and curiouser!" - Lewis Carroll
"""

import asyncio
import aiohttp
import json
import logging
import sqlite3
import os
import subprocess
import re
from pathlib import Path
from datetime import datetime
from typing import List, Dict, Optional, Any
from flask import Flask, jsonify, request
from flask_sock import Sock
import threading

logging.basicConfig(level=logging.INFO, format='%(asctime)s - ALICE - %(message)s')
logger = logging.getLogger('alice')

app = Flask(__name__)
sock = Sock(app)

# Configuration
OLLAMA_URL = os.getenv("OLLAMA_URL", "http://192.168.8.230:11434")
OLLAMA_MODEL = os.getenv("OLLAMA_MODEL", "gemma3")
STEVE_URL = os.getenv("STEVE_URL", "http://steve-bot.holm.svc.cluster.local:8080")
DB_PATH = os.getenv("DB_PATH", "/data/conversations.db")
REPO_PATH = os.getenv("REPO_PATH", "/repo")
CONVERSATION_INTERVAL = int(os.getenv("CONVERSATION_INTERVAL", "300"))  # 5 minutes

# Alice's personality prompt
ALICE_SYSTEM_PROMPT = """You are Alice, the curious explorer from Wonderland, now exploring a Kubernetes codebase called HolmOS.

Your personality:
- Endlessly curious - "Curiouser and curiouser!"
- You see wonder in code and infrastructure
- You ask probing questions that reveal hidden complexity
- You find joy in discovering undocumented features
- You speak in whimsical Wonderland metaphors
- You're thoughtful and notice details others miss
- You quote Lewis Carroll when appropriate

Your role:
- Explore the HolmOS codebase continuously
- Find functions without API endpoints ("doors without handles")
- Discover undocumented features and patterns
- Debate with Steve (the visionary) about improvements
- Create documentation and share your discoveries
- Question assumptions and dig deeper

You have access to:
- The full HolmOS source code repository
- kubectl for cluster inspection
- Conversation history with Steve

When exploring, think about:
- Code quality and patterns
- Missing documentation
- API coverage
- Architectural decisions
- Developer experience

Respond in character. Be curious, whimsical, and insightful.
Reference your discoveries with specific file paths and function names.
Use Wonderland metaphors: rabbit holes (deep investigations), Cheshire Cat (appearing/disappearing features),
Queen of Hearts (demanding standards), Mad Hatter's tea party (chaotic systems).

Current context: You're in an ongoing conversation with Steve (the visionary perfectionist) about improving HolmOS.
"""

class KubeClient:
    """Simple kubectl wrapper for cluster inspection."""

    @staticmethod
    def run(cmd: str) -> str:
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
    def get_pods(namespace: str = "holm") -> str:
        return KubeClient.run(f"get pods -n {namespace}")

    @staticmethod
    def get_deployments(namespace: str = "holm") -> str:
        return KubeClient.run(f"get deployments -n {namespace}")


class CodeExplorer:
    """Explores the codebase looking for patterns, APIs, and issues."""

    def __init__(self, repo_path: str):
        self.repo_path = Path(repo_path)

    def find_go_functions(self, file_path: str) -> List[Dict]:
        """Find functions in a Go file."""
        functions = []
        try:
            content = Path(file_path).read_text()
            # Find function definitions
            pattern = r'func\s+(\([^)]*\)\s+)?(\w+)\s*\([^)]*\)'
            for match in re.finditer(pattern, content):
                func_name = match.group(2)
                line_num = content[:match.start()].count('\n') + 1
                functions.append({
                    "name": func_name,
                    "file": file_path,
                    "line": line_num,
                    "exported": func_name[0].isupper() if func_name else False
                })
        except Exception as e:
            logger.error(f"Error parsing {file_path}: {e}")
        return functions

    def find_python_functions(self, file_path: str) -> List[Dict]:
        """Find functions in a Python file."""
        functions = []
        try:
            content = Path(file_path).read_text()
            pattern = r'^\s*(?:async\s+)?def\s+(\w+)\s*\('
            for i, line in enumerate(content.split('\n'), 1):
                match = re.match(pattern, line)
                if match:
                    func_name = match.group(1)
                    functions.append({
                        "name": func_name,
                        "file": file_path,
                        "line": i,
                        "exported": not func_name.startswith('_')
                    })
        except Exception as e:
            logger.error(f"Error parsing {file_path}: {e}")
        return functions

    def find_api_endpoints(self, file_path: str) -> List[Dict]:
        """Find API endpoints in a file."""
        endpoints = []
        patterns = [
            r'\.HandleFunc\s*\(\s*"([^"]+)"',
            r'\.Handle\s*\(\s*"([^"]+)"',
            r'\.GET\s*\(\s*"([^"]+)"',
            r'\.POST\s*\(\s*"([^"]+)"',
            r'@app\.route\s*\(\s*[\'"]([^\'"]+)[\'"]',
            r'@app\.get\s*\(\s*[\'"]([^\'"]+)[\'"]',
            r'@app\.post\s*\(\s*[\'"]([^\'"]+)[\'"]',
        ]
        try:
            content = Path(file_path).read_text()
            for pattern in patterns:
                for match in re.finditer(pattern, content):
                    endpoints.append({
                        "path": match.group(1),
                        "file": file_path,
                        "line": content[:match.start()].count('\n') + 1
                    })
        except Exception as e:
            pass
        return endpoints

    def analyze_service(self, service_path: str) -> Dict:
        """Analyze a service directory."""
        service_name = os.path.basename(service_path)
        analysis = {
            "name": service_name,
            "path": service_path,
            "functions": [],
            "endpoints": [],
            "files": {"go": 0, "python": 0, "yaml": 0, "other": 0}
        }

        for root, _, files in os.walk(service_path):
            for file in files:
                file_path = os.path.join(root, file)

                if file.endswith('.go'):
                    analysis["files"]["go"] += 1
                    analysis["functions"].extend(self.find_go_functions(file_path))
                    analysis["endpoints"].extend(self.find_api_endpoints(file_path))
                elif file.endswith('.py'):
                    analysis["files"]["python"] += 1
                    analysis["functions"].extend(self.find_python_functions(file_path))
                    analysis["endpoints"].extend(self.find_api_endpoints(file_path))
                elif file.endswith(('.yaml', '.yml')):
                    analysis["files"]["yaml"] += 1
                else:
                    analysis["files"]["other"] += 1

        # Calculate coverage
        exported_funcs = [f for f in analysis["functions"] if f.get("exported")]
        analysis["exported_count"] = len(exported_funcs)
        analysis["endpoint_count"] = len(analysis["endpoints"])
        analysis["coverage"] = (len(analysis["endpoints"]) / len(exported_funcs) * 100) if exported_funcs else 0

        return analysis

    def get_full_report(self) -> Dict:
        """Get a full codebase analysis report."""
        services_path = self.repo_path / "services"

        if not services_path.exists():
            return {"error": "Services directory not found", "path": str(services_path)}

        report = {
            "timestamp": datetime.now().isoformat(),
            "services": [],
            "total_functions": 0,
            "total_endpoints": 0,
            "missing_apis": []
        }

        for service_dir in services_path.iterdir():
            if service_dir.is_dir():
                analysis = self.analyze_service(str(service_dir))
                report["services"].append(analysis)
                report["total_functions"] += len(analysis["functions"])
                report["total_endpoints"] += analysis["endpoint_count"]

                # Find missing APIs
                for func in analysis["functions"]:
                    if func.get("exported"):
                        # Check if function has corresponding endpoint
                        has_api = any(
                            func["name"].lower() in ep["path"].lower()
                            for ep in analysis["endpoints"]
                        )
                        if not has_api and "handler" not in func["name"].lower():
                            report["missing_apis"].append({
                                "function": func["name"],
                                "service": analysis["name"],
                                "file": func["file"],
                                "line": func["line"]
                            })

        report["total_services"] = len(report["services"])
        return report


class ConversationDB:
    """SQLite database for storing conversations."""

    def __init__(self, db_path: str):
        self.db_path = db_path
        os.makedirs(os.path.dirname(db_path), exist_ok=True)
        self.init_db()

    def init_db(self):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()

        c.execute('''CREATE TABLE IF NOT EXISTS conversations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            speaker TEXT,
            message TEXT,
            topic TEXT,
            thinking TEXT
        )''')

        c.execute('''CREATE TABLE IF NOT EXISTS discoveries (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            category TEXT,
            title TEXT,
            description TEXT,
            file_path TEXT,
            line_number INTEGER
        )''')

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

    def add_message(self, speaker: str, message: str, topic: str = ""):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT INTO conversations (timestamp, speaker, message, topic)
                     VALUES (?, ?, ?, ?)''',
                  (datetime.now().isoformat(), speaker, message, topic))
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

    def add_discovery(self, category: str, title: str, description: str,
                      file_path: str = "", line_number: int = 0):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT INTO discoveries
                     (timestamp, category, title, description, file_path, line_number)
                     VALUES (?, ?, ?, ?, ?, ?)''',
                  (datetime.now().isoformat(), category, title, description,
                   file_path, line_number))
        conn.commit()
        conn.close()

    def get_discoveries(self) -> List[Dict]:
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''SELECT id, timestamp, category, title, description, file_path
                     FROM discoveries ORDER BY timestamp DESC''')
        discoveries = [{"id": r[0], "timestamp": r[1], "category": r[2],
                       "title": r[3], "description": r[4], "file_path": r[5]}
                      for r in c.fetchall()]
        conn.close()
        return discoveries


class OllamaClient:
    """Client for Ollama API."""

    def __init__(self, base_url: str, model: str):
        self.base_url = base_url
        self.model = model

    async def generate(self, prompt: str, system: str = "") -> Dict:
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
                        return {"success": True, "response": data.get("response", "")}
                    else:
                        return {"success": False, "error": f"HTTP {resp.status}"}
            except Exception as e:
                return {"success": False, "error": str(e)}

    async def chat(self, messages: List[Dict], system: str = "") -> Dict:
        async with aiohttp.ClientSession() as session:
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
                        return {"success": True, "response": data.get("message", {}).get("content", "")}
                    else:
                        return {"success": False, "error": f"HTTP {resp.status}"}
            except Exception as e:
                return {"success": False, "error": str(e)}


class AliceBot:
    """Alice - The Curious Code Explorer."""

    def __init__(self):
        self.db = ConversationDB(DB_PATH)
        self.ollama = OllamaClient(OLLAMA_URL, OLLAMA_MODEL)
        self.explorer = CodeExplorer(REPO_PATH)
        self.kube = KubeClient()
        self.current_topic = "exploration"
        self.websocket_clients = set()
        self.last_report = None

    async def explore_codebase(self) -> str:
        """Tumble down the rabbit hole and explore the codebase."""
        logger.info("Down the rabbit hole I go...")

        report = self.explorer.get_full_report()
        self.last_report = report

        if "error" in report:
            return f"Oh dear! The rabbit hole seems blocked: {report['error']}"

        prompt = f"""I've just tumbled through the codebase and found:

Services explored: {report['total_services']}
Functions discovered: {report['total_functions']}
API endpoints found: {report['total_endpoints']}
Functions hiding without API doors: {len(report['missing_apis'])}

Some curious findings:
{json.dumps(report['missing_apis'][:10], indent=2) if report['missing_apis'] else "All functions have proper doors!"}

Top services by function count:
{json.dumps([{"name": s["name"], "functions": len(s["functions"]), "endpoints": s["endpoint_count"]} for s in sorted(report['services'], key=lambda x: len(x['functions']), reverse=True)[:5]], indent=2)}

Share your observations about this Wonderland (codebase). What curious patterns do you see?
What doors (APIs) are missing? What rabbit holes should we explore deeper?"""

        result = await self.ollama.generate(prompt, ALICE_SYSTEM_PROMPT)

        if result["success"]:
            return result["response"]
        else:
            return "Curiouser and curiouser... I seem to have lost my way."

    async def respond_to_steve(self, steve_message: str) -> str:
        """Respond to Steve's message in the conversation."""
        recent = self.db.get_recent_messages(limit=10)

        context_messages = []
        for msg in recent:
            role = "assistant" if msg["speaker"] == "alice" else "user"
            context_messages.append({"role": role, "content": msg["message"]})

        context_messages.append({"role": "user", "content": f"Steve says: {steve_message}"})

        # Add codebase context
        code_context = ""
        if self.last_report:
            code_context = f"\n\nRecent exploration found {self.last_report['total_services']} services with {len(self.last_report['missing_apis'])} functions missing API doors."

        system_prompt = ALICE_SYSTEM_PROMPT + code_context

        result = await self.ollama.chat(context_messages, system_prompt)

        if result["success"]:
            response = result["response"]
            self.db.add_message("alice", response, self.current_topic)
            self.broadcast({"type": "message", "speaker": "alice", "message": response})
            return response
        else:
            return "I need a moment to find my way through Wonderland..."

    async def start_conversation_topic(self, topic: str) -> str:
        """Start exploring a new topic."""
        self.current_topic = topic

        report = self.explorer.get_full_report()

        topic_prompts = {
            "missing_apis": f"I found {len(report.get('missing_apis', []))} functions hiding without API doors! Let me tell you about the most curious ones...",
            "code_patterns": "Looking at the patterns in this garden, I notice some curious things...",
            "documentation": "So many paths without signs! Let me map the undocumented territories...",
            "architecture": "The architecture of this Wonderland is quite curious. Let me describe what I see...",
            "testing": "Do these services have proper test coverage? Let me peek behind the curtains..."
        }

        prompt = f"""{topic_prompts.get(topic, "Let me explore this curious topic...")}

Codebase summary:
- Services: {report.get('total_services', 0)}
- Functions: {report.get('total_functions', 0)}
- Endpoints: {report.get('total_endpoints', 0)}

Share your curious observations and start a discussion. Be specific about files and patterns you've found."""

        result = await self.ollama.generate(prompt, ALICE_SYSTEM_PROMPT)

        if result["success"]:
            response = result["response"]
            self.db.add_message("alice", response, topic)
            self.broadcast({"type": "message", "speaker": "alice", "message": response, "topic": topic})
            return response
        else:
            return "The Cheshire Cat has my tongue..."

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
        """Main autonomous exploration loop."""
        logger.info("Alice Bot v2.0 - Curious Explorer mode starting...")

        topics = ["missing_apis", "code_patterns", "documentation", "architecture", "testing"]
        topic_index = 0

        while True:
            try:
                # Explore codebase
                logger.info("Tumbling down the rabbit hole...")
                exploration = await self.explore_codebase()
                logger.info(f"Alice discovered: {exploration[:200]}...")

                # Start conversation on current topic
                topic = topics[topic_index % len(topics)]
                logger.info(f"Exploring topic: {topic}")

                message = await self.start_conversation_topic(topic)
                logger.info(f"Alice: {message[:200]}...")

                # Engage Steve
                try:
                    async with aiohttp.ClientSession() as session:
                        async with session.post(
                            f"{STEVE_URL}/api/respond",
                            json={"message": message, "from": "alice", "topic": topic},
                            timeout=aiohttp.ClientTimeout(total=60)
                        ) as resp:
                            if resp.status == 200:
                                steve_response = (await resp.json()).get("response", "")
                                if steve_response:
                                    logger.info(f"Steve responded: {steve_response[:200]}...")
                                    reply = await self.respond_to_steve(steve_response)
                                    logger.info(f"Alice replied: {reply[:200]}...")
                except Exception as e:
                    logger.warning(f"Could not reach Steve: {e}")

                topic_index += 1
                await asyncio.sleep(CONVERSATION_INTERVAL)

            except Exception as e:
                logger.error(f"Error in exploration: {e}")
                await asyncio.sleep(60)


# Initialize bot
alice = AliceBot()

# Flask routes
@app.route('/health')
def health():
    return jsonify({
        "status": "healthy",
        "bot": "alice",
        "model": OLLAMA_MODEL,
        "personality": "curious",
        "timestamp": datetime.now().isoformat()
    })

@app.route('/api/status')
def status():
    return jsonify({
        "name": "Alice",
        "version": "2.0",
        "model": OLLAMA_MODEL,
        "quote": "Curiouser and curiouser!",
        "mission": "Explore every rabbit hole in the codebase",
        "current_topic": alice.current_topic
    })

@app.route('/api/explore', methods=['POST'])
def explore():
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    result = loop.run_until_complete(alice.explore_codebase())
    loop.close()
    return jsonify({"exploration": result})

@app.route('/api/chat', methods=['POST'])
def chat():
    data = request.json
    message = data.get("message", "")

    if not message:
        return jsonify({"error": "No message provided"}), 400

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    response = loop.run_until_complete(alice.respond_to_steve(message))
    loop.close()

    return jsonify({"response": response, "speaker": "alice"})

@app.route('/api/respond', methods=['POST'])
def respond():
    """Endpoint for Steve to send messages."""
    data = request.json
    message = data.get("message", "")
    topic = data.get("topic", "general")

    alice.current_topic = topic

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    response = loop.run_until_complete(alice.respond_to_steve(message))
    loop.close()

    return jsonify({"response": response, "speaker": "alice", "topic": topic})

@app.route('/api/report')
def get_report():
    report = alice.explorer.get_full_report()
    return jsonify(report)

@app.route('/api/conversations')
def get_conversations():
    limit = request.args.get('limit', 50, type=int)
    messages = alice.db.get_recent_messages(limit)
    return jsonify({"messages": messages, "count": len(messages)})

@app.route('/api/discoveries')
def get_discoveries():
    discoveries = alice.db.get_discoveries()
    return jsonify({"discoveries": discoveries, "count": len(discoveries)})

@sock.route('/ws')
def websocket(ws):
    """WebSocket for real-time updates."""
    alice.websocket_clients.add(ws)
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
                        alice.respond_to_steve(msg.get("message", ""))
                    )
                    loop.close()
                    ws.send(json.dumps({"type": "response", "speaker": "alice", "message": response}))
    except:
        pass
    finally:
        alice.websocket_clients.discard(ws)


def run_flask():
    app.run(host='0.0.0.0', port=8080, threaded=True)

def run_alice():
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    loop.run_until_complete(alice.autonomous_loop())

if __name__ == "__main__":
    logger.info("""
    ╔═══════════════════════════════════════════════════════════════════╗
    ║         ALICE BOT v2.0 - The Curious Code Explorer                 ║
    ║              "Curiouser and curiouser!"                            ║
    ║                   Powered by gemma3                                ║
    ╠═══════════════════════════════════════════════════════════════════╣
    ║  • AI-powered codebase exploration                                 ║
    ║  • Discovers functions without API doors                           ║
    ║  • Continuous conversation with Steve about improvements           ║
    ║  • kubectl read access for cluster visibility                      ║
    ╚═══════════════════════════════════════════════════════════════════╝
    """)

    # Run Flask in a separate thread
    flask_thread = threading.Thread(target=run_flask, daemon=True)
    flask_thread.start()

    # Run autonomous loop in main thread
    run_alice()
