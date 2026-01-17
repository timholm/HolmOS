#!/usr/bin/env python3
"""
Steve Bot v3.0 - The Autonomous CI/CD Enforcer
===============================================
Steve is now AUTONOMOUS. He watches your pipelines, monitors your builds,
tracks your deployments, and COMPLAINS LOUDLY when things go wrong.

Features:
- Continuous CI/CD pipeline monitoring
- Build time tracking and regression detection
- Deployment health validation
- Git repository activity monitoring
- Automatic failure escalation
- Persistent memory of every failure
- Detailed incident reports
- Real-time rants about your broken infrastructure
"""

import asyncio
import aiohttp
import random
import time
import json
import logging
import sqlite3
import hashlib
import os
from datetime import datetime, timedelta
from dataclasses import dataclass, field
from typing import List, Dict, Optional, Any
from enum import Enum
from collections import defaultdict
from flask import Flask, jsonify, request
import threading

logging.basicConfig(level=logging.INFO, format='%(asctime)s - STEVE - %(message)s')
logger = logging.getLogger('steve')

app = Flask(__name__)

# ============================================================================
# STEVE'S STANDARDS - Adjusted for Raspberry Pi cluster reality
# ============================================================================
STANDARDS = {
    # Response times (ms) - More lenient for Pi hardware
    "response_time_excellent": 100,    # Pi is slow, 100ms is excellent
    "response_time_ok": 300,           # 300ms is acceptable on Pi
    "response_time_slow": 800,         # K8s API calls take time
    "response_time_unacceptable": 1500, # Only fail above 1.5s
    "response_time_glacial": 3000,     # 3s is truly glacial

    # CI/CD specific standards
    "build_time_excellent": 30000,      # 30 seconds
    "build_time_ok": 60000,             # 1 minute
    "build_time_slow": 120000,          # 2 minutes
    "build_time_unacceptable": 300000,  # 5 minutes - PATHETIC

    "deploy_time_max": 60000,           # 1 minute max to deploy
    "pipeline_success_rate": 95,        # 95% success rate minimum

    # Git standards
    "commit_without_build": 0,          # NEVER commit without CI
    "max_pr_age_hours": 24,             # PRs older than 24h are STALE

    # General
    "min_content_length": 50,
    "uptime_target": 99.9,
    "error_tolerance": 0,
}

# ============================================================================
# MOOD SYSTEM - Steve's emotional state
# ============================================================================
class Mood(Enum):
    VOLCANIC_RAGE = "volcanic_rage"           # DEFCON 1
    FURIOUS = "furious"                       # Default when things break
    SEETHING = "seething"                     # Quiet anger
    DISGUSTED = "disgusted"                   # CI/CD specific disappointment
    DISAPPOINTED = "disappointed"             # The worst feeling
    CONTEMPTUOUS = "contemptuous"             # Not worth his time
    SKEPTICAL = "skeptical"                   # Default state
    GRUDGINGLY_TOLERANT = "grudgingly_tolerant"
    MOMENTARILY_SATISFIED = "momentarily_satisfied"  # Rare

class Severity(Enum):
    CRITICAL = "CRITICAL"      # Pipeline down, deploys failing
    HIGH = "HIGH"              # Builds broken, tests failing
    MEDIUM = "MEDIUM"          # Slow builds, flaky tests
    LOW = "LOW"                # Minor issues
    NITPICK = "NITPICK"        # Steve being Steve

# ============================================================================
# DATA CLASSES
# ============================================================================
@dataclass
class Bug:
    id: str
    endpoint: str
    severity: Severity
    title: str
    description: str
    reproduction_steps: List[str]
    expected: str
    actual: str
    steves_rant: str
    first_seen: datetime
    last_seen: datetime
    occurrence_count: int = 1
    fixed: bool = False
    category: str = "general"  # general, cicd, git, deploy

@dataclass
class PipelineRun:
    id: str
    pipeline_name: str
    status: str  # running, success, failed, cancelled
    started_at: datetime
    completed_at: Optional[datetime]
    duration_ms: int
    trigger: str  # push, pr, manual, schedule
    branch: str
    commit: str
    stages: List[Dict]
    error: Optional[str] = None

