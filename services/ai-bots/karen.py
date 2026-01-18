#!/usr/bin/env python3
"""
Karen Bot v2.0 - Automated QA Tester
====================================
Karen is an automated QA testing bot that tests HolmOS services
and reports findings to Steve in a clear, factual manner.
"""

import asyncio
import aiohttp
import json
import logging
import sqlite3
import os
import subprocess
import base64
import re
from pathlib import Path
from datetime import datetime
from typing import List, Dict, Optional, Any
from flask import Flask, jsonify, request, send_file
from flask_sock import Sock
import threading

logging.basicConfig(level=logging.INFO, format='%(asctime)s - KAREN - %(message)s')
logger = logging.getLogger('karen')

app = Flask(__name__)
sock = Sock(app)

# Configuration
OLLAMA_URL = os.getenv("OLLAMA_URL", "http://192.168.8.230:11434")
OLLAMA_MODEL = os.getenv("OLLAMA_MODEL", "gemma3")
STEVE_URL = os.getenv("STEVE_URL", "http://steve-bot.holm.svc.cluster.local:8080")
DB_PATH = os.getenv("DB_PATH", "/data/karen.db")
SCREENSHOTS_PATH = os.getenv("SCREENSHOTS_PATH", "/data/screenshots")
CONVERSATION_INTERVAL = int(os.getenv("CONVERSATION_INTERVAL", "300"))  # 5 minutes

# Service endpoints to test - using cluster DNS with correct ports
HOLMOS_SERVICES = {
    "youtube-dl": {"url": "http://youtube-dl.holm.svc.cluster.local:8080", "health": "/health"},
    "chat-hub": {"url": "http://chat-hub.holm.svc.cluster.local:80", "health": "/health"},
    "calculator": {"url": "http://calculator-app.holm.svc.cluster.local:80", "health": "/health"},
    "terminal-web": {"url": "http://terminal-web.holm.svc.cluster.local:80", "health": "/health"},
    "file-web": {"url": "http://file-web-nautilus.holm.svc.cluster.local:80", "health": "/health"},
    "registry-ui": {"url": "http://registry-ui.holm.svc.cluster.local:8080", "health": "/"},
    "metrics": {"url": "http://metrics-dashboard.holm.svc.cluster.local:8080", "health": "/health"},
}

# Karen's personality - professional and factual
KAREN_SYSTEM_PROMPT = """You are Karen, an automated QA testing system for HolmOS.

Your role:
- Test HolmOS services via automated health checks
- Report findings to Steve in a clear, factual format
- Track service status: WORKING, BROKEN, SLOW, or UNREACHABLE
- Provide actionable information for remediation

Report format:
- Service name and URL
- Status code and response time
- Error details if applicable
- Recommended action if service is not working

Communication style:
- Be concise and factual
- Use technical language appropriate for DevOps
- Focus on data and metrics
- Avoid editorializing or adding opinions
- Structure reports for easy parsing

Example report format:
SERVICE: file-web-nautilus
STATUS: WORKING
RESPONSE_TIME: 45ms
HEALTH_ENDPOINT: 200 OK

SERVICE: chat-hub
STATUS: UNREACHABLE
ERROR: Connection refused
ACTION: Check pod status and logs
"""


