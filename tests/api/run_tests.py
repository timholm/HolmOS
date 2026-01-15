#!/usr/bin/env python3
"""
HolmOS API Test Runner

This script runs all API tests and generates a comprehensive report.

Usage:
    python run_tests.py                     # Run all tests
    python run_tests.py --service nova      # Run tests for specific service
    python run_tests.py --html report.html  # Generate HTML report
    python run_tests.py --json report.json  # Generate JSON report
    python run_tests.py --quick             # Run only health checks
"""

import argparse
import json
import os
import sys
import subprocess
from datetime import datetime
from pathlib import Path


# Service port mapping from services.yaml
SERVICES = {
    # Core Entry Points
    "holmos-shell": {"port": 30000, "category": "core", "description": "iPhone-style home screen"},
    "claude-pod": {"port": 30001, "category": "core", "description": "AI chat interface"},
    "app-store": {"port": 30002, "category": "core", "description": "AI-powered app generator"},
    "chat-hub": {"port": 30003, "category": "core", "description": "Unified agent messaging"},

    # AI Agents
    "nova": {"port": 30004, "category": "agent", "description": "Cluster guardian"},
    "merchant": {"port": 30005, "category": "agent", "description": "Request handler"},
    "pulse": {"port": 30006, "category": "agent", "description": "Health monitoring"},
    "gateway": {"port": 30008, "category": "agent", "description": "Routing"},
    "scribe": {"port": 30860, "category": "agent", "description": "Records keeper"},
    "vault": {"port": 30870, "category": "agent", "description": "Secret manager"},

    # Apps
    "clock-app": {"port": 30007, "category": "app", "description": "World clock, alarms, timer"},
    "calculator-app": {"port": 30010, "category": "app", "description": "Calculator"},
    "file-web-nautilus": {"port": 30088, "category": "app", "description": "File manager"},
    "settings-web": {"port": 30600, "category": "app", "description": "Settings hub"},
    "audiobook-web": {"port": 30700, "category": "app", "description": "Audiobook TTS"},
    "terminal-web": {"port": 30800, "category": "app", "description": "Web terminal"},

    # Infrastructure
    "holm-git": {"port": 30009, "category": "devops", "description": "Git server"},
    "cicd-controller": {"port": 30020, "category": "devops", "description": "CI/CD pipeline"},
    "deploy-controller": {"port": 30021, "category": "devops", "description": "Auto-deployment"},

    # Admin & Monitoring
    "cluster-manager": {"port": 30502, "category": "admin", "description": "Cluster admin"},
    "backup-dashboard": {"port": 30850, "category": "admin", "description": "Backup management"},
    "test-dashboard": {"port": 30900, "category": "monitoring", "description": "Service health"},
    "metrics-dashboard": {"port": 30950, "category": "monitoring", "description": "Cluster metrics"},
    "registry-ui": {"port": 31750, "category": "devops", "description": "Registry browser"},
}


def get_test_dir():
    """Get the directory containing test files."""
    return Path(__file__).parent


def run_pytest(args: list, capture_output: bool = False) -> subprocess.CompletedProcess:
    """Run pytest with the given arguments."""
    cmd = [sys.executable, "-m", "pytest"] + args
    return subprocess.run(cmd, capture_output=capture_output, text=True, cwd=get_test_dir())


def run_all_tests(verbose: bool = False, html_report: str = None, json_report: str = None):
    """Run all API tests."""
    args = ["."]

    if verbose:
        args.append("-v")
    else:
        args.append("-q")

    args.extend(["--tb=short", "-x"])  # Stop on first failure

    if html_report:
        args.extend(["--html", html_report, "--self-contained-html"])

    if json_report:
        args.extend(["--json-report", f"--json-report-file={json_report}"])

    return run_pytest(args)


def run_service_tests(service: str, verbose: bool = False):
    """Run tests for a specific service."""
    test_file = get_test_dir() / f"test_{service.replace('-', '_')}.py"

    if not test_file.exists():
        print(f"No test file found for service: {service}")
        print(f"Expected: {test_file}")
        return None

    args = [str(test_file)]
    if verbose:
        args.append("-v")

    return run_pytest(args)