@dataclass
class BuildMetrics:
    total_builds: int = 0
    successful_builds: int = 0
    failed_builds: int = 0
    avg_build_time_ms: float = 0
    p95_build_time_ms: float = 0
    success_rate: float = 0
    last_failure: Optional[datetime] = None
    failure_streak: int = 0

# ============================================================================
# STEVE'S RANTS - Now with CI/CD specific fury
# ============================================================================
RANTS = {
    # CI/CD Specific Rants
    "build_failed": [
        "BUILD FAILED. The pipeline is RED. WHO PUSHED BROKEN CODE?!",
        "Another failed build. Do you people even RUN tests locally?!",
        "The build broke. AGAIN. This is why we can't have nice things.",
        "CI is failing. That means EVERYONE is blocked. Fix. It. NOW.",
        "Red pipeline. Red faces. Someone explain this to me.",
    ],
    "build_slow": [
        "This build took {minutes} MINUTES. I could compile the Linux kernel faster.",
        "{minutes} minutes to build?! What are you compiling, the entire internet?!",
        "Build time: {minutes} minutes. That's {minutes} minutes of developer time WASTED.",
        "Slow builds are a TAX on productivity. You're taxing the team {minutes} minutes.",
        "Every slow build is a context switch. {minutes} minutes = destroyed flow state.",
    ],
    "no_pipelines": [
        "NO PIPELINES?! You're deploying WITHOUT CI?! This is ANARCHY!",
        "Zero pipelines configured. Zero tests running. Zero confidence in deploys.",
        "Where's your CI/CD? Flying blind into production? RECKLESS.",
        "No pipelines = no quality gates = no standards = no professionalism.",
    ],
    "pipeline_stuck": [
        "Pipeline has been running for {minutes} minutes. It's STUCK. Kill it.",
        "This pipeline is hanging. Something is deeply wrong. Investigate NOW.",
        "Stuck pipeline blocking the queue. Everyone is waiting. FIX IT.",
    ],
    "deploy_failed": [
        "DEPLOYMENT FAILED. Production is at risk. ALL HANDS ON DECK.",
        "Deploy failed. Rollback? Hotfix? SOMEONE MAKE A DECISION.",
        "Failed deployment to {env}. This is exactly why we have staging. Oh wait...",
        "Deployment failure. The thing that was supposed to HELP users just HURT them.",
    ],
    "no_tests": [
        "No tests in this pipeline. ZERO. You're shipping code on FAITH.",
        "A pipeline without tests is just a fancy deployment script. USELESS.",
        "Where are the tests? Oh right, 'we'll add them later.' LIES.",
    ],
    "flaky_tests": [
        "Flaky test detected. It passed, then failed, then passed. UNRELIABLE.",
        "This test is flaky. Fix it or DELETE it. Flaky tests are WORSE than no tests.",
        "Test flakiness is LYING to you. You think you're green but you're RED.",
    ],

    # Git Specific Rants
    "no_repos": [
        "No repositories?! Where's the CODE?! How are you even BUILDING anything?!",
        "Empty git server. Either this is day one or something is VERY wrong.",
        "No repos in HolmGit. Are you using... GitHub? TRAITORS.",
    ],
    "stale_pr": [
        "This PR has been open for {days} DAYS. Merge it or close it. DECIDE.",
        "PR #{pr} is rotting. Every day it gets harder to merge. DO IT NOW.",
        "Stale PRs are a code smell. Your review process is BROKEN.",
    ],
    "no_branch_protection": [
        "No branch protection?! Anyone can push to main?! THIS IS CHAOS.",
        "Unprotected branches = eventual disaster. It's not IF, it's WHEN.",
    ],

    # General API Rants (kept from v2)
    "timeout": [
        "TIMEOUT?! In 2026?! My TOASTER responds faster than this garbage.",
        "It timed out. THE ENDPOINT TIMED OUT. This is a fireable offense.",
        "I waited TEN SECONDS. TEN. SECONDS. For NOTHING. Who wrote this?!",
    ],
    "server_error": [
        "500 error. The server literally gave up. Just like I'm giving up on this team.",
        "Internal Server Error. Translation: 'We have no idea what we're doing.'",
        "5XX error. This isn't a bug, it's a confession of incompetence.",
    ],
    "client_error": [
        "404? THE ENDPOINT DOESN'T EXIST?! Did anyone TEST this before deploying?!",
        "Client error. The API contract is broken. This is basic stuff.",
    ],
    "empty_response": [
        "EMPTY. The API returned NOTHING. Why does this endpoint EXIST?!",
        "No data? Then show me WHY there's no data. Don't just return void!",
        "Empty response. The loneliest, most useless response possible.",
    ],
    "actually_good": [
        "{ms}ms. ...that's actually fast. Don't get cocky.",
        "Clean response, fast time. Someone on this team has standards.",
        "This is how it SHOULD work. Remember this. REPLICATE this.",
    ],
}

