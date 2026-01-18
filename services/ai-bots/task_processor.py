#!/usr/bin/env python3
"""
HolmOS Task Processor - Claude Code Integration for Auto-Healing

This script connects Claude Code to Steve's task queue, enabling automatic
bug fixing and issue resolution.

Usage:
    # List pending tasks
    python task_processor.py

    # Get next task as Claude Code prompt
    python task_processor.py --next

    # Watch mode - polls for tasks and outputs prompts
    python task_processor.py --watch

    # Mark task complete
    python task_processor.py --complete 1

    # Get task stats
    python task_processor.py --stats

Integration with Claude Code:
    # One-shot: Get next task and work on it
    python task_processor.py --next | claude

    # Continuous mode: Watch for tasks
    python task_processor.py --watch --interval 30
"""

import argparse
import json
import sys
import time
import urllib.request
import urllib.error
import os

# Configuration
STEVE_URL = os.getenv("STEVE_URL", "http://192.168.8.197:30099")
POLL_INTERVAL = 30  # seconds between polls in watch mode

def make_request(url: str, method: str = "GET", data: dict = None) -> dict:
    """Make HTTP request to Steve's API."""
    try:
        if data:
            req = urllib.request.Request(
                url,
                data=json.dumps(data).encode(),
                method=method
            )
            req.add_header("Content-Type", "application/json")
        else:
            req = urllib.request.Request(url, method=method)

        with urllib.request.urlopen(req, timeout=10) as response:
            return json.loads(response.read().decode())
    except urllib.error.URLError as e:
        return {"error": f"Connection error: {e}"}
    except json.JSONDecodeError as e:
        return {"error": f"Parse error: {e}"}

def get_tasks(status: str = "pending", limit: int = 20) -> dict:
    """Get tasks from Steve's API."""
    return make_request(f"{STEVE_URL}/api/tasks?status={status}&limit={limit}")

def get_stats() -> dict:
    """Get task queue statistics."""
    return make_request(f"{STEVE_URL}/api/tasks/stats")

def claim_next_task(claimed_by: str = "claude-code") -> dict:
    """Claim the next pending task."""
    return make_request(
        f"{STEVE_URL}/api/tasks/next",
        method="POST",
        data={"claimed_by": claimed_by}
    )

def complete_task(task_id: int, completed_by: str = "claude-code") -> dict:
    """Mark a task as completed."""
    return make_request(
        f"{STEVE_URL}/api/tasks/{task_id}/complete",
        method="POST",
        data={"completed_by": completed_by}
    )

def fail_task(task_id: int) -> dict:
    """Mark a task as failed."""
    return make_request(
        f"{STEVE_URL}/api/tasks/{task_id}/status",
        method="PUT",
        data={"status": "failed"}
    )

def format_task_prompt(task: dict) -> str:
    """Format a task as a Claude Code prompt."""
    return f"""
================================================================================
HOLMOS AUTO-HEALER TASK
================================================================================

## Task #{task['id']}: {task['title']}

**Type:** {task['task_type']}
**Priority:** {task['priority']} (1=critical, 10=low)
**Service:** {task.get('affected_service', 'N/A')}
**Reported by:** {task['reported_by']}
**Timestamp:** {task['timestamp']}

### Description:
{task['description']}

### Instructions:
1. Investigate this issue in the HolmOS codebase
2. Identify the root cause
3. Implement a fix
4. Test the fix if possible
5. When done, run: python /Users/tim/HolmOS/services/ai-bots/task_processor.py --complete {task['id']}

### Quick Commands:
- Complete: `python task_processor.py --complete {task['id']}`
- Fail: `python task_processor.py --fail {task['id']}`
- Stats: `python task_processor.py --stats`

================================================================================
"""

def print_tasks(tasks_response: dict, as_json: bool = False):
    """Print tasks in human-readable or JSON format."""
    if as_json:
        print(json.dumps(tasks_response, indent=2))
        return

    if "error" in tasks_response:
        print(f"Error: {tasks_response['error']}", file=sys.stderr)
        return

    tasks = tasks_response.get("tasks", [])
    if not tasks:
        print("No pending tasks!")
        return

    print(f"\n{'='*60}")
    print(f"  HOLMOS TASK QUEUE - {len(tasks)} pending tasks")
    print(f"{'='*60}\n")

    priority_labels = {
        1: "CRITICAL", 2: "HIGH", 3: "HIGH",
        4: "MEDIUM", 5: "MEDIUM", 6: "MEDIUM",
        7: "LOW", 8: "LOW", 9: "LOW", 10: "LOWEST"
    }

    for task in tasks:
        priority = task.get('priority', 5)
        label = priority_labels.get(priority, "MEDIUM")
        print(f"[#{task['id']}] [{label}] {task['title']}")
        print(f"       Type: {task['task_type']} | Service: {task.get('affected_service', 'N/A')}")
        desc = task.get('description', '')[:100]
        print(f"       {desc}...")
        print()