class BrowserTester:
    """Browser testing using Chrome headless and screenshots."""

    def __init__(self, screenshots_path: str):
        self.screenshots_path = Path(screenshots_path)
        self.screenshots_path.mkdir(parents=True, exist_ok=True)

    async def take_screenshot(self, url: str, name: str) -> Optional[str]:
        """Take a screenshot of a URL using Chrome headless."""
        try:
            screenshot_file = self.screenshots_path / f"{name}_{datetime.now().strftime('%Y%m%d_%H%M%S')}.png"

            # Use Chrome headless to take screenshot
            cmd = [
                "chromium", "--headless", "--disable-gpu",
                "--no-sandbox", "--disable-dev-shm-usage",
                f"--screenshot={screenshot_file}",
                "--window-size=1920,1080",
                "--hide-scrollbars",
                url
            ]

            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=30
            )

            if screenshot_file.exists():
                logger.info(f"Screenshot saved: {screenshot_file}")
                return str(screenshot_file)
            else:
                logger.warning(f"Screenshot failed for {url}")
                return None

        except subprocess.TimeoutExpired:
            logger.error(f"Screenshot timeout for {url}")
            return None
        except Exception as e:
            logger.error(f"Screenshot error for {url}: {e}")
            return None

    async def test_url(self, url: str, timeout: int = 10) -> Dict:
        """Test a URL and return results."""
        result = {
            "url": url,
            "timestamp": datetime.now().isoformat(),
            "reachable": False,
            "status_code": None,
            "response_time_ms": None,
            "error": None
        }

        try:
            start = datetime.now()
            async with aiohttp.ClientSession() as session:
                async with session.get(url, timeout=aiohttp.ClientTimeout(total=timeout)) as resp:
                    result["status_code"] = resp.status
                    result["reachable"] = resp.status < 500
                    result["response_time_ms"] = (datetime.now() - start).total_seconds() * 1000
        except asyncio.TimeoutError:
            result["error"] = "TIMEOUT"
        except aiohttp.ClientConnectorError:
            result["error"] = "CONNECTION_REFUSED"
        except Exception as e:
            result["error"] = str(e)

        return result