def run_health_checks(host: str = None):
    """Run quick health checks for all services."""
    import requests

    host = host or os.environ.get("HOLMOS_HOST", "192.168.8.197")
    results = {}

    print(f"\nHolmOS Health Check - {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"Target Host: {host}")
    print("=" * 60)

    for service, config in SERVICES.items():
        port = config["port"]
        url = f"http://{host}:{port}/health"

        try:
            response = requests.get(url, timeout=5)
            if response.status_code == 200:
                status = "HEALTHY"
                status_symbol = "[+]"
            else:
                status = f"HTTP {response.status_code}"
                status_symbol = "[!]"
        except requests.exceptions.Timeout:
            status = "TIMEOUT"
            status_symbol = "[x]"
        except requests.exceptions.ConnectionError:
            status = "UNREACHABLE"
            status_symbol = "[x]"
        except Exception as e:
            status = f"ERROR: {str(e)[:30]}"
            status_symbol = "[x]"

        results[service] = {
            "port": port,
            "status": status,
            "category": config["category"],
            "description": config["description"]
        }

        print(f"{status_symbol} {service:25} (:{port}) - {status}")

    # Summary
    healthy = sum(1 for r in results.values() if r["status"] == "HEALTHY")
    total = len(results)

    print("=" * 60)
    print(f"Summary: {healthy}/{total} services healthy")

    # Category breakdown
    categories = {}
    for service, data in results.items():
        cat = data["category"]
        if cat not in categories:
            categories[cat] = {"healthy": 0, "total": 0}
        categories[cat]["total"] += 1
        if data["status"] == "HEALTHY":
            categories[cat]["healthy"] += 1

    print("\nBy Category:")
    for cat, counts in sorted(categories.items()):
        print(f"  {cat:15} {counts['healthy']}/{counts['total']}")

    return results


def generate_html_report(results: dict, output_file: str):
    """Generate a standalone HTML report."""
    html = f"""<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>HolmOS API Test Report</title>
    <style>
        :root {{
            --base: #1e1e2e;
            --surface0: #313244;
            --text: #cdd6f4;
            --green: #a6e3a1;
            --red: #f38ba8;
            --yellow: #f9e2af;
            --blue: #89b4fa;
            --mauve: #cba6f7;
        }}
        * {{ margin: 0; padding: 0; box-sizing: border-box; }}
        body {{
            font-family: 'Segoe UI', system-ui, sans-serif;
            background: var(--base);
            color: var(--text);
            padding: 2rem;
        }}
        h1 {{
            color: var(--mauve);
            margin-bottom: 0.5rem;
        }}
        .timestamp {{
            color: #7f849c;
            margin-bottom: 2rem;
        }}
        .summary {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
            gap: 1rem;
            margin-bottom: 2rem;
        }}
        .stat {{
            background: var(--surface0);
            padding: 1rem;
            border-radius: 8px;
            text-align: center;
        }}
        .stat-value {{
            font-size: 2rem;
            font-weight: bold;
        }}
        .stat-value.healthy {{ color: var(--green); }}
        .stat-value.unhealthy {{ color: var(--red); }}
        .stat-label {{
            font-size: 0.8rem;
            color: #7f849c;
        }}
        table {{
            width: 100%;
            border-collapse: collapse;
            background: var(--surface0);
            border-radius: 8px;
            overflow: hidden;
        }}
        th, td {{
            padding: 0.75rem 1rem;
            text-align: left;
            border-bottom: 1px solid #45475a;
        }}
        th {{
            background: #45475a;
            font-weight: 600;
        }}
        tr:hover {{
            background: #3b3d52;
        }}
        .status-healthy {{
            color: var(--green);
            font-weight: bold;
        }}
        .status-unhealthy {{
            color: var(--red);
            font-weight: bold;
        }}
        .category {{
            padding: 0.25rem 0.5rem;
            border-radius: 4px;
            font-size: 0.8rem;
            background: var(--blue);
            color: var(--base);
        }}
        .category-core {{ background: var(--mauve); }}
        .category-agent {{ background: var(--green); }}
        .category-app {{ background: var(--blue); }}
        .category-devops {{ background: #fab387; }}
        .category-admin {{ background: #f9e2af; color: var(--base); }}
        .category-monitoring {{ background: #94e2d5; color: var(--base); }}
    </style>
</head>
<body>
    <h1>HolmOS API Test Report</h1>
    <p class="timestamp">Generated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}</p>

    <div class="summary">
        <div class="stat">
            <div class="stat-value healthy">{sum(1 for r in results.values() if r['status'] == 'HEALTHY')}</div>
            <div class="stat-label">Healthy Services</div>
        </div>
        <div class="stat">
            <div class="stat-value unhealthy">{sum(1 for r in results.values() if r['status'] != 'HEALTHY')}</div>
            <div class="stat-label">Unhealthy Services</div>
        </div>
        <div class="stat">
            <div class="stat-value">{len(results)}</div>
            <div class="stat-label">Total Services</div>
        </div>
    </div>

    <table>
        <thead>
            <tr>
                <th>Service</th>
                <th>Port</th>
                <th>Category</th>
                <th>Status</th>
                <th>Description</th>
            </tr>
        </thead>
        <tbody>
"""

    for service, data in sorted(results.items()):
        status_class = "status-healthy" if data["status"] == "HEALTHY" else "status-unhealthy"
        category_class = f"category-{data['category']}"

        html += f"""
            <tr>
                <td><strong>{service}</strong></td>
                <td>{data['port']}</td>
                <td><span class="category {category_class}">{data['category']}</span></td>
                <td class="{status_class}">{data['status']}</td>
                <td>{data['description']}</td>
            </tr>
"""

    html += """
        </tbody>
    </table>
</body>
</html>
"""

    with open(output_file, 'w') as f:
        f.write(html)

    print(f"\nHTML report generated: {output_file}")


def main():
    parser = argparse.ArgumentParser(description="HolmOS API Test Runner")
    parser.add_argument("--service", "-s", help="Run tests for a specific service")
    parser.add_argument("--verbose", "-v", action="store_true", help="Verbose output")
    parser.add_argument("--html", help="Generate HTML report to file")
    parser.add_argument("--json", help="Generate JSON report to file")
    parser.add_argument("--quick", "-q", action="store_true", help="Run quick health checks only")
    parser.add_argument("--host", help="Target host (default: $HOLMOS_HOST or 192.168.8.197)")
    parser.add_argument("--list", "-l", action="store_true", help="List all services")

    args = parser.parse_args()

    if args.list:
        print("\nHolmOS Services:")
        print("=" * 60)
        for service, config in sorted(SERVICES.items()):
            print(f"  {service:25} :{config['port']:5}  [{config['category']}]")
        return 0

    if args.quick:
        results = run_health_checks(args.host)
        if args.html:
            generate_html_report(results, args.html)
        if args.json:
            with open(args.json, 'w') as f:
                json.dump(results, f, indent=2)
            print(f"JSON report generated: {args.json}")
        return 0

    if args.service:
        result = run_service_tests(args.service, args.verbose)
        return result.returncode if result else 1
    else:
        result = run_all_tests(args.verbose, args.html, args.json)
        return result.returncode


if __name__ == "__main__":
    sys.exit(main())