def watch_mode(interval: int = 30, auto_claim: bool = False):
    """Watch for new tasks and output them as prompts."""
    print(f"🔍 Watching for tasks (polling every {interval}s)...", file=sys.stderr)
    print("Press Ctrl+C to stop\n", file=sys.stderr)

    processed_tasks = set()

    while True:
        try:
            stats = get_stats()
            pending = stats.get("pending", 0)

            if pending > 0:
                if auto_claim:
                    # Auto-claim mode: grab the next task
                    result = claim_next_task()
                    if result.get("success") and result.get("task"):
                        task = result["task"]
                        if task["id"] not in processed_tasks:
                            processed_tasks.add(task["id"])
                            print(f"\n🆕 New task claimed: #{task['id']}", file=sys.stderr)
                            print(format_task_prompt(task))
                else:
                    # Just notify about pending tasks
                    tasks = get_tasks(limit=5)
                    for task in tasks.get("tasks", []):
                        if task["id"] not in processed_tasks:
                            processed_tasks.add(task["id"])
                            print(f"\n📋 Pending: #{task['id']} - {task['title']}", file=sys.stderr)

            time.sleep(interval)

        except KeyboardInterrupt:
            print("\n\nStopped watching.", file=sys.stderr)
            break

def main():
    parser = argparse.ArgumentParser(
        description="HolmOS Task Processor - Claude Code Integration",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s                          List pending tasks
  %(prog)s --next                   Claim and print next task as prompt
  %(prog)s --watch                  Watch for new tasks
  %(prog)s --complete 5             Mark task #5 as complete
  %(prog)s --stats                  Show task queue statistics
  %(prog)s --next | claude          Pipe task directly to Claude Code
        """
    )

    parser.add_argument("--json", action="store_true",
                       help="Output as JSON")
    parser.add_argument("--watch", action="store_true",
                       help="Watch mode - poll for tasks continuously")
    parser.add_argument("--interval", type=int, default=30,
                       help="Poll interval in seconds (default: 30)")
    parser.add_argument("--auto-claim", action="store_true",
                       help="Auto-claim tasks in watch mode")
    parser.add_argument("--next", action="store_true",
                       help="Claim next task and output as prompt")
    parser.add_argument("--complete", type=int, metavar="ID",
                       help="Mark task as complete")
    parser.add_argument("--fail", type=int, metavar="ID",
                       help="Mark task as failed")
    parser.add_argument("--stats", action="store_true",
                       help="Show task queue statistics")
    parser.add_argument("--status", default="pending",
                       help="Filter by status (default: pending)")
    parser.add_argument("--limit", type=int, default=20,
                       help="Max tasks to fetch (default: 20)")

    args = parser.parse_args()

    # Handle specific commands
    if args.complete:
        result = complete_task(args.complete)
        if result.get("success"):
            print(f"✅ Task #{args.complete} marked as complete!")
        else:
            print(f"❌ Error: {result.get('error', 'Unknown error')}", file=sys.stderr)
        return

    if args.fail:
        result = fail_task(args.fail)
        if result.get("success"):
            print(f"❌ Task #{args.fail} marked as failed")
        else:
            print(f"Error: {result.get('error', 'Unknown error')}", file=sys.stderr)
        return

    if args.stats:
        stats = get_stats()
        if "error" in stats:
            print(f"Error: {stats['error']}", file=sys.stderr)
            return

        print("\n📊 Task Queue Statistics:")
        print(f"   Total:       {stats.get('total', 0)}")
        print(f"   Pending:     {stats.get('pending', 0)}")
        print(f"   In Progress: {stats.get('in_progress', 0)}")
        print(f"   Completed:   {stats.get('completed', 0)}")
        print(f"   Failed:      {stats.get('failed', 0)}")
        return

    if args.next:
        result = claim_next_task()
        if result.get("success") and result.get("task"):
            task = result["task"]
            if args.json:
                print(json.dumps(result, indent=2))
            else:
                print(format_task_prompt(task))
        else:
            print("No pending tasks available.", file=sys.stderr)
        return

    if args.watch:
        watch_mode(args.interval, args.auto_claim)
        return

    # Default: list tasks
    result = get_tasks(status=args.status, limit=args.limit)
    print_tasks(result, as_json=args.json)


if __name__ == "__main__":
    main()
