#!/usr/bin/env python3
"""
HolmOS Task Processor - Polls Steve's task queue and outputs tasks for Claude Code.

This script fetches pending tasks from Steve's /api/tasks endpoint and outputs them
in a format that can be consumed by Claude Code for automated fixing.

Usage:
    python task_processor.py                    # List pending tasks
    python task_processor.py --json             # Output as JSON
    python task_processor.py --mark-complete 1  # Mark task #1 as complete
    python task_processor.py --watch            # Continuous polling mode
"""

import argparse
import json
import sys
import time
import urllib.request
import urllib.error

# Configuration
STEVE_URL = "http://192.168.8.197:30099"  # Steve's NodePort
POLL_INTERVAL = 60  # seconds between polls in watch mode
MAX_TASKS_PER_BATCH = 10


def fetch_tasks(status="pending", limit=20):
    """Fetch tasks from Steve's API."""
    url = f"{STEVE_URL}/api/tasks?status={status}&limit={limit}"
    try:
        with urllib.request.urlopen(url, timeout=10) as response:
            return json.loads(response.read().decode())
    except urllib.error.URLError as e:
        print(f"Error connecting to Steve: {e}", file=sys.stderr)
        return None
    except json.JSONDecodeError as e:
        print(f"Error parsing response: {e}", file=sys.stderr)
        return None


def mark_task_complete(task_id, completed_by="claude-code"):
    """Mark a task as completed."""
    url = f"{STEVE_URL}/api/tasks/{task_id}/complete"
    data = json.dumps({"completed_by": completed_by}).encode()
    req = urllib.request.Request(url, data=data, method="POST")
    req.add_header("Content-Type", "application/json")

    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return json.loads(response.read().decode())
    except Exception as e:
        print(f"Error marking task complete: {e}", file=sys.stderr)
        return None


def update_task_status(task_id, status):
    """Update task status (pending, in_progress, completed, failed)."""
    url = f"{STEVE_URL}/api/tasks/{task_id}/status"
    data = json.dumps({"status": status}).encode()
    req = urllib.request.Request(url, data=data, method="PUT")
    req.add_header("Content-Type", "application/json")

    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return json.loads(response.read().decode())
    except Exception as e:
        print(f"Error updating task status: {e}", file=sys.stderr)
        return None


def format_task_for_claude(task):
    """Format a task as a prompt for Claude Code."""
    return f"""
## Task #{task['id']}: {task['title']}

**Type:** {task['task_type']}
**Priority:** {task['priority']} (1=critical, 10=low)
**Service:** {task.get('affected_service', 'N/A')}
**Reported by:** {task['reported_by']}
**File:** {task.get('file_path', 'N/A')}

### Description:
{task['description']}

### Instructions for Claude Code:
1. Investigate and fix this issue
2. Test the fix if possible
3. When done, mark complete with: `python task_processor.py --mark-complete {task['id']}`
"""


def print_tasks(tasks, as_json=False):
    """Print tasks in human-readable or JSON format."""
    if as_json:
        print(json.dumps(tasks, indent=2))
        return

    if not tasks.get("tasks"):
        print("No pending tasks!")
        return

    print(f"\n{'='*60}")
    print(f"  HOLMOS TASK QUEUE - {tasks['count']} pending tasks")
    print(f"{'='*60}\n")

    for task in tasks["tasks"]:
        priority_label = {1: "CRITICAL", 2: "HIGH", 3: "HIGH",
                         4: "MEDIUM", 5: "MEDIUM", 6: "MEDIUM",
                         7: "LOW", 8: "LOW", 9: "LOW", 10: "LOWEST"}.get(task['priority'], "MEDIUM")

        print(f"[#{task['id']}] [{priority_label}] {task['title']}")
        print(f"       Type: {task['task_type']} | Service: {task.get('affected_service', 'N/A')}")
        print(f"       {task['description'][:100]}...")
        print()


def watch_mode():
    """Continuously poll for tasks and output them."""
    print(f"Watching for tasks (polling every {POLL_INTERVAL}s)...")
    print("Press Ctrl+C to stop\n")

    seen_tasks = set()

    while True:
        try:
            result = fetch_tasks(limit=MAX_TASKS_PER_BATCH)
            if result and result.get("tasks"):
                for task in result["tasks"]:
                    if task["id"] not in seen_tasks:
                        seen_tasks.add(task["id"])
                        print(f"\n{'='*60}")
                        print(f"NEW TASK DETECTED!")
                        print(format_task_for_claude(task))

            time.sleep(POLL_INTERVAL)

        except KeyboardInterrupt:
            print("\nStopping watch mode.")
            break


def main():
    parser = argparse.ArgumentParser(description="HolmOS Task Processor")
    parser.add_argument("--json", action="store_true", help="Output as JSON")
    parser.add_argument("--watch", action="store_true", help="Continuous polling mode")
    parser.add_argument("--mark-complete", type=int, metavar="ID", help="Mark task as complete")
    parser.add_argument("--mark-in-progress", type=int, metavar="ID", help="Mark task as in progress")
    parser.add_argument("--mark-failed", type=int, metavar="ID", help="Mark task as failed")
    parser.add_argument("--status", default="pending", help="Filter by status")
    parser.add_argument("--limit", type=int, default=20, help="Max tasks to fetch")
    parser.add_argument("--prompt", type=int, metavar="ID", help="Output task as Claude Code prompt")

    args = parser.parse_args()

    if args.mark_complete:
        result = mark_task_complete(args.mark_complete)
        if result:
            print(f"Task #{args.mark_complete} marked as complete!")
        return

    if args.mark_in_progress:
        result = update_task_status(args.mark_in_progress, "in_progress")
        if result:
            print(f"Task #{args.mark_in_progress} marked as in_progress!")
        return

    if args.mark_failed:
        result = update_task_status(args.mark_failed, "failed")
        if result:
            print(f"Task #{args.mark_failed} marked as failed!")
        return

    if args.watch:
        watch_mode()
        return

    if args.prompt:
        result = fetch_tasks(limit=100)
        if result:
            for task in result.get("tasks", []):
                if task["id"] == args.prompt:
                    print(format_task_for_claude(task))
                    return
            print(f"Task #{args.prompt} not found")
        return

    # Default: list tasks
    result = fetch_tasks(status=args.status, limit=args.limit)
    if result:
        print_tasks(result, as_json=args.json)


if __name__ == "__main__":
    main()