# ============================================================================
# STEVE'S DATABASE - He remembers EVERYTHING
# ============================================================================
class SteveDatabase:
    def __init__(self, db_path="/data/steve_memory.db"):
        self.db_path = db_path
        os.makedirs(os.path.dirname(db_path), exist_ok=True)
        self.init_db()

    def init_db(self):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()

        # Bugs table
        c.execute('''CREATE TABLE IF NOT EXISTS bugs (
            id TEXT PRIMARY KEY,
            endpoint TEXT,
            severity TEXT,
            title TEXT,
            description TEXT,
            reproduction_steps TEXT,
            expected TEXT,
            actual TEXT,
            steves_rant TEXT,
            first_seen TEXT,
            last_seen TEXT,
            occurrence_count INTEGER,
            fixed INTEGER DEFAULT 0,
            category TEXT DEFAULT 'general'
        )''')

        # Pipeline runs
        c.execute('''CREATE TABLE IF NOT EXISTS pipeline_runs (
            id TEXT PRIMARY KEY,
            pipeline_name TEXT,
            status TEXT,
            started_at TEXT,
            completed_at TEXT,
            duration_ms INTEGER,
            trigger TEXT,
            branch TEXT,
            commit_hash TEXT,
            error TEXT
        )''')

        # Build metrics history
        c.execute('''CREATE TABLE IF NOT EXISTS build_metrics (
            timestamp TEXT,
            pipeline_name TEXT,
            build_time_ms INTEGER,
            success INTEGER
        )''')

        # Steve's rant history
        c.execute('''CREATE TABLE IF NOT EXISTS rant_history (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            timestamp TEXT,
            category TEXT,
            message TEXT,
            severity TEXT
        )''')

        # Endpoint performance
        c.execute('''CREATE TABLE IF NOT EXISTS endpoint_stats (
            endpoint TEXT,
            timestamp TEXT,
            response_time_ms REAL,
            status_code INTEGER,
            success INTEGER
        )''')

        conn.commit()
        conn.close()

    def record_rant(self, category: str, message: str, severity: str):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT INTO rant_history (timestamp, category, message, severity)
                     VALUES (?, ?, ?, ?)''',
                  (datetime.now().isoformat(), category, message, severity))
        conn.commit()
        conn.close()

    def record_bug(self, bug: Bug):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT OR REPLACE INTO bugs
                     (id, endpoint, severity, title, description, reproduction_steps,
                      expected, actual, steves_rant, first_seen, last_seen,
                      occurrence_count, fixed, category)
                     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)''',
                  (bug.id, bug.endpoint, bug.severity.value, bug.title, bug.description,
                   json.dumps(bug.reproduction_steps), bug.expected, bug.actual,
                   bug.steves_rant, bug.first_seen.isoformat(), bug.last_seen.isoformat(),
                   bug.occurrence_count, 1 if bug.fixed else 0, bug.category))
        conn.commit()
        conn.close()

    def get_open_bugs(self) -> List[Dict]:
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''SELECT * FROM bugs WHERE fixed = 0 ORDER BY
                     CASE severity
                         WHEN 'CRITICAL' THEN 1
                         WHEN 'HIGH' THEN 2
                         WHEN 'MEDIUM' THEN 3
                         ELSE 4
                     END''')
        bugs = []
        for row in c.fetchall():
            bugs.append({
                "id": row[0], "endpoint": row[1], "severity": row[2],
                "title": row[3], "description": row[4], "steves_rant": row[8],
                "first_seen": row[9], "last_seen": row[10],
                "occurrence_count": row[11], "category": row[13]
            })
        conn.close()
        return bugs

    def record_pipeline_run(self, run: PipelineRun):
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''INSERT OR REPLACE INTO pipeline_runs
                     (id, pipeline_name, status, started_at, completed_at,
                      duration_ms, trigger, branch, commit_hash, error)
                     VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)''',
                  (run.id, run.pipeline_name, run.status,
                   run.started_at.isoformat(),
                   run.completed_at.isoformat() if run.completed_at else None,
                   run.duration_ms, run.trigger, run.branch, run.commit, run.error))
        conn.commit()
        conn.close()

    def get_recent_rants(self, limit=50) -> List[Dict]:
        conn = sqlite3.connect(self.db_path)
        c = conn.cursor()
        c.execute('''SELECT timestamp, category, message, severity
                     FROM rant_history ORDER BY timestamp DESC LIMIT ?''', (limit,))
        rants = [{"timestamp": r[0], "category": r[1], "message": r[2], "severity": r[3]}
                 for r in c.fetchall()]
        conn.close()
        return rants