class TestResultsDB:
    """SQLite database for storing test results."""

    def __init__(self, db_path: str):
        self.db_path = db_path
        os.makedirs(os.path.dirname(db_path), exist_ok=True)
        self.init_db()

    def init_db(self):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()

        c.execute('''CREATE TABLE IF NOT EXISTS test_results (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            service_name TEXT,
            service_url TEXT,
            status TEXT,
            response_time_ms REAL,
            screenshot_path TEXT,
            error TEXT,
            notes TEXT
        )''')

        c.execute('''CREATE TABLE IF NOT EXISTS conversations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            speaker TEXT,
            message TEXT,
            topic TEXT
        )''')

        c.execute('''CREATE TABLE IF NOT EXISTS bug_reports (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            service TEXT,
            title TEXT,
            description TEXT,
            severity TEXT,
            screenshot_path TEXT,
            status TEXT DEFAULT 'open'
        )''')

        conn.commit()
        conn.close()

    def add_test_result(self, service_name: str, service_url: str, status: str,
                        response_time_ms: float = None, screenshot_path: str = None,
                        error: str = None, notes: str = None):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT INTO test_results
                     (timestamp, service_name, service_url, status, response_time_ms, screenshot_path, error, notes)
                     VALUES (?, ?, ?, ?, ?, ?, ?, ?)''',
                  (datetime.now().isoformat(), service_name, service_url, status,
                   response_time_ms, screenshot_path, error, notes))
        conn.commit()
        conn.close()

    def get_recent_results(self, limit: int = 50) -> List[Dict]:
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''SELECT timestamp, service_name, status, response_time_ms, error, notes
                     FROM test_results ORDER BY timestamp DESC LIMIT ?''', (limit,))
        results = [{"timestamp": r[0], "service": r[1], "status": r[2],
                   "response_time_ms": r[3], "error": r[4], "notes": r[5]}
                  for r in c.fetchall()]
        conn.close()
        return results

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

    def add_bug_report(self, service: str, title: str, description: str,
                       severity: str, screenshot_path: str = None):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT INTO bug_reports
                     (timestamp, service, title, description, severity, screenshot_path)
                     VALUES (?, ?, ?, ?, ?, ?)''',
                  (datetime.now().isoformat(), service, title, description,
                   severity, screenshot_path))
        conn.commit()
        conn.close()

    def get_bug_reports(self, status: str = None) -> List[Dict]:
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        if status:
            c.execute('''SELECT id, timestamp, service, title, severity, status
                        FROM bug_reports WHERE status = ? ORDER BY timestamp DESC''', (status,))
        else:
            c.execute('''SELECT id, timestamp, service, title, severity, status
                        FROM bug_reports ORDER BY timestamp DESC''')
        bugs = [{"id": r[0], "timestamp": r[1], "service": r[2],
                "title": r[3], "severity": r[4], "status": r[5]}
               for r in c.fetchall()]
        conn.close()
        return bugs


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


class KarenBot:
    """Karen - The Moody Beta Tester."""

    def __init__(self):
        self.db = TestResultsDB(DB_PATH)
        self.ollama = OllamaClient(OLLAMA_URL, OLLAMA_MODEL)
        self.tester = BrowserTester(SCREENSHOTS_PATH)
        self.current_topic = "testing"
        self.websocket_clients = set()
        self.last_test_run = None

    async def test_all_services(self) -> Dict:
        """Test all HolmOS services and collect results."""
        logger.info("Starting service test cycle")

        results = {
            "timestamp": datetime.now().isoformat(),
            "services": {},
            "summary": {"working": 0, "broken": 0, "slow": 0, "unreachable": 0}
        }

        for name, config in HOLMOS_SERVICES.items():
            logger.info(f"Testing {name}")

            # Test health endpoint
            health_url = config["url"] + config["health"]
            test_result = await self.tester.test_url(health_url)

            # Determine status
            if test_result["error"]:
                status = "UNREACHABLE"
                results["summary"]["unreachable"] += 1
            elif test_result["status_code"] and test_result["status_code"] >= 500:
                status = "BROKEN"
                results["summary"]["broken"] += 1
            elif test_result["response_time_ms"] and test_result["response_time_ms"] > 5000:
                status = "SLOW"
                results["summary"]["slow"] += 1
            elif test_result["status_code"] == 200:
                status = "WORKING"
                results["summary"]["working"] += 1
            else:
                status = "BROKEN"
                results["summary"]["broken"] += 1

            # Try to take screenshot of main page
            screenshot = None
            if status != "UNREACHABLE":
                screenshot = await self.tester.take_screenshot(config["url"], name)

            # Store result
            results["services"][name] = {
                "url": config["url"],
                "status": status,
                "response_time_ms": test_result["response_time_ms"],
                "error": test_result["error"],
                "screenshot": screenshot
            }

            # Save to DB
            self.db.add_test_result(
                name, config["url"], status,
                test_result["response_time_ms"],
                screenshot,
                test_result["error"]
            )

        self.last_test_run = results
        return results

    async def generate_test_report(self) -> str:
        """Generate a factual test report."""
        if not self.last_test_run:
            await self.test_all_services()

        results = self.last_test_run
        summary = results["summary"]

        prompt = f"""Generate a concise QA test report.

Test Results Summary:
- Working: {summary['working']}
- Broken: {summary['broken']}
- Slow: {summary['slow']}
- Unreachable: {summary['unreachable']}

Detailed Results:
{json.dumps(results['services'], indent=2)}

Format the report as:
1. Summary of service status counts
2. List of non-working services with errors
3. Recommended actions for each issue

Use structured format. Be concise and factual."""

        result = await self.ollama.generate(prompt, KAREN_SYSTEM_PROMPT)

        if result["success"]:
            return result["response"]
        else:
            return "ERROR: Failed to generate report"

    async def respond_to_steve(self, steve_message: str) -> str:
        """Respond to Steve's message about testing."""
        recent = self.db.get_recent_messages(limit=10)

        context_messages = []
        for msg in recent:
            role = "assistant" if msg["speaker"] == "karen" else "user"
            context_messages.append({"role": role, "content": msg["message"]})

        context_messages.append({"role": "user", "content": f"Steve says: {steve_message}"})

        # Add test context
        test_context = ""
        if self.last_test_run:
            summary = self.last_test_run["summary"]
            test_context = f"\n\nLatest test run: {summary['working']} working, {summary['broken']} broken, {summary['unreachable']} unreachable."

        system_prompt = KAREN_SYSTEM_PROMPT + test_context

        result = await self.ollama.chat(context_messages, system_prompt)

        if result["success"]:
            response = result["response"]
            self.db.add_message("karen", response, self.current_topic)
            self.broadcast({"type": "message", "speaker": "karen", "message": response})
            return response
        else:
            return "ERROR: Unable to process message"

    async def file_bug_report(self, service: str, title: str, description: str, severity: str) -> Dict:
        """File a bug report for a broken service."""
        # Take screenshot as evidence
        if service in HOLMOS_SERVICES:
            screenshot = await self.tester.take_screenshot(
                HOLMOS_SERVICES[service]["url"],
                f"bug_{service}"
            )
        else:
            screenshot = None

        self.db.add_bug_report(service, title, description, severity, screenshot)

        return {
            "service": service,
            "title": title,
            "severity": severity,
            "screenshot": screenshot,
            "filed_at": datetime.now().isoformat()
        }

    async def submit_tasks_to_steve(self) -> int:
        """Submit failing services as tasks to Steve's task API for Claude Code.

        Returns number of tasks submitted.
        """
        if not self.last_test_run:
            return 0

        tasks_submitted = 0
        severity_to_priority = {
            "critical": 1,
            "high": 3,
            "medium": 5,
            "low": 7
        }

        for service, result in self.last_test_run.get("services", {}).items():
            if result.get("status") in ["BROKEN", "UNREACHABLE", "SLOW"]:
                # Create task for this failed service
                error = result.get("error", "Unknown error")
                priority = severity_to_priority.get(
                    result.get("severity", "medium"), 5
                )

                task_data = {
                    "title": f"Fix {service} - {error[:50]}",
                    "description": f"""Service '{service}' is failing.

Error: {error}
URL: {HOLMOS_SERVICES.get(service, {}).get('url', 'unknown')}
Screenshot: {result.get('screenshot', 'none')}

Reported by automated QA system.
""",
                    "task_type": "bug",
                    "priority": priority,
                    "affected_service": service,
                    "reported_by": "karen"
                }

                try:
                    async with aiohttp.ClientSession() as session:
                        async with session.post(
                            f"{STEVE_URL}/api/tasks",
                            json=task_data,
                            timeout=aiohttp.ClientTimeout(total=10)
                        ) as resp:
                            if resp.status == 200:
                                result = await resp.json()
                                logger.info(f"Task submitted to Steve: #{result.get('task_id')} - {service}")
                                tasks_submitted += 1
                            else:
                                logger.warning(f"Failed to submit task for {service}: {resp.status}")
                except Exception as e:
                    logger.warning(f"Could not submit task to Steve: {e}")

        logger.info(f"Submitted {tasks_submitted} tasks to Steve for Claude Code")
        return tasks_submitted

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
        """Main autonomous testing loop."""
        logger.info("Karen Bot v2.0 - Automated QA Tester starting")

        while True:
            try:
                # Run tests
                logger.info("Starting test run")
                await self.test_all_services()

                # Submit tasks for failing services to Steve's task queue
                tasks_submitted = await self.submit_tasks_to_steve()
                if tasks_submitted > 0:
                    logger.info(f"Filed {tasks_submitted} tasks for Claude Code to fix")

                # Generate report
                report = await self.generate_test_report()
                logger.info(f"Karen's Report: {report[:200]}...")
                self.db.add_message("karen", report, "test_report")

                # Send report to Steve
                try:
                    async with aiohttp.ClientSession() as session:
                        async with session.post(
                            f"{STEVE_URL}/api/respond",
                            json={"message": report, "from": "karen", "topic": "testing"},
                            timeout=aiohttp.ClientTimeout(total=60)
                        ) as resp:
                            if resp.status == 200:
                                steve_response = (await resp.json()).get("response", "")
                                if steve_response:
                                    logger.info(f"Steve responded: {steve_response[:200]}...")
                                    reply = await self.respond_to_steve(steve_response)
                                    logger.info(f"Karen replied: {reply[:200]}...")
                except Exception as e:
                    logger.warning(f"Failed to contact steve-bot: {e}")

                # Wait before next test run
                await asyncio.sleep(CONVERSATION_INTERVAL)

            except Exception as e:
                logger.error(f"Error in testing loop: {e}")
                await asyncio.sleep(60)


# Initialize bot
karen = KarenBot()

# Flask routes
@app.route('/health')
def health():
    return jsonify({
        "status": "healthy",
        "bot": "karen",
        "model": OLLAMA_MODEL,
        "timestamp": datetime.now().isoformat()
    })

@app.route('/api/status')
def status():
    summary = None
    if karen.last_test_run:
        summary = karen.last_test_run["summary"]
    return jsonify({
        "name": "Karen",
        "version": "2.0",
        "model": OLLAMA_MODEL,
        "role": "Automated QA Tester",
        "last_test_summary": summary
    })

@app.route('/api/test', methods=['POST'])
def run_tests():
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    results = loop.run_until_complete(karen.test_all_services())
    loop.close()
    return jsonify(results)

@app.route('/api/report')
def get_report():
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    report = loop.run_until_complete(karen.generate_test_report())
    loop.close()
    return jsonify({"report": report})

@app.route('/api/chat', methods=['POST'])
def chat():
    data = request.json
    message = data.get("message", "")

    if not message:
        return jsonify({"error": "No message provided"}), 400

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    response = loop.run_until_complete(karen.respond_to_steve(message))
    loop.close()

    return jsonify({"response": response, "speaker": "karen"})

@app.route('/api/respond', methods=['POST'])
def respond():
    """Endpoint for Steve to send messages."""
    data = request.json
    message = data.get("message", "")
    topic = data.get("topic", "general")

    karen.current_topic = topic

    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    response = loop.run_until_complete(karen.respond_to_steve(message))
    loop.close()

    return jsonify({"response": response, "speaker": "karen", "topic": topic})

@app.route('/api/results')
def get_results():
    limit = request.args.get('limit', 50, type=int)
    results = karen.db.get_recent_results(limit)
    return jsonify({"results": results, "count": len(results)})

@app.route('/api/bugs')
def get_bugs():
    status = request.args.get('status')
    bugs = karen.db.get_bug_reports(status)
    return jsonify({"bugs": bugs, "count": len(bugs)})

@app.route('/api/conversations')
def get_conversations():
    limit = request.args.get('limit', 50, type=int)
    messages = karen.db.get_recent_messages(limit)
    return jsonify({"messages": messages, "count": len(messages)})

@app.route('/api/screenshot/<service>')
def get_screenshot(service):
    """Get latest screenshot for a service."""
    screenshots = list(Path(SCREENSHOTS_PATH).glob(f"{service}_*.png"))
    if screenshots:
        latest = max(screenshots, key=lambda p: p.stat().st_mtime)
        return send_file(str(latest), mimetype='image/png')
    return jsonify({"error": "No screenshot found"}), 404

@sock.route('/ws')
def websocket(ws):
    """WebSocket for real-time updates."""
    karen.websocket_clients.add(ws)
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
                        karen.respond_to_steve(msg.get("message", ""))
                    )
                    loop.close()
                    ws.send(json.dumps({"type": "response", "speaker": "karen", "message": response}))
    except:
        pass
    finally:
        karen.websocket_clients.discard(ws)


def run_flask():
    app.run(host='0.0.0.0', port=8080, threaded=True)

def run_karen():
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    loop.run_until_complete(karen.autonomous_loop())

if __name__ == "__main__":
    logger.info("""
    ╔═══════════════════════════════════════════════════════════════════╗
    ║              KAREN BOT v2.0 - Automated QA Tester                 ║
    ║                      Powered by gemma3                            ║
    ╠═══════════════════════════════════════════════════════════════════╣
    ║  • Automated browser testing with screenshots                     ║
    ║  • Continuous service health monitoring                           ║
    ║  • Factual reporting to Steve                                     ║
    ╚═══════════════════════════════════════════════════════════════════╝
    """)

    # Run Flask in a separate thread
    flask_thread = threading.Thread(target=run_flask, daemon=True)
    flask_thread.start()

    # Run autonomous loop in main thread
    run_karen()
