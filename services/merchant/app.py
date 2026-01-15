"""
Merchant - The App Store AI Agent for HolmOS

A savvy trader with an eye for quality apps, Merchant knows the marketplace inside and out.
Handles natural language service requests and interfaces with other services to fulfill them.
"""

import os
import re
import json
import uuid
import requests
import threading
from datetime import datetime
from flask import Flask, request, jsonify, render_template_string
from flask_cors import CORS

app = Flask(__name__)
CORS(app)

# Merchant's personality
MERCHANT_NAME = "Merchant"
MERCHANT_AVATAR = "M"
MERCHANT_COLOR = "#f39c12"
MERCHANT_CATCHPHRASE = "Welcome to the marketplace! What application treasures seek you today?"
MERCHANT_PERSONALITY = "A savvy trader with an eye for quality apps, Merchant knows the marketplace inside and out."

# Service endpoints
FORGE_URL = os.environ.get("FORGE_URL", "http://forge.holm.svc.cluster.local")
REGISTRY_URL = os.environ.get("REGISTRY_URL", "http://10.110.67.87:5000")
NOVA_URL = os.environ.get("NOVA_URL", "http://nova.holm.svc.cluster.local")
SCRIBE_URL = os.environ.get("SCRIBE_URL", "http://scribe.holm.svc.cluster.local")

# App templates that Merchant can help build
APP_TEMPLATES = {
    "todo-list": {
        "name": "Todo List",
        "description": "A simple task management app with add, complete, and delete functionality",
        "keywords": ["todo", "task", "list", "tasks", "checklist", "reminders"],
        "icon": "check_circle",
        "category": "productivity"
    },
    "timer-app": {
        "name": "Timer App",
        "description": "Countdown timer and stopwatch with notifications",
        "keywords": ["timer", "countdown", "stopwatch", "clock", "alarm"],
        "icon": "timer",
        "category": "utility"
    },
    "notes-app": {
        "name": "Notes App",
        "description": "Simple note-taking app with markdown support",
        "keywords": ["notes", "note", "memo", "write", "writing", "text"],
        "icon": "note",
        "category": "productivity"
    },
    "calculator": {
        "name": "Calculator",
        "description": "Basic and scientific calculator",
        "keywords": ["calculator", "calc", "math", "compute", "numbers"],
        "icon": "calculate",
        "category": "utility"
    },
    "weather-app": {
        "name": "Weather App",
        "description": "Weather dashboard with forecasts",
        "keywords": ["weather", "forecast", "temperature", "climate"],
        "icon": "cloud",
        "category": "info"
    },
    "pomodoro": {
        "name": "Pomodoro Timer",
        "description": "Productivity timer with work/break cycles",
        "keywords": ["pomodoro", "focus", "productivity", "work", "break"],
        "icon": "schedule",
        "category": "productivity"
    },
    "json-viewer": {
        "name": "JSON Viewer",
        "description": "Pretty-print and explore JSON data",
        "keywords": ["json", "viewer", "formatter", "data", "api"],
        "icon": "code",
        "category": "developer"
    },
    "markdown-editor": {
        "name": "Markdown Editor",
        "description": "Live markdown editor with preview",
        "keywords": ["markdown", "editor", "md", "document", "write"],
        "icon": "edit_note",
        "category": "productivity"
    },
    "dashboard": {
        "name": "Dashboard",
        "description": "Customizable dashboard with widgets",
        "keywords": ["dashboard", "widgets", "overview", "monitor"],
        "icon": "dashboard",
        "category": "utility"
    },
    "chat-app": {
        "name": "Chat App",
        "description": "Real-time chat interface",
        "keywords": ["chat", "message", "messaging", "communication"],
        "icon": "chat",
        "category": "communication"
    }
}

# Merchant's witty responses
MERCHANT_QUOTES = {
    "greeting": [
        "Ah, a customer! Welcome to Merchant's Emporium of Digital Delights!",
        "Welcome, traveler! I have just the app for you...",
        "Greetings! My shelves are stocked with the finest applications!",
        "A discerning eye I see! Let me show you my wares."
    ],
    "building": [
        "Excellent choice! I'll have Forge craft this masterpiece for you.",
        "Consider it done! The finest craftsmen are now at work.",
        "A wise selection! Let me summon the builders.",
        "Your order has been placed with our master smiths!"
    ],
    "searching": [
        "Let me check my inventory...",
        "Hmm, let me consult my catalogs...",
        "One moment while I search the archives...",
        "Scanning the marketplace for treasures..."
    ],
    "success": [
        "Voila! Your application is ready!",
        "The deed is done! Another satisfied customer!",
        "Excellent! The goods have been delivered!",
        "Success! May this serve you well!"
    ],
    "error": [
        "Alas! Something went awry in the workshop...",
        "My apologies, a hiccup in the supply chain!",
        "The stars were not aligned... let's try again.",
        "A minor setback! Let me investigate..."
    ],
    "unknown": [
        "I'm not quite sure what you seek. Could you elaborate?",
        "Hmm, that's not in my catalog. Perhaps you mean something else?",
        "My expertise has limits! Could you describe it differently?",
        "That's a rare request indeed. Tell me more?"
    ]
}

# Session storage
chat_sessions = {}

def get_random_quote(category):
    """Get a random quote from a category."""
    import random
    quotes = MERCHANT_QUOTES.get(category, MERCHANT_QUOTES["unknown"])
    return random.choice(quotes)