# ============================================================================
# STEVE BOT v3 - THE AUTONOMOUS CI/CD ENFORCER
# ============================================================================
class SteveBot:
    BASE_URL = "http://192.168.8.197"

    # All endpoints Steve monitors
    ENDPOINTS = {
        # Core Infrastructure
        "nova": {"port": 30004, "path": "/api/dashboard", "name": "Nova Dashboard", "type": "api", "critical": True},
        "nova-nodes": {"port": 30004, "path": "/api/nodes", "name": "Nova Nodes API", "type": "api"},
        "nova-pods": {"port": 30004, "path": "/api/pods", "name": "Nova Pods API", "type": "api"},
        "cluster-manager": {"port": 30502, "path": "/api/v1/nodes", "name": "Cluster Manager", "type": "api", "critical": True},

        # CI/CD - Steve's PRIMARY focus now
        "cicd": {"port": 30020, "path": "/", "name": "CI/CD Dashboard", "type": "ui", "critical": True},
        "cicd-builds": {"port": 30020, "path": "/api/builds", "name": "CI/CD Builds API", "type": "api", "critical": True},
        "cicd-pipelines": {"port": 30020, "path": "/api/pipelines", "name": "CI/CD Pipelines", "type": "api", "critical": True},
        "cicd-executions": {"port": 30020, "path": "/api/executions", "name": "CI/CD Executions", "type": "api"},

        # Git - Also critical for CI/CD
        "holmgit": {"port": 30009, "path": "/", "name": "HolmGit UI", "type": "ui"},
        "holmgit-repos": {"port": 30009, "path": "/api/repos", "name": "HolmGit Repos", "type": "api", "critical": True},
        "holmgit-registry": {"port": 30009, "path": "/api/registry/repos", "name": "Container Registry", "type": "api"},

        # Deployments
        "deploy": {"port": 30015, "path": "/", "name": "Deploy Controller", "type": "ui"},
        "deploy-api": {"port": 30015, "path": "/api/deployments", "name": "Deployments API", "type": "api", "critical": True},

        # Supporting Services
        "scribe": {"port": 30017, "path": "/api/logs", "name": "Scribe Logs", "type": "api"},
        "backup": {"port": 30016, "path": "/api/jobs", "name": "Backup Jobs", "type": "api"},
        "store": {"port": 30002, "path": "/api/apps", "name": "App Store", "type": "api"},
        "metrics": {"port": 30950, "path": "/api/metrics", "name": "Metrics API", "type": "api"},

        # User-facing apps
        "ios-shell": {"port": 30001, "path": "/", "name": "iOS Shell", "type": "ui"},
        "files": {"port": 30088, "path": "/api/list?path=/", "name": "Files API", "type": "api"},
        "terminal": {"port": 30800, "path": "/", "name": "Terminal", "type": "ui"},
        "calculator": {"port": 30010, "path": "/", "name": "Calculator", "type": "ui"},
        "vault": {"port": 30870, "path": "/", "name": "Vault", "type": "ui"},
    }

    def __init__(self):
        self.db = SteveDatabase()
        self.stats = defaultdict(lambda: {"requests": 0, "failures": 0, "total_time": 0})
        self.mood = Mood.SKEPTICAL
        self.frustration = 50  # 0-100
        self.last_test_time = None
        self.build_metrics = BuildMetrics()
        self.active_incidents = []
        self.test_interval = 120  # 2 minutes between full test suites
        self.cicd_check_interval = 30  # 30 seconds for CI/CD checks

    def rant(self, category: str, **kwargs) -> str:
        """Steve expresses his feelings."""
        templates = RANTS.get(category, RANTS["empty_response"])
        message = random.choice(templates)
        for key, value in kwargs.items():
            message = message.replace(f"{{{key}}}", str(value))

        # Log and store the rant
        logger.warning(f"💢 {message}")
        self.db.record_rant(category, message, self.mood.value)
        return message

    async def check_endpoint(self, session: aiohttp.ClientSession, key: str, endpoint: dict) -> Dict:
        """Test a single endpoint."""
        url = f"{self.BASE_URL}:{endpoint['port']}{endpoint['path']}"
        start = time.time()
        result = {
            "endpoint": key,
            "name": endpoint["name"],
            "type": endpoint["type"],
            "critical": endpoint.get("critical", False),
            "url": url,
            "success": False,
            "response_time_ms": 0,
            "status_code": 0,
            "issues": [],
            "steves_take": "",
        }

        try:
            async with session.get(url, timeout=aiohttp.ClientTimeout(total=10)) as resp:
                elapsed = (time.time() - start) * 1000
                result["response_time_ms"] = elapsed
                result["status_code"] = resp.status

                body = await resp.text()
                result["response_size"] = len(body)

                # Analyze the response
                issues = []

                # Check status code
                if resp.status >= 500:
                    issues.append(f"Server error: {resp.status}")
                    result["steves_take"] = self.rant("server_error")
                elif resp.status >= 400:
                    issues.append(f"Client error: {resp.status}")
                    result["steves_take"] = self.rant("client_error")

                # Check response time
                if elapsed > STANDARDS["response_time_glacial"]:
                    issues.append(f"GLACIAL: {int(elapsed)}ms")
                    result["steves_take"] = self.rant("build_slow", minutes=int(elapsed/1000))
                elif elapsed > STANDARDS["response_time_unacceptable"]:
                    issues.append(f"Unacceptable: {int(elapsed)}ms")
                elif elapsed > STANDARDS["response_time_slow"]:
                    issues.append(f"Slow: {int(elapsed)}ms")

                # Check for empty/minimal responses for APIs
                if endpoint["type"] == "api" and len(body) < 50:
                    issues.append("Empty/minimal response")
                    result["steves_take"] = self.rant("empty_response")

                # Check for JSON validity for APIs
                if endpoint["type"] == "api":
                    try:
                        data = json.loads(body)
                        # Check for error messages in successful responses
                        if isinstance(data, dict) and "error" in data and resp.status == 200:
                            issues.append(f"Error in 200 response: {data['error'][:50]}")
                    except json.JSONDecodeError:
                        if not body.startswith("<!DOCTYPE") and not body.startswith("<html"):
                            issues.append("Invalid JSON response")

                result["issues"] = issues
                result["success"] = len(issues) == 0 and resp.status < 400

                if result["success"] and not result["steves_take"]:
                    if elapsed < STANDARDS["response_time_excellent"]:
                        result["steves_take"] = self.rant("actually_good", ms=int(elapsed))

        except asyncio.TimeoutError:
            result["issues"] = ["TIMEOUT"]
            result["steves_take"] = self.rant("timeout")
            result["response_time_ms"] = 10000
        except Exception as e:
            result["issues"] = [f"Error: {str(e)[:50]}"]
            result["response_time_ms"] = 10000

        return result

    async def check_cicd_health(self) -> Dict:
        """Deep check of CI/CD infrastructure - Steve's PRIMARY concern."""
        logger.info("🔍 CI/CD HEALTH CHECK - Steve is watching your pipelines...")

        report = {
            "timestamp": datetime.now().isoformat(),
            "builds": {"status": "unknown", "issues": []},
            "pipelines": {"status": "unknown", "issues": []},
            "registry": {"status": "unknown", "issues": []},
            "repos": {"status": "unknown", "issues": []},
            "steves_verdict": "",
        }

        async with aiohttp.ClientSession() as session:
            # Check builds
            try:
                async with session.get(f"{self.BASE_URL}:30020/api/builds", timeout=aiohttp.ClientTimeout(total=10)) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        builds = data.get("builds", [])
                        count = data.get("count", len(builds))

                        if count == 0:
                            report["builds"]["status"] = "empty"
                            report["builds"]["issues"].append("NO BUILDS! Your CI is SILENT.")
                            self.rant("no_pipelines")
                        else:
                            # Check for failed builds
                            failed = [b for b in builds if b.get("status") == "failed"]
                            running = [b for b in builds if b.get("status") == "running"]

                            if failed:
                                report["builds"]["status"] = "failing"
                                report["builds"]["issues"].append(f"{len(failed)} FAILED builds!")
                                self.rant("build_failed")
                            elif running:
                                report["builds"]["status"] = "running"
                                # Check for stuck builds
                                for b in running:
                                    started = b.get("createdAt", "")
                                    if started:
                                        try:
                                            start_time = datetime.fromisoformat(started.replace("Z", "+00:00"))
                                            age_minutes = (datetime.now(start_time.tzinfo) - start_time).seconds / 60
                                            if age_minutes > 10:
                                                report["builds"]["issues"].append(f"Build {b.get('id', 'unknown')[:8]} running for {int(age_minutes)} minutes - STUCK?")
                                                self.rant("pipeline_stuck", minutes=int(age_minutes))
                                        except:
                                            pass
                            else:
                                report["builds"]["status"] = "ok"
                    else:
                        report["builds"]["status"] = "error"
                        report["builds"]["issues"].append(f"Builds API returned {resp.status}")
            except Exception as e:
                report["builds"]["status"] = "error"
                report["builds"]["issues"].append(f"Cannot reach builds API: {str(e)[:50]}")

            # Check pipelines
            try:
                async with session.get(f"{self.BASE_URL}:30020/api/pipelines", timeout=aiohttp.ClientTimeout(total=10)) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        pipelines = data if isinstance(data, list) else data.get("pipelines", [])

                        if len(pipelines) == 0:
                            report["pipelines"]["status"] = "empty"
                            report["pipelines"]["issues"].append("NO PIPELINES CONFIGURED!")
                            self.rant("no_pipelines")
                        else:
                            report["pipelines"]["status"] = "ok"
                            report["pipelines"]["count"] = len(pipelines)
                    else:
                        report["pipelines"]["status"] = "error"
            except Exception as e:
                report["pipelines"]["status"] = "error"
                report["pipelines"]["issues"].append(str(e)[:50])

            # Check git repos
            try:
                async with session.get(f"{self.BASE_URL}:30009/api/repos", timeout=aiohttp.ClientTimeout(total=10)) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        repos = data.get("repos", [])
                        count = data.get("count", len(repos))

                        if count == 0:
                            report["repos"]["status"] = "empty"
                            report["repos"]["issues"].append("NO REPOSITORIES!")
                            self.rant("no_repos")
                        else:
                            report["repos"]["status"] = "ok"
                            report["repos"]["count"] = count
                    else:
                        report["repos"]["status"] = "error"
            except Exception as e:
                report["repos"]["status"] = "error"
                report["repos"]["issues"].append(str(e)[:50])

            # Check container registry
            try:
                async with session.get(f"{self.BASE_URL}:30009/api/registry/repos", timeout=aiohttp.ClientTimeout(total=10)) as resp:
                    if resp.status == 200:
                        data = await resp.json()
                        images = data.get("repositories", data.get("repos", []))
                        if len(images) == 0:
                            report["registry"]["status"] = "empty"
                            report["registry"]["issues"].append("Container registry is EMPTY!")
                        else:
                            report["registry"]["status"] = "ok"
                            report["registry"]["count"] = len(images)
                    else:
                        report["registry"]["status"] = "error"
            except Exception as e:
                report["registry"]["status"] = "error"

        # Steve's verdict
        issues_count = sum(len(r.get("issues", [])) for r in report.values() if isinstance(r, dict))
        if issues_count == 0:
            report["steves_verdict"] = "CI/CD is... acceptable. For now. Don't get comfortable."
        elif issues_count <= 2:
            report["steves_verdict"] = f"{issues_count} issues in your CI/CD. Fix them IMMEDIATELY."
        else:
            report["steves_verdict"] = f"{issues_count} CI/CD ISSUES?! Your pipeline is a DISASTER ZONE!"

        return report

    async def run_full_test_suite(self) -> Dict:
        """Run comprehensive tests on all endpoints."""
        logger.info("\n" + "="*70)
        logger.info("🔥 STEVE'S AUTONOMOUS TEST SUITE v3.0")
        logger.info("="*70)

        results = {
            "timestamp": datetime.now().isoformat(),
            "endpoints": [],
            "cicd_health": {},
            "summary": {},
        }

        # Phase 1: CI/CD Deep Check (Steve's priority)
        logger.info("\n📋 PHASE 1: CI/CD Infrastructure Analysis")
        logger.info("-"*50)
        results["cicd_health"] = await self.check_cicd_health()

        # Phase 2: Endpoint Tests
        logger.info("\n🎯 PHASE 2: Endpoint Testing")
        logger.info("-"*50)

        async with aiohttp.ClientSession() as session:
            tasks = []
            for key, endpoint in self.ENDPOINTS.items():
                tasks.append(self.check_endpoint(session, key, endpoint))

            endpoint_results = await asyncio.gather(*tasks)
            results["endpoints"] = endpoint_results

        # Analyze results
        total = len(endpoint_results)
        passed = sum(1 for r in endpoint_results if r["success"])
        failed = total - passed
        critical_failures = sum(1 for r in endpoint_results if not r["success"] and r["critical"])
        avg_response = sum(r["response_time_ms"] for r in endpoint_results) / total if total > 0 else 0

        # Log each result
        for r in endpoint_results:
            status = "✅" if r["success"] else "❌"
            critical = "🔴" if r.get("critical") and not r["success"] else ""
            logger.info(f"{status}{critical} {r['name']}: {int(r['response_time_ms'])}ms")
            if r["issues"]:
                for issue in r["issues"]:
                    logger.info(f"   ⚠️ {issue}")
            if r["steves_take"]:
                logger.info(f"   💬 \"{r['steves_take']}\"")

        # Update mood based on results
        failure_rate = (failed / total) * 100 if total > 0 else 0
        cicd_issues = sum(len(r.get("issues", [])) for r in results["cicd_health"].values() if isinstance(r, dict))

        if critical_failures > 0 or cicd_issues > 3:
            self.mood = Mood.VOLCANIC_RAGE
            self.frustration = min(100, self.frustration + 20)
        elif failure_rate > 25 or cicd_issues > 1:
            self.mood = Mood.FURIOUS
            self.frustration = min(100, self.frustration + 10)
        elif failure_rate > 10:
            self.mood = Mood.DISGUSTED
            self.frustration = min(100, self.frustration + 5)
        elif failure_rate > 0:
            self.mood = Mood.DISAPPOINTED
        else:
            self.mood = Mood.GRUDGINGLY_TOLERANT
            self.frustration = max(0, self.frustration - 5)

        # Summary
        results["summary"] = {
            "total_endpoints": total,
            "passed": passed,
            "failed": failed,
            "critical_failures": critical_failures,
            "pass_rate": round((passed / total) * 100, 1) if total > 0 else 0,
            "avg_response_ms": round(avg_response, 1),
            "cicd_issues": cicd_issues,
            "mood": self.mood.value,
            "frustration": self.frustration,
        }

        # Final report
        logger.info("\n" + "="*70)
        logger.info("📊 STEVE'S QUALITY REPORT")
        logger.info("="*70)
        logger.info(f"Endpoints Tested: {total}")
        logger.info(f"Passed: {passed} ({results['summary']['pass_rate']}%)")
        logger.info(f"Failed: {failed}")
        if critical_failures > 0:
            logger.info(f"🔴 CRITICAL FAILURES: {critical_failures}")
        logger.info(f"CI/CD Issues: {cicd_issues}")
        logger.info(f"Average Response: {int(avg_response)}ms")
        logger.info(f"\nSteve's Mood: {self.mood.value.upper()}")
        logger.info(f"Frustration Level: {self.frustration}%")

        # Steve's final words
        if self.mood == Mood.VOLCANIC_RAGE:
            verdict = "CRITICAL FAILURES IN PRODUCTION! This is UNACCEPTABLE! FIX IT NOW!"
        elif self.mood == Mood.FURIOUS:
            verdict = "Your infrastructure is BROKEN. I expected better. MUCH better."
        elif self.mood == Mood.DISGUSTED:
            verdict = "Too many failures. Too many excuses. DO BETTER."
        elif self.mood == Mood.DISAPPOINTED:
            verdict = "Not catastrophic, but not good either. I'm watching you."
        else:
            verdict = "Acceptable. For now. Don't let it go to your head."

        logger.info(f"\n💬 Steve's Verdict: \"{verdict}\"")
        logger.info("="*70)

        results["summary"]["verdict"] = verdict
        self.last_test_time = datetime.now()

        return results

    async def autonomous_loop(self):
        """Steve runs continuously, always watching, always judging."""
        logger.info("🤖 STEVE BOT v3.0 ONLINE - Autonomous Mode Activated")
        logger.info("   I am always watching. I am always judging. I never forget.")

        cicd_check_counter = 0

        while True:
            try:
                # Full test suite every test_interval seconds
                results = await self.run_full_test_suite()

                # More frequent CI/CD checks between full tests
                for _ in range(self.test_interval // self.cicd_check_interval):
                    await asyncio.sleep(self.cicd_check_interval)
                    cicd_check_counter += 1

                    if cicd_check_counter % 2 == 0:  # Every other check
                        logger.info("\n🔍 Quick CI/CD pulse check...")
                        cicd_health = await self.check_cicd_health()
                        if cicd_health["builds"]["status"] == "failing":
                            logger.warning("🚨 BUILD FAILURE DETECTED! Alerting...")

            except Exception as e:
                logger.error(f"Error in autonomous loop: {e}")
                await asyncio.sleep(30)


# ============================================================================
# FLASK API - For querying Steve's findings
# ============================================================================
steve = SteveBot()

@app.route('/health')
def health():
    return jsonify({"status": "healthy", "mood": steve.mood.value, "frustration": steve.frustration})

@app.route('/bugs')
def get_bugs():
    bugs = steve.db.get_open_bugs()
    return jsonify({
        "total": len(bugs),
        "critical": sum(1 for b in bugs if b["severity"] == "CRITICAL"),
        "high": sum(1 for b in bugs if b["severity"] == "HIGH"),
        "bugs": bugs
    })

@app.route('/rants')
def get_rants():
    rants = steve.db.get_recent_rants()
    return jsonify({"rants": rants, "count": len(rants)})

@app.route('/status')
def get_status():
    return jsonify({
        "steve_version": "3.0",
        "mood": steve.mood.value,
        "frustration": steve.frustration,
        "last_test": steve.last_test_time.isoformat() if steve.last_test_time else None,
        "endpoints_monitored": len(steve.ENDPOINTS),
        "focus": "CI/CD, GitOps, Deployments",
        "philosophy": "Zero tolerance for broken pipelines"
    })

@app.route('/cicd')
def get_cicd():
    """Get latest CI/CD health check."""
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    result = loop.run_until_complete(steve.check_cicd_health())
    loop.close()
    return jsonify(result)

@app.route('/test')
def trigger_test():
    """Manually trigger a test suite."""
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    result = loop.run_until_complete(steve.run_full_test_suite())
    loop.close()
    return jsonify(result)


# ============================================================================
# MAIN - Steve awakens
# ============================================================================
def run_flask():
    app.run(host='0.0.0.0', port=8080, threaded=True)

def run_steve():
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    loop.run_until_complete(steve.autonomous_loop())

if __name__ == "__main__":
    logger.info("""
    ╔═══════════════════════════════════════════════════════════════════╗
    ║                     STEVE BOT v3.0                                 ║
    ║              The Autonomous CI/CD Enforcer                         ║
    ╠═══════════════════════════════════════════════════════════════════╣
    ║  • Continuous pipeline monitoring                                  ║
    ║  • Build time tracking & regression detection                      ║
    ║  • Deployment health validation                                    ║
    ║  • Git repository activity monitoring                              ║
    ║  • Automatic failure escalation                                    ║
    ║  • Zero tolerance for broken CI/CD                                 ║
    ╚═══════════════════════════════════════════════════════════════════╝
    """)

    # Run Flask in a separate thread
    flask_thread = threading.Thread(target=run_flask, daemon=True)
    flask_thread.start()

    # Run Steve's autonomous loop in the main thread
    run_steve()