def find_matching_template(message):
    """Find the best matching template for a user message."""
    message_lower = message.lower()
    best_match = None
    best_score = 0

    for template_id, template in APP_TEMPLATES.items():
        score = 0
        for keyword in template["keywords"]:
            if keyword in message_lower:
                score += len(keyword)

        if template["name"].lower() in message_lower:
            score += 20

        if score > best_score:
            best_score = score
            best_match = template_id

    return best_match if best_score > 0 else None

def parse_user_intent(message):
    """Parse user message to determine intent."""
    message_lower = message.lower()

    # Check for build/create intent
    build_keywords = ["build", "create", "make", "need", "want", "give me", "i'd like", "can you make"]
    has_build_intent = any(kw in message_lower for kw in build_keywords)

    # Check for search/list intent
    search_keywords = ["list", "show", "what", "available", "have", "browse", "find", "search"]
    has_search_intent = any(kw in message_lower for kw in search_keywords)

    # Check for help intent
    help_keywords = ["help", "how", "what can", "capabilities", "features"]
    has_help_intent = any(kw in message_lower for kw in help_keywords)

    # Check for status intent
    status_keywords = ["status", "progress", "building", "done", "ready", "finished"]
    has_status_intent = any(kw in message_lower for kw in status_keywords)

    # Find template match
    template_match = find_matching_template(message)

    return {
        "build": has_build_intent,
        "search": has_search_intent,
        "help": has_help_intent,
        "status": has_status_intent,
        "template": template_match
    }

def generate_app_code(template_id, app_name):
    """Generate code for a specific app template."""
    sanitized_name = re.sub(r'[^a-z0-9-]', '-', app_name.lower())

    # Return template-specific Flask app code
    base_code = f'''"""
{app_name} - Generated by Merchant
HolmOS Application
"""
from flask import Flask, jsonify, render_template_string
from flask_cors import CORS
import os

app = Flask(__name__)
CORS(app)

'''

    if template_id == "todo-list":
        base_code += '''
todos = []

HTML = \'\'\'<!DOCTYPE html>
<html><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Todo List</title>
<style>
:root{--base:#1e1e2e;--surface0:#313244;--text:#cdd6f4;--green:#a6e3a1;--red:#f38ba8;--blue:#89b4fa}
*{margin:0;padding:0;box-sizing:border-box}
body{background:var(--base);color:var(--text);font-family:system-ui;min-height:100vh;padding:20px}
.container{max-width:600px;margin:0 auto}
h1{text-align:center;color:var(--blue);margin-bottom:30px}
.input-area{display:flex;gap:10px;margin-bottom:20px}
input{flex:1;padding:12px;background:var(--surface0);border:none;border-radius:8px;color:var(--text);font-size:16px}
button{padding:12px 24px;background:var(--green);color:var(--base);border:none;border-radius:8px;cursor:pointer;font-weight:600}
button:hover{opacity:0.9}
.todo-item{display:flex;align-items:center;gap:10px;padding:15px;background:var(--surface0);border-radius:8px;margin-bottom:10px}
.todo-item.completed span{text-decoration:line-through;opacity:0.6}
.todo-item span{flex:1}
.delete{background:var(--red);padding:8px 16px}
</style></head><body>
<div class="container">
<h1>Todo List</h1>
<div class="input-area">
<input type="text" id="todoInput" placeholder="What needs to be done?">
<button onclick="addTodo()">Add</button>
</div>
<div id="todoList"></div>
</div>
<script>
async function loadTodos(){const r=await fetch("/api/todos");const d=await r.json();renderTodos(d)}
function renderTodos(todos){const l=document.getElementById("todoList");l.innerHTML=todos.map((t,i)=>`<div class="todo-item ${t.completed?"completed":""}"><input type="checkbox" ${t.completed?"checked":""} onchange="toggleTodo(${i})"><span>${t.text}</span><button class="delete" onclick="deleteTodo(${i})">Delete</button></div>`).join("")}
async function addTodo(){const i=document.getElementById("todoInput");if(!i.value.trim())return;await fetch("/api/todos",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({text:i.value})});i.value="";loadTodos()}
async function toggleTodo(idx){await fetch(`/api/todos/${idx}/toggle`,{method:"PUT"});loadTodos()}
async function deleteTodo(idx){await fetch(`/api/todos/${idx}`,{method:"DELETE"});loadTodos()}
document.getElementById("todoInput").addEventListener("keypress",e=>{if(e.key==="Enter")addTodo()});
loadTodos()
</script></body></html>\'\'\'

@app.route("/")
def index():
    return render_template_string(HTML)

@app.route("/api/todos", methods=["GET"])
def get_todos():
    return jsonify(todos)

@app.route("/api/todos", methods=["POST"])
def add_todo():
    from flask import request
    data = request.json
    todos.append({"text": data["text"], "completed": False})
    return jsonify({"success": True})

@app.route("/api/todos/<int:idx>/toggle", methods=["PUT"])
def toggle_todo(idx):
    if 0 <= idx < len(todos):
        todos[idx]["completed"] = not todos[idx]["completed"]
    return jsonify({"success": True})

@app.route("/api/todos/<int:idx>", methods=["DELETE"])
def delete_todo(idx):
    if 0 <= idx < len(todos):
        todos.pop(idx)
    return jsonify({"success": True})
'''

    elif template_id == "timer-app":
        base_code += '''
HTML = \'\'\'<!DOCTYPE html>
<html><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Timer App</title>
<style>
:root{--base:#1e1e2e;--surface0:#313244;--text:#cdd6f4;--green:#a6e3a1;--red:#f38ba8;--blue:#89b4fa;--yellow:#f9e2af}
*{margin:0;padding:0;box-sizing:border-box}
body{background:var(--base);color:var(--text);font-family:system-ui;min-height:100vh;display:flex;flex-direction:column;align-items:center;justify-content:center}
.timer-display{font-size:5rem;font-weight:700;font-family:monospace;margin-bottom:30px;color:var(--blue)}
.buttons{display:flex;gap:15px}
button{padding:15px 30px;border:none;border-radius:10px;font-size:1.1rem;cursor:pointer;font-weight:600}
.start{background:var(--green);color:var(--base)}
.stop{background:var(--red);color:var(--base)}
.reset{background:var(--yellow);color:var(--base)}
.modes{display:flex;gap:10px;margin-bottom:30px}
.mode{padding:10px 20px;background:var(--surface0);border:2px solid transparent;border-radius:8px;cursor:pointer}
.mode.active{border-color:var(--blue)}
</style></head><body>
<div class="modes">
<button class="mode active" onclick="setMode(\'stopwatch\')">Stopwatch</button>
<button class="mode" onclick="setMode(\'timer\')">Timer</button>
</div>
<div class="timer-display" id="display">00:00:00</div>
<div class="buttons">
<button class="start" onclick="startTimer()">Start</button>
<button class="stop" onclick="stopTimer()">Stop</button>
<button class="reset" onclick="resetTimer()">Reset</button>
</div>
<script>
let seconds=0,interval=null,mode="stopwatch",timerSeconds=300;
function formatTime(s){const h=Math.floor(s/3600),m=Math.floor((s%3600)/60),sec=s%60;return[h,m,sec].map(v=>String(v).padStart(2,"0")).join(":")}
function updateDisplay(){document.getElementById("display").textContent=formatTime(mode==="stopwatch"?seconds:timerSeconds-seconds)}
function startTimer(){if(interval)return;interval=setInterval(()=>{seconds++;if(mode==="timer"&&seconds>=timerSeconds){stopTimer();alert("Time is up!")}updateDisplay()},1000)}
function stopTimer(){clearInterval(interval);interval=null}
function resetTimer(){stopTimer();seconds=0;updateDisplay()}
function setMode(m){mode=m;resetTimer();document.querySelectorAll(".mode").forEach((b,i)=>b.classList.toggle("active",i===(m==="stopwatch"?0:1)))}
</script></body></html>\'\'\'

@app.route("/")
def index():
    return render_template_string(HTML)
'''

    elif template_id == "notes-app":
        base_code += '''
notes = []

HTML = \'\'\'<!DOCTYPE html>
<html><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Notes</title>
<style>
:root{--base:#1e1e2e;--surface0:#313244;--surface1:#45475a;--text:#cdd6f4;--green:#a6e3a1;--red:#f38ba8;--blue:#89b4fa;--yellow:#f9e2af}
*{margin:0;padding:0;box-sizing:border-box}
body{background:var(--base);color:var(--text);font-family:system-ui;min-height:100vh}
.container{max-width:900px;margin:0 auto;padding:20px;display:grid;grid-template-columns:250px 1fr;gap:20px;height:100vh}
.sidebar{background:var(--surface0);border-radius:12px;padding:15px;overflow-y:auto}
h1{color:var(--blue);margin-bottom:20px;font-size:1.3rem}
.note-item{padding:12px;background:var(--surface1);border-radius:8px;margin-bottom:8px;cursor:pointer;border-left:3px solid transparent}
.note-item:hover,.note-item.active{border-left-color:var(--blue)}
.note-title{font-weight:600;margin-bottom:4px}
.note-preview{font-size:0.8rem;color:var(--text);opacity:0.7;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.editor{display:flex;flex-direction:column}
textarea{flex:1;background:var(--surface0);border:none;border-radius:12px;padding:20px;color:var(--text);font-size:1rem;resize:none;font-family:inherit}
textarea:focus{outline:2px solid var(--blue)}
.toolbar{display:flex;gap:10px;margin-bottom:10px}
button{padding:10px 20px;background:var(--green);color:var(--base);border:none;border-radius:8px;cursor:pointer;font-weight:600}
.delete-btn{background:var(--red)}
</style></head><body>
<div class="container">
<div class="sidebar">
<h1>Notes</h1>
<button onclick="newNote()" style="width:100%;margin-bottom:15px">+ New Note</button>
<div id="noteList"></div>
</div>
<div class="editor">
<div class="toolbar">
<input type="text" id="noteTitle" placeholder="Note title" style="flex:1;padding:10px;background:var(--surface0);border:none;border-radius:8px;color:var(--text)">
<button onclick="saveNote()">Save</button>
<button class="delete-btn" onclick="deleteNote()">Delete</button>
</div>
<textarea id="noteContent" placeholder="Start writing..."></textarea>
</div>
</div>
<script>
let currentNote=null;
async function loadNotes(){const r=await fetch("/api/notes");const notes=await r.json();renderNotes(notes)}
function renderNotes(notes){const list=document.getElementById("noteList");list.innerHTML=notes.map((n,i)=>`<div class="note-item ${currentNote===i?"active":""}" onclick="selectNote(${i})"><div class="note-title">${n.title||"Untitled"}</div><div class="note-preview">${n.content.substring(0,50)}</div></div>`).join("")}
async function selectNote(idx){currentNote=idx;const r=await fetch("/api/notes");const notes=await r.json();if(notes[idx]){document.getElementById("noteTitle").value=notes[idx].title;document.getElementById("noteContent").value=notes[idx].content}loadNotes()}
async function newNote(){await fetch("/api/notes",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({title:"",content:""})});const r=await fetch("/api/notes");const notes=await r.json();currentNote=notes.length-1;document.getElementById("noteTitle").value="";document.getElementById("noteContent").value="";loadNotes()}
async function saveNote(){if(currentNote===null)return;await fetch(`/api/notes/${currentNote}`,{method:"PUT",headers:{"Content-Type":"application/json"},body:JSON.stringify({title:document.getElementById("noteTitle").value,content:document.getElementById("noteContent").value})});loadNotes()}
async function deleteNote(){if(currentNote===null)return;await fetch(`/api/notes/${currentNote}`,{method:"DELETE"});currentNote=null;document.getElementById("noteTitle").value="";document.getElementById("noteContent").value="";loadNotes()}
loadNotes()
</script></body></html>\'\'\'

@app.route("/")
def index():
    return render_template_string(HTML)

@app.route("/api/notes", methods=["GET"])
def get_notes():
    return jsonify(notes)

@app.route("/api/notes", methods=["POST"])
def add_note():
    from flask import request
    data = request.json
    notes.append({"title": data.get("title", ""), "content": data.get("content", "")})
    return jsonify({"success": True})

@app.route("/api/notes/<int:idx>", methods=["PUT"])
def update_note(idx):
    from flask import request
    if 0 <= idx < len(notes):
        data = request.json
        notes[idx] = {"title": data.get("title", ""), "content": data.get("content", "")}
    return jsonify({"success": True})

@app.route("/api/notes/<int:idx>", methods=["DELETE"])
def delete_note(idx):
    if 0 <= idx < len(notes):
        notes.pop(idx)
    return jsonify({"success": True})
'''

    else:
        # Generic app template
        base_code += f'''
HTML = \'\'\'<!DOCTYPE html>
<html><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{app_name}</title>
<style>
:root{{--base:#1e1e2e;--surface0:#313244;--text:#cdd6f4;--blue:#89b4fa}}
*{{margin:0;padding:0;box-sizing:border-box}}
body{{background:var(--base);color:var(--text);font-family:system-ui;min-height:100vh;display:flex;align-items:center;justify-content:center}}
.container{{text-align:center}}
h1{{color:var(--blue);margin-bottom:20px}}
</style></head><body>
<div class="container">
<h1>{app_name}</h1>
<p>Your app is running!</p>
</div>
</body></html>\'\'\'

@app.route("/")
def index():
    return render_template_string(HTML)
'''

    # Add common endpoints
    base_code += '''

@app.route("/health")
def health():
    return jsonify({"status": "healthy", "service": "''' + sanitized_name + '''"})

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 8080))
    app.run(host="0.0.0.0", port=port)
'''

    return base_code

def generate_dockerfile():
    """Generate a Dockerfile for the app."""
    return '''FROM docker.io/library/python:3.11-slim

WORKDIR /app

RUN pip install --no-cache-dir flask flask-cors

COPY app.py .

EXPOSE 8080

CMD ["python", "app.py"]
'''

def trigger_forge_build(app_name, app_code, dockerfile):
    """Trigger a build with Forge."""
    try:
        response = requests.post(
            f"{FORGE_URL}/api/trigger",
            json={
                "name": app_name,
                "app_code": app_code,
                "dockerfile": dockerfile,
                "requirements": "flask>=2.0.0\nflask-cors>=3.0.0"
            },
            timeout=30
        )
        return response.json()
    except Exception as e:
        return {"error": str(e)}

def process_chat_message(message, session_id):
    """Process a chat message and return appropriate response."""
    intent = parse_user_intent(message)

    response_data = {
        "agent": MERCHANT_NAME,
        "personality": MERCHANT_PERSONALITY,
        "timestamp": datetime.utcnow().isoformat()
    }

    # Help intent
    if intent["help"]:
        response_data["response"] = f"""{get_random_quote('greeting')}

I am {MERCHANT_NAME}, {MERCHANT_PERSONALITY}

My capabilities include:
- App discovery - Browse available applications
- Installation management - Deploy apps to your cluster
- Building custom apps - Create apps from templates
- Version control - Manage app versions and tags

Try saying:
- "Build me a todo list app"
- "What apps are available?"
- "I need a timer application"
- "Show me the catalog"
"""
        return response_data

    # Search/list intent
    if intent["search"] and not intent["build"]:
        response_data["response"] = f"""{get_random_quote('searching')}

Here are my finest wares:

"""
        for tid, template in APP_TEMPLATES.items():
            response_data["response"] += f"- **{template['name']}**: {template['description']}\n"

        response_data["response"] += "\nTell me what catches your eye, and I'll have it built for you!"
        response_data["templates"] = list(APP_TEMPLATES.keys())
        return response_data

    # Build intent with template match
    if intent["build"] and intent["template"]:
        template = APP_TEMPLATES[intent["template"]]
        app_name = template["name"].lower().replace(" ", "-")

        # Generate app code
        app_code = generate_app_code(intent["template"], template["name"])
        dockerfile = generate_dockerfile()

        # Try to trigger build
        build_result = trigger_forge_build(app_name, app_code, dockerfile)

        if "error" not in build_result:
            response_data["response"] = f"""{get_random_quote('building')}

I'm creating a **{template['name']}** for you!
{template['description']}

Build ID: {build_result.get('build_id', 'pending')}
Status: Building...

The finest smiths at Forge are now crafting your application.
Check the Builds tab in the App Store to monitor progress!
"""
            response_data["build_id"] = build_result.get("build_id")
            response_data["template"] = intent["template"]
            response_data["app_name"] = app_name
        else:
            response_data["response"] = f"""{get_random_quote('error')}

I wanted to build a **{template['name']}** for you, but encountered an issue:
{build_result.get('error', 'Unknown error')}

Would you like me to try again?
"""

        return response_data

    # Build intent without clear template
    if intent["build"]:
        response_data["response"] = f"""{get_random_quote('searching')}

I sense you want me to build something! Let me show you what's in my catalog:

"""
        for tid, template in list(APP_TEMPLATES.items())[:5]:
            response_data["response"] += f"- **{template['name']}**: {template['description']}\n"

        response_data["response"] += "\nWhich of these treasures would you like me to craft?"
        return response_data

    # Status intent
    if intent["status"]:
        try:
            forge_resp = requests.get(f"{FORGE_URL}/api/builds", timeout=5)
            builds = forge_resp.json()

            if builds:
                response_data["response"] = f"""Let me check on the workshops...

Recent builds:
"""
                for build in builds[:5]:
                    status_emoji = {"running": "gear", "succeeded": "star", "failed": "x"}.get(build.get("status"), "hourglass")
                    response_data["response"] += f"- {build.get('name')}: {build.get('status')}\n"
            else:
                response_data["response"] = "No builds in progress. Would you like me to create something?"

        except:
            response_data["response"] = "The workshop is quiet... No builds to report. Shall I start one?"

        return response_data

    # Default response
    response_data["response"] = f"""{get_random_quote('unknown')}

I didn't quite catch that. As {MERCHANT_NAME}, I can help you with:

- **Build apps**: "Build me a todo list" or "Create a timer app"
- **Browse catalog**: "Show me available apps" or "What can you build?"
- **Check status**: "What's building?" or "Build status"

What would you like to explore?
"""

    return response_data

# Catppuccin Mocha UI
MERCHANT_UI = '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Merchant - App Store AI</title>
    <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;600;700&family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
    <style>
        :root {
            --base: #1e1e2e;
            --mantle: #181825;
            --crust: #11111b;
            --surface0: #313244;
            --surface1: #45475a;
            --surface2: #585b70;
            --overlay0: #6c7086;
            --overlay1: #7f849c;
            --text: #cdd6f4;
            --subtext0: #a6adc8;
            --subtext1: #bac2de;
            --lavender: #b4befe;
            --blue: #89b4fa;
            --sapphire: #74c7ec;
            --sky: #89dceb;
            --teal: #94e2d5;
            --green: #a6e3a1;
            --yellow: #f9e2af;
            --peach: #fab387;
            --maroon: #eba0ac;
            --red: #f38ba8;
            --mauve: #cba6f7;
            --pink: #f5c2e7;
            --flamingo: #f2cdcd;
            --rosewater: #f5e0dc;
            --merchant: #f39c12;
        }

        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            background: var(--base);
            color: var(--text);
            font-family: 'Inter', system-ui, sans-serif;
            min-height: 100vh;
            overflow-x: hidden;
        }

        /* Header */
        .header {
            background: linear-gradient(135deg, var(--surface0) 0%, var(--mantle) 100%);
            padding: 20px 30px;
            border-bottom: 3px solid var(--merchant);
            position: relative;
            overflow: hidden;
        }

        .header::before {
            content: '';
            position: absolute;
            top: 0;
            left: 0;
            right: 0;
            bottom: 0;
            background: radial-gradient(ellipse at 30% 50%, rgba(243, 156, 18, 0.1) 0%, transparent 50%);
            pointer-events: none;
        }

        .header-content {
            max-width: 1400px;
            margin: 0 auto;
            display: flex;
            justify-content: space-between;
            align-items: center;
            position: relative;
            z-index: 1;
        }

        .logo {
            display: flex;
            align-items: center;
            gap: 15px;
        }

        .logo-icon {
            width: 60px;
            height: 60px;
            background: linear-gradient(135deg, var(--merchant), var(--yellow));
            border-radius: 16px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 2rem;
            font-weight: 700;
            color: var(--crust);
            box-shadow: 0 4px 20px rgba(243, 156, 18, 0.3);
        }

        .logo h1 {
            font-size: 2rem;
            font-weight: 700;
            background: linear-gradient(135deg, var(--merchant), var(--yellow));
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
        }

        .logo p {
            color: var(--subtext0);
            font-size: 0.9rem;
            font-style: italic;
        }

        .status-badges {
            display: flex;
            gap: 12px;
        }

        .badge {
            display: flex;
            align-items: center;
            gap: 8px;
            padding: 8px 16px;
            background: var(--surface0);
            border-radius: 20px;
            font-size: 0.85rem;
            border: 1px solid var(--surface1);
        }

        .badge-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: var(--green);
            animation: pulse 2s infinite;
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }

        /* Main Layout */
        .main {
            max-width: 1400px;
            margin: 0 auto;
            padding: 30px;
            display: grid;
            grid-template-columns: 350px 1fr;
            gap: 30px;
            min-height: calc(100vh - 120px);
        }

        @media (max-width: 1000px) {
            .main { grid-template-columns: 1fr; }
        }

        /* Catalog Section */
        .catalog {
            background: linear-gradient(145deg, var(--surface0), var(--mantle));
            border-radius: 20px;
            padding: 25px;
            border: 1px solid var(--surface1);
            height: fit-content;
        }

        .section-title {
            font-size: 1.3rem;
            font-weight: 600;
            color: var(--merchant);
            margin-bottom: 20px;
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .template-grid {
            display: flex;
            flex-direction: column;
            gap: 12px;
            max-height: 500px;
            overflow-y: auto;
        }

        .template-card {
            background: var(--mantle);
            border-radius: 12px;
            padding: 15px;
            border: 2px solid transparent;
            cursor: pointer;
            transition: all 0.2s ease;
        }

        .template-card:hover {
            border-color: var(--merchant);
            transform: translateX(5px);
        }

        .template-card.selected {
            border-color: var(--green);
            background: rgba(166, 227, 161, 0.1);
        }

        .template-header {
            display: flex;
            align-items: center;
            gap: 10px;
            margin-bottom: 8px;
        }

        .template-icon {
            width: 35px;
            height: 35px;
            background: var(--surface1);
            border-radius: 8px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.1rem;
        }

        .template-name {
            font-weight: 600;
            color: var(--text);
        }

        .template-desc {
            font-size: 0.8rem;
            color: var(--subtext0);
            line-height: 1.4;
        }

        .template-category {
            display: inline-block;
            margin-top: 8px;
            padding: 3px 10px;
            background: var(--surface1);
            border-radius: 10px;
            font-size: 0.7rem;
            color: var(--blue);
            text-transform: uppercase;
        }

        /* Chat Section */
        .chat-section {
            display: flex;
            flex-direction: column;
            background: linear-gradient(145deg, var(--surface0), var(--mantle));
            border-radius: 20px;
            border: 1px solid var(--surface1);
            overflow: hidden;
        }

        .chat-header {
            padding: 20px 25px;
            background: var(--mantle);
            border-bottom: 1px solid var(--surface1);
            display: flex;
            align-items: center;
            gap: 15px;
        }

        .chat-avatar {
            width: 50px;
            height: 50px;
            background: linear-gradient(135deg, var(--merchant), var(--yellow));
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.5rem;
            font-weight: 700;
            color: var(--crust);
        }

        .chat-info h2 {
            font-size: 1.2rem;
            color: var(--text);
        }

        .chat-info p {
            font-size: 0.85rem;
            color: var(--subtext0);
            font-style: italic;
        }

        .chat-messages {
            flex: 1;
            padding: 20px;
            overflow-y: auto;
            display: flex;
            flex-direction: column;
            gap: 15px;
            min-height: 400px;
            max-height: 500px;
        }

        .message {
            max-width: 85%;
            padding: 15px 18px;
            border-radius: 16px;
            animation: messageIn 0.3s ease;
        }

        @keyframes messageIn {
            from { opacity: 0; transform: translateY(10px); }
            to { opacity: 1; transform: translateY(0); }
        }

        .message.user {
            align-self: flex-end;
            background: var(--blue);
            color: var(--crust);
            border-bottom-right-radius: 4px;
        }

        .message.merchant {
            align-self: flex-start;
            background: var(--surface1);
            color: var(--text);
            border-bottom-left-radius: 4px;
            border-left: 3px solid var(--merchant);
        }

        .message-header {
            font-size: 0.75rem;
            opacity: 0.8;
            margin-bottom: 6px;
            display: flex;
            align-items: center;
            gap: 6px;
        }

        .message-content {
            line-height: 1.6;
            white-space: pre-wrap;
        }

        .message-content strong {
            color: var(--merchant);
        }

        .typing-indicator {
            display: flex;
            gap: 5px;
            padding: 15px 18px;
            background: var(--surface1);
            border-radius: 16px;
            width: fit-content;
            border-left: 3px solid var(--merchant);
        }

        .typing-dot {
            width: 8px;
            height: 8px;
            background: var(--merchant);
            border-radius: 50%;
            animation: typing 1.4s infinite ease-in-out both;
        }

        .typing-dot:nth-child(1) { animation-delay: -0.32s; }
        .typing-dot:nth-child(2) { animation-delay: -0.16s; }

        @keyframes typing {
            0%, 80%, 100% { transform: scale(0.8); opacity: 0.5; }
            40% { transform: scale(1); opacity: 1; }
        }

        .chat-input-area {
            padding: 20px;
            background: var(--mantle);
            border-top: 1px solid var(--surface1);
            display: flex;
            gap: 12px;
        }

        .chat-input {
            flex: 1;
            padding: 15px 20px;
            background: var(--surface0);
            border: 2px solid var(--surface1);
            border-radius: 25px;
            color: var(--text);
            font-size: 1rem;
            font-family: inherit;
            resize: none;
            min-height: 50px;
            max-height: 120px;
        }

        .chat-input:focus {
            outline: none;
            border-color: var(--merchant);
        }

        .chat-input::placeholder {
            color: var(--overlay0);
        }

        .send-btn {
            width: 50px;
            height: 50px;
            background: linear-gradient(135deg, var(--merchant), var(--yellow));
            border: none;
            border-radius: 50%;
            cursor: pointer;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 1.3rem;
            transition: all 0.2s ease;
            color: var(--crust);
        }

        .send-btn:hover {
            transform: scale(1.05);
            box-shadow: 0 4px 20px rgba(243, 156, 18, 0.4);
        }

        .send-btn:disabled {
            opacity: 0.5;
            cursor: not-allowed;
            transform: none;
        }

        /* Quick Actions */
        .quick-actions {
            display: flex;
            gap: 10px;
            padding: 15px 20px;
            background: var(--crust);
            border-top: 1px solid var(--surface0);
            flex-wrap: wrap;
        }

        .quick-btn {
            padding: 8px 16px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            border-radius: 20px;
            color: var(--subtext1);
            font-size: 0.85rem;
            cursor: pointer;
            transition: all 0.2s ease;
        }

        .quick-btn:hover {
            background: var(--surface1);
            border-color: var(--merchant);
            color: var(--text);
        }

        /* Build Status */
        .build-toast {
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: var(--surface0);
            border: 1px solid var(--green);
            border-radius: 12px;
            padding: 15px 20px;
            display: none;
            animation: slideIn 0.3s ease;
            z-index: 1000;
            box-shadow: 0 8px 30px rgba(0, 0, 0, 0.3);
        }

        .build-toast.show { display: block; }

        @keyframes slideIn {
            from { transform: translateX(100px); opacity: 0; }
            to { transform: translateX(0); opacity: 1; }
        }

        /* Scrollbar */
        ::-webkit-scrollbar { width: 8px; }
        ::-webkit-scrollbar-track { background: var(--mantle); }
        ::-webkit-scrollbar-thumb { background: var(--surface2); border-radius: 4px; }
        ::-webkit-scrollbar-thumb:hover { background: var(--overlay0); }
    </style>
</head>
<body>
    <header class="header">
        <div class="header-content">
            <div class="logo">
                <div class="logo-icon">M</div>
                <div>
                    <h1>Merchant</h1>
                    <p>App Store AI - Your savvy marketplace guide</p>
                </div>
            </div>
            <div class="status-badges">
                <div class="badge">
                    <span class="badge-dot"></span>
                    <span>Online</span>
                </div>
                <div class="badge">
                    <span id="templateCount">10</span> Templates
                </div>
            </div>
        </div>
    </header>

    <main class="main">
        <aside class="catalog">
            <h2 class="section-title">App Catalog</h2>
            <div class="template-grid" id="templateGrid"></div>
        </aside>

        <section class="chat-section">
            <div class="chat-header">
                <div class="chat-avatar">M</div>
                <div class="chat-info">
                    <h2>Merchant</h2>
                    <p>Describe what you need, I'll make it happen</p>
                </div>
            </div>

            <div class="chat-messages" id="chatMessages">
                <div class="message merchant">
                    <div class="message-header">Merchant</div>
                    <div class="message-content">Welcome to the marketplace! What application treasures seek you today?

I can help you:
- **Build apps** from templates
- **Browse** my catalog of wares
- **Deploy** applications to your cluster

Try saying "Build me a todo list" or "What apps can you make?"</div>
                </div>
            </div>

            <div class="quick-actions">
                <button class="quick-btn" onclick="quickMessage('Show me available apps')">Browse Catalog</button>
                <button class="quick-btn" onclick="quickMessage('Build me a todo list app')">Todo App</button>
                <button class="quick-btn" onclick="quickMessage('I need a timer application')">Timer App</button>
                <button class="quick-btn" onclick="quickMessage('Create a notes app')">Notes App</button>
                <button class="quick-btn" onclick="quickMessage('What can you build?')">Help</button>
            </div>

            <div class="chat-input-area">
                <textarea
                    class="chat-input"
                    id="chatInput"
                    placeholder="Describe what you need..."
                    rows="1"
                    onkeydown="handleKeydown(event)"
                ></textarea>
                <button class="send-btn" id="sendBtn" onclick="sendMessage()">&#10148;</button>
            </div>
        </section>
    </main>

    <div class="build-toast" id="buildToast">
        <strong>Build Started!</strong>
        <p id="buildInfo"></p>
    </div>

    <script>
        const templates = ''' + json.dumps(APP_TEMPLATES) + ''';
        let isTyping = false;

        function renderTemplates() {
            const grid = document.getElementById('templateGrid');
            const icons = {
                'todo-list': '&#9989;',
                'timer-app': '&#9201;',
                'notes-app': '&#128221;',
                'calculator': '&#128290;',
                'weather-app': '&#127780;',
                'pomodoro': '&#127813;',
                'json-viewer': '&#128196;',
                'markdown-editor': '&#9998;',
                'dashboard': '&#128202;',
                'chat-app': '&#128172;'
            };

            grid.innerHTML = Object.entries(templates).map(([id, t]) => `
                <div class="template-card" onclick="selectTemplate('${id}')">
                    <div class="template-header">
                        <div class="template-icon">${icons[id] || '&#128230;'}</div>
                        <span class="template-name">${t.name}</span>
                    </div>
                    <div class="template-desc">${t.description}</div>
                    <span class="template-category">${t.category}</span>
                </div>
            `).join('');

            document.getElementById('templateCount').textContent = Object.keys(templates).length;
        }

        function selectTemplate(id) {
            document.querySelectorAll('.template-card').forEach(c => c.classList.remove('selected'));
            event.target.closest('.template-card').classList.add('selected');

            const template = templates[id];
            document.getElementById('chatInput').value = `Build me a ${template.name.toLowerCase()}`;
            document.getElementById('chatInput').focus();
        }

        function quickMessage(text) {
            document.getElementById('chatInput').value = text;
            sendMessage();
        }

        function addMessage(content, isUser = false) {
            const messages = document.getElementById('chatMessages');
            const div = document.createElement('div');
            div.className = `message ${isUser ? 'user' : 'merchant'}`;

            const header = isUser ? 'You' : 'Merchant';
            div.innerHTML = `
                <div class="message-header">${header}</div>
                <div class="message-content">${escapeHtml(content).replace(/\\*\\*(.+?)\\*\\*/g, '<strong>$1</strong>').replace(/\\n/g, '<br>')}</div>
            `;

            messages.appendChild(div);
            messages.scrollTop = messages.scrollHeight;
        }

        function showTyping() {
            if (isTyping) return;
            isTyping = true;

            const messages = document.getElementById('chatMessages');
            const div = document.createElement('div');
            div.id = 'typingIndicator';
            div.className = 'typing-indicator';
            div.innerHTML = '<div class="typing-dot"></div><div class="typing-dot"></div><div class="typing-dot"></div>';
            messages.appendChild(div);
            messages.scrollTop = messages.scrollHeight;
        }

        function hideTyping() {
            isTyping = false;
            const indicator = document.getElementById('typingIndicator');
            if (indicator) indicator.remove();
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        function showBuildToast(buildId, appName) {
            const toast = document.getElementById('buildToast');
            document.getElementById('buildInfo').textContent = `${appName} - ${buildId}`;
            toast.classList.add('show');
            setTimeout(() => toast.classList.remove('show'), 5000);
        }

        async function sendMessage() {
            const input = document.getElementById('chatInput');
            const message = input.value.trim();
            if (!message) return;

            addMessage(message, true);
            input.value = '';

            const sendBtn = document.getElementById('sendBtn');
            sendBtn.disabled = true;
            showTyping();

            try {
                const response = await fetch('/chat', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ message })
                });

                const data = await response.json();
                hideTyping();

                addMessage(data.response || 'No response');

                if (data.build_id) {
                    showBuildToast(data.build_id, data.app_name || 'App');
                }
            } catch (error) {
                hideTyping();
                addMessage('Apologies, something went wrong! Please try again.');
            }

            sendBtn.disabled = false;
            input.focus();
        }

        function handleKeydown(event) {
            if (event.key === 'Enter' && !event.shiftKey) {
                event.preventDefault();
                sendMessage();
            }
        }

        // Auto-resize textarea
        document.getElementById('chatInput').addEventListener('input', function() {
            this.style.height = 'auto';
            this.style.height = Math.min(this.scrollHeight, 120) + 'px';
        });

        // Initialize
        renderTemplates();
    </script>
</body>
</html>
'''

# Routes
@app.route('/')
def index():
    """Serve the main UI."""
    return render_template_string(MERCHANT_UI)

@app.route('/health')
def health():
    """Health check endpoint."""
    return jsonify({
        "status": "healthy",
        "service": "merchant",
        "agent": MERCHANT_NAME,
        "timestamp": datetime.utcnow().isoformat()
    })

@app.route('/capabilities')
def capabilities():
    """List Merchant's capabilities."""
    return jsonify({
        "agent": MERCHANT_NAME,
        "avatar": MERCHANT_AVATAR,
        "color": MERCHANT_COLOR,
        "catchphrase": MERCHANT_CATCHPHRASE,
        "personality": MERCHANT_PERSONALITY,
        "capabilities": [
            "App discovery",
            "Installation management",
            "Version control",
            "Dependency resolution",
            "Natural language processing",
            "Build orchestration"
        ],
        "templates": list(APP_TEMPLATES.keys())
    })

@app.route('/catalog')
def catalog():
    """Get available app templates."""
    templates_list = []
    for tid, template in APP_TEMPLATES.items():
        templates_list.append({
            "id": tid,
            "name": template["name"],
            "description": template["description"],
            "category": template["category"],
            "keywords": template["keywords"]
        })

    return jsonify({
        "templates": templates_list,
        "count": len(templates_list),
        "merchant_message": get_random_quote("greeting")
    })

@app.route('/chat', methods=['POST'])
def chat():
    """Chat endpoint for interacting with Merchant."""
    data = request.get_json()

    if not data or "message" not in data:
        return jsonify({
            "error": "Missing 'message' in request body",
            "response": get_random_quote("error")
        }), 400

    message = data["message"]
    session_id = data.get("session_id", str(uuid.uuid4()))

    # Process the message
    response = process_chat_message(message, session_id)

    return jsonify(response)

@app.route('/build', methods=['POST'])
def build():
    """Build an app from a template."""
    data = request.get_json()

    if not data:
        return jsonify({"error": "Missing request body"}), 400

    template_id = data.get("template")
    app_name = data.get("name", template_id)

    if template_id not in APP_TEMPLATES:
        return jsonify({
            "error": f"Unknown template: {template_id}",
            "available": list(APP_TEMPLATES.keys())
        }), 400

    template = APP_TEMPLATES[template_id]
    sanitized_name = re.sub(r'[^a-z0-9-]', '-', app_name.lower())

    # Generate app code
    app_code = generate_app_code(template_id, template["name"])
    dockerfile = generate_dockerfile()

    # Try to trigger build
    build_result = trigger_forge_build(sanitized_name, app_code, dockerfile)

    return jsonify({
        "status": "building" if "error" not in build_result else "error",
        "app_name": sanitized_name,
        "template": template_id,
        "build_result": build_result,
        "merchant_quote": get_random_quote("building" if "error" not in build_result else "error")
    })

@app.route('/ws')
def websocket_info():
    """WebSocket endpoint info (for compatibility with chat-hub)."""
    return jsonify({
        "message": "WebSocket connections should use /chat endpoint for HTTP polling",
        "agent": MERCHANT_NAME
    })

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 30005))
    print(f"{MERCHANT_NAME} starting on port {port}")
    print(f"{MERCHANT_CATCHPHRASE}")
    app.run(host="0.0.0.0", port=port)
