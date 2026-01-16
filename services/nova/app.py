from flask import Flask, request, jsonify, render_template_string
import subprocess
import json
import re
import os
from datetime import datetime

app = Flask(__name__)

# Nova's personality
NOVA_NAME = "Nova"
NOVA_CATCHPHRASE = "I see all 13 stars in our constellation."

# Catppuccin Mocha theme colors
CATPPUCCIN = {
    "base": "#1e1e2e",
    "mantle": "#181825",
    "crust": "#11111b",
    "surface0": "#313244",
    "surface1": "#45475a",
    "surface2": "#585b70",
    "overlay0": "#6c7086",
    "overlay1": "#7f849c",
    "text": "#cdd6f4",
    "subtext0": "#a6adc8",
    "subtext1": "#bac2de",
    "lavender": "#b4befe",
    "blue": "#89b4fa",
    "sapphire": "#74c7ec",
    "sky": "#89dceb",
    "teal": "#94e2d5",
    "green": "#a6e3a1",
    "yellow": "#f9e2af",
    "peach": "#fab387",
    "maroon": "#eba0ac",
    "red": "#f38ba8",
    "mauve": "#cba6f7",
    "pink": "#f5c2e7",
    "flamingo": "#f2cdcd",
    "rosewater": "#f5e0dc"
}

DASHBOARD_HTML = '''
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Nova - Cluster Guardian</title>
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
            --flamingo: "#f2cdcd";
            --rosewater: #f5e0dc;
        }

        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            background: var(--base);
            color: var(--text);
            font-family: 'Inter', sans-serif;
            min-height: 100vh;
            overflow-x: hidden;
        }

        /* Animated Star Constellation Background */
        .constellation-bg {
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            pointer-events: none;
            z-index: 0;
            overflow: hidden;
        }

        .constellation-canvas {
            position: absolute;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
        }

        .star {
            position: absolute;
            background: var(--lavender);
            border-radius: 50%;
            animation: twinkle 3s infinite ease-in-out;
            box-shadow: 0 0 10px var(--lavender), 0 0 20px var(--lavender);
        }

        .star.primary {
            background: var(--mauve);
            box-shadow: 0 0 15px var(--mauve), 0 0 30px var(--mauve), 0 0 45px var(--mauve);
            animation: twinklePrimary 4s infinite ease-in-out;
        }

        .star.secondary {
            background: var(--blue);
            box-shadow: 0 0 12px var(--blue), 0 0 24px var(--blue);
        }

        @keyframes twinkle {
            0%, 100% { opacity: 0.3; transform: scale(1); }
            50% { opacity: 1; transform: scale(1.3); }
        }

        @keyframes twinklePrimary {
            0%, 100% { opacity: 0.5; transform: scale(1); }
            50% { opacity: 1; transform: scale(1.5); }
        }

        .shooting-star {
            position: absolute;
            width: 100px;
            height: 2px;
            background: linear-gradient(90deg, var(--mauve), transparent);
            opacity: 0;
            animation: shoot 3s infinite;
        }

        @keyframes shoot {
            0% { transform: translateX(-100px) translateY(0); opacity: 0; }
            10% { opacity: 1; }
            90% { opacity: 1; }
            100% { transform: translateX(calc(100vw + 100px)) translateY(200px); opacity: 0; }
        }

        .container {
            position: relative;
            z-index: 1;
            max-width: 1900px;
            margin: 0 auto;
            padding: 15px;
        }

        header {
            text-align: center;
            padding: 20px 0;
            border-bottom: 1px solid var(--surface1);
            margin-bottom: 20px;
            background: rgba(30, 30, 46, 0.8);
            backdrop-filter: blur(10px);
            border-radius: 16px;
        }

        .logo {
            font-size: 3rem;
            font-weight: 700;
            background: linear-gradient(135deg, var(--mauve), var(--blue), var(--teal), var(--green));
            background-size: 300% 300%;
            -webkit-background-clip: text;
            -webkit-text-fill-color: transparent;
            background-clip: text;
            animation: gradientFlow 8s ease infinite;
            text-shadow: 0 0 60px rgba(203, 166, 247, 0.5);
        }

        @keyframes gradientFlow {
            0% { background-position: 0% 50%; }
            50% { background-position: 100% 50%; }
            100% { background-position: 0% 50%; }
        }

        .tagline {
            font-size: 1rem;
            color: var(--subtext0);
            font-style: italic;
            margin-top: 5px;
        }

        .status-bar {
            display: flex;
            gap: 12px;
            justify-content: center;
            margin-top: 15px;
            flex-wrap: wrap;
        }

        .status-pill {
            background: var(--surface0);
            padding: 8px 16px;
            border-radius: 25px;
            display: flex;
            align-items: center;
            gap: 8px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.85rem;
            border: 1px solid var(--surface1);
            transition: all 0.3s ease;
        }

        .status-pill:hover {
            border-color: var(--lavender);
            transform: translateY(-2px);
            box-shadow: 0 5px 20px rgba(180, 190, 254, 0.2);
        }

        .status-dot {
            width: 10px;
            height: 10px;
            border-radius: 50%;
            animation: pulse 2s infinite;
        }

        .status-dot.green { background: var(--green); box-shadow: 0 0 10px var(--green); }
        .status-dot.yellow { background: var(--yellow); box-shadow: 0 0 10px var(--yellow); }
        .status-dot.red { background: var(--red); box-shadow: 0 0 10px var(--red); }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }

        /* Grid Layout */
        .main-grid {
            display: grid;
            grid-template-columns: 1fr 300px;
            gap: 15px;
        }

        .left-panel {
            display: flex;
            flex-direction: column;
            gap: 15px;
        }

        .right-panel {
            display: flex;
            flex-direction: column;
            gap: 15px;
        }

        .card {
            background: linear-gradient(145deg, rgba(49, 50, 68, 0.9), rgba(24, 24, 37, 0.95));
            border-radius: 16px;
            padding: 15px;
            border: 1px solid var(--surface1);
            transition: all 0.3s ease;
            backdrop-filter: blur(10px);
        }

        .card:hover {
            border-color: var(--lavender);
            box-shadow: 0 8px 32px rgba(0, 0, 0, 0.3), 0 0 20px rgba(180, 190, 254, 0.1);
        }

        .card-header {
            display: flex;
            align-items: center;
            justify-content: space-between;
            margin-bottom: 12px;
            padding-bottom: 10px;
            border-bottom: 1px solid var(--surface1);
        }

        .card-title {
            font-size: 1rem;
            font-weight: 600;
            color: var(--lavender);
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .card-icon {
            font-size: 1.2rem;
        }

        /* Node Constellation Grid - 13 nodes */
        .constellation-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(130px, 1fr));
            gap: 10px;
        }

        .node-card {
            background: var(--surface0);
            border-radius: 12px;
            padding: 12px;
            text-align: center;
            border: 2px solid transparent;
            transition: all 0.3s ease;
            cursor: pointer;
            position: relative;
            overflow: hidden;
        }

        .node-card::before {
            content: '';
            position: absolute;
            top: -50%;
            left: -50%;
            width: 200%;
            height: 200%;
            background: radial-gradient(circle, rgba(203, 166, 247, 0.1) 0%, transparent 70%);
            opacity: 0;
            transition: opacity 0.3s;
        }

        .node-card:hover::before {
            opacity: 1;
        }

        .node-card:hover {
            border-color: var(--blue);
            transform: translateY(-3px);
            box-shadow: 0 10px 30px rgba(0, 0, 0, 0.3);
        }

        .node-card.control-plane {
            border-color: var(--mauve);
            background: linear-gradient(145deg, var(--surface0), rgba(203, 166, 247, 0.1));
        }

        .node-card.control-plane .node-name {
            color: var(--mauve);
        }

        .node-card.healthy {
            border-left: 3px solid var(--green);
        }

        .node-card.unhealthy {
            border-left: 3px solid var(--red);
            animation: alertPulse 2s infinite;
        }

        .node-card.unknown {
            border-left: 3px solid var(--overlay0);
            opacity: 0.7;
        }

        .node-card.unknown .node-name {
            color: var(--overlay1);
        }

        @keyframes alertPulse {
            0%, 100% { box-shadow: 0 0 0 0 rgba(243, 139, 168, 0.4); }
            50% { box-shadow: 0 0 0 10px rgba(243, 139, 168, 0); }
        }

        .node-name {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.8rem;
            font-weight: 600;
            color: var(--text);
            margin-bottom: 10px;
        }

        /* Circular Gauge */
        .gauge-container {
            display: flex;
            justify-content: center;
            gap: 8px;
            margin-bottom: 8px;
        }

        .gauge {
            position: relative;
            width: 45px;
            height: 45px;
        }

        .gauge svg {
            transform: rotate(-90deg);
            width: 45px;
            height: 45px;
        }

        .gauge-bg {
            fill: none;
            stroke: var(--surface1);
            stroke-width: 4;
        }

        .gauge-fill {
            fill: none;
            stroke-width: 4;
            stroke-linecap: round;
            transition: stroke-dashoffset 0.5s ease;
        }

        .gauge-fill.cpu {
            stroke: var(--peach);
        }

        .gauge-fill.mem {
            stroke: var(--blue);
        }

        .gauge-fill.low { stroke: var(--green); }
        .gauge-fill.medium { stroke: var(--yellow); }
        .gauge-fill.high { stroke: var(--peach); }
        .gauge-fill.critical { stroke: var(--red); }

        .gauge-text {
            position: absolute;
            top: 50%;
            left: 50%;
            transform: translate(-50%, -50%);
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.65rem;
            font-weight: 600;
        }

        .gauge-text.cpu { color: var(--peach); }
        .gauge-text.mem { color: var(--blue); }

        .gauge-label {
            font-size: 0.6rem;
            color: var(--subtext0);
            text-align: center;
            margin-top: 2px;
        }

        .node-role {
            font-size: 0.65rem;
            padding: 2px 6px;
            border-radius: 8px;
            background: var(--surface1);
            color: var(--subtext0);
            display: inline-block;
        }

        .node-role.control-plane {
            background: rgba(203, 166, 247, 0.2);
            color: var(--mauve);
        }

        /* Pod Distribution */
        .pod-distribution-visual {
            display: flex;
            flex-wrap: wrap;
            gap: 4px;
            padding: 10px;
            background: var(--mantle);
            border-radius: 10px;
            min-height: 60px;
        }

        .pod-dot {
            width: 12px;
            height: 12px;
            border-radius: 3px;
            transition: all 0.2s ease;
            cursor: pointer;
        }

        .pod-dot:hover {
            transform: scale(1.5);
            z-index: 10;
        }

        .pod-dot.Running { background: var(--green); }
        .pod-dot.Pending { background: var(--yellow); }
        .pod-dot.Failed { background: var(--red); }
        .pod-dot.Succeeded { background: var(--blue); }
        .pod-dot.Unknown { background: var(--overlay0); }

        .pod-legend {
            display: flex;
            gap: 15px;
            justify-content: center;
            margin-top: 10px;
            flex-wrap: wrap;
        }

        .pod-legend-item {
            display: flex;
            align-items: center;
            gap: 5px;
            font-size: 0.75rem;
            color: var(--subtext0);
        }

        .pod-legend-dot {
            width: 10px;
            height: 10px;
            border-radius: 3px;
        }

        /* Quick Actions */
        .quick-actions {
            display: grid;
            grid-template-columns: repeat(2, 1fr);
            gap: 8px;
        }

        .quick-action {
            background: var(--surface0);
            border: 1px solid var(--surface1);
            color: var(--text);
            padding: 12px 8px;
            border-radius: 10px;
            cursor: pointer;
            text-align: center;
            transition: all 0.3s ease;
            font-size: 0.8rem;
        }

        .quick-action:hover {
            border-color: var(--lavender);
            transform: scale(1.02);
            box-shadow: 0 5px 15px rgba(0, 0, 0, 0.2);
        }

        .quick-action-icon {
            font-size: 1.3rem;
            margin-bottom: 5px;
            display: block;
        }

        .quick-action.scale:hover { border-color: var(--blue); background: rgba(137, 180, 250, 0.1); }
        .quick-action.restart:hover { border-color: var(--peach); background: rgba(250, 179, 135, 0.1); }
        .quick-action.logs:hover { border-color: var(--green); background: rgba(166, 227, 161, 0.1); }
        .quick-action.refresh:hover { border-color: var(--mauve); background: rgba(203, 166, 247, 0.1); }

        /* Cluster Metrics */
        .cluster-metrics {
            display: flex;
            flex-direction: column;
            gap: 10px;
        }

        .metric-row {
            display: flex;
            align-items: center;
            gap: 10px;
        }

        .metric-label {
            min-width: 60px;
            color: var(--subtext0);
            font-size: 0.8rem;
        }

        .metric-bar {
            flex: 1;
            height: 20px;
            background: var(--mantle);
            border-radius: 10px;
            overflow: hidden;
            position: relative;
        }

        .metric-fill {
            height: 100%;
            border-radius: 10px;
            transition: width 0.5s ease;
            display: flex;
            align-items: center;
            justify-content: flex-end;
            padding-right: 8px;
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.7rem;
            font-weight: 600;
            color: var(--crust);
            min-width: 35px;
        }

        .metric-fill.cpu { background: linear-gradient(90deg, var(--peach), var(--yellow)); }
        .metric-fill.mem { background: linear-gradient(90deg, var(--blue), var(--sapphire)); }

        /* Deployment Management */
        .deployment-controls {
            display: flex;
            gap: 8px;
            margin-bottom: 10px;
            flex-wrap: wrap;
        }

        .namespace-btn {
            background: var(--surface0);
            border: 1px solid var(--surface1);
            color: var(--subtext0);
            padding: 5px 12px;
            border-radius: 15px;
            cursor: pointer;
            font-size: 0.75rem;
            transition: all 0.2s ease;
        }

        .namespace-btn:hover, .namespace-btn.active {
            background: var(--mauve);
            color: var(--crust);
            border-color: var(--mauve);
        }

        .deployment-list {
            max-height: 350px;
            overflow-y: auto;
        }

        .deployment-list::-webkit-scrollbar {
            width: 6px;
        }

        .deployment-list::-webkit-scrollbar-track {
            background: var(--surface0);
            border-radius: 3px;
        }

        .deployment-list::-webkit-scrollbar-thumb {
            background: var(--surface2);
            border-radius: 3px;
        }

        .deployment-item {
            display: flex;
            align-items: center;
            justify-content: space-between;
            padding: 10px;
            background: var(--surface0);
            border-radius: 8px;
            margin-bottom: 6px;
            border-left: 3px solid var(--surface2);
            transition: all 0.2s ease;
        }

        .deployment-item:hover {
            background: var(--surface1);
        }

        .deployment-item.healthy { border-left-color: var(--green); }
        .deployment-item.warning { border-left-color: var(--yellow); }
        .deployment-item.error { border-left-color: var(--red); }

        .deployment-info {
            flex: 1;
            min-width: 0;
        }

        .deployment-name {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.8rem;
            color: var(--text);
            white-space: nowrap;
            overflow: hidden;
            text-overflow: ellipsis;
        }

        .deployment-namespace {
            font-size: 0.65rem;
            color: var(--subtext0);
        }

        .deployment-replicas {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.75rem;
            padding: 3px 8px;
            background: var(--mantle);
            border-radius: 10px;
            margin-right: 8px;
        }

        .deployment-replicas.full { color: var(--green); }
        .deployment-replicas.partial { color: var(--yellow); }
        .deployment-replicas.zero { color: var(--red); }

        .action-btns {
            display: flex;
            gap: 4px;
        }

        .action-btn {
            background: var(--surface1);
            border: none;
            color: var(--subtext0);
            padding: 4px 8px;
            border-radius: 5px;
            cursor: pointer;
            font-size: 0.7rem;
            transition: all 0.2s ease;
        }

        .action-btn:hover {
            background: var(--surface2);
            color: var(--text);
        }

        .action-btn.restart:hover { background: var(--peach); color: var(--crust); }
        .action-btn.scale:hover { background: var(--blue); color: var(--crust); }
        .action-btn.logs:hover { background: var(--green); color: var(--crust); }

        /* Node Distribution Bar Chart */
        .node-bar-chart {
            display: flex;
            flex-direction: column;
            gap: 6px;
        }

        .node-bar-item {
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .node-bar-label {
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.7rem;
            color: var(--subtext0);
            min-width: 70px;
        }

        .node-bar {
            flex: 1;
            height: 16px;
            background: var(--mantle);
            border-radius: 8px;
            overflow: hidden;
        }

        .node-bar-fill {
            height: 100%;
            background: linear-gradient(90deg, var(--mauve), var(--blue));
            border-radius: 8px;
            transition: width 0.5s ease;
            display: flex;
            align-items: center;
            justify-content: flex-end;
            padding-right: 6px;
            font-size: 0.65rem;
            font-weight: 600;
            color: var(--crust);
        }

        /* Modal Styles */
        .modal {
            display: none;
            position: fixed;
            top: 0;
            left: 0;
            width: 100%;
            height: 100%;
            background: rgba(0, 0, 0, 0.8);
            z-index: 1000;
            align-items: center;
            justify-content: center;
            backdrop-filter: blur(5px);
        }

        .modal.active { display: flex; }

        .modal-content {
            background: var(--surface0);
            border-radius: 16px;
            padding: 20px;
            max-width: 500px;
            width: 90%;
            border: 1px solid var(--surface1);
            box-shadow: 0 20px 60px rgba(0, 0, 0, 0.5);
        }

        .modal-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 15px;
            padding-bottom: 10px;
            border-bottom: 1px solid var(--surface1);
        }

        .modal-title {
            font-size: 1.1rem;
            color: var(--lavender);
        }

        .modal-close {
            background: none;
            border: none;
            color: var(--subtext0);
            font-size: 1.5rem;
            cursor: pointer;
            transition: color 0.2s;
        }

        .modal-close:hover {
            color: var(--red);
        }

        .form-group {
            margin-bottom: 12px;
        }

        .form-label {
            display: block;
            margin-bottom: 6px;
            color: var(--subtext1);
            font-size: 0.85rem;
        }

        .form-input, .form-select {
            width: 100%;
            padding: 10px;
            background: var(--mantle);
            border: 1px solid var(--surface1);
            border-radius: 8px;
            color: var(--text);
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.9rem;
        }

        .form-input:focus, .form-select:focus {
            outline: none;
            border-color: var(--lavender);
        }

        .form-btn {
            width: 100%;
            padding: 12px;
            background: linear-gradient(135deg, var(--mauve), var(--blue));
            border: none;
            border-radius: 8px;
            color: var(--crust);
            font-weight: 600;
            cursor: pointer;
            transition: all 0.3s ease;
        }

        .form-btn:hover {
            transform: translateY(-2px);
            box-shadow: 0 5px 20px rgba(203, 166, 247, 0.3);
        }

        /* Logs container */
        .logs-container {
            background: var(--mantle);
            padding: 12px;
            border-radius: 8px;
            max-height: 300px;
            overflow-y: auto;
            font-family: 'JetBrains Mono', monospace;
            font-size: 0.75rem;
            color: var(--text);
            white-space: pre-wrap;
            word-break: break-all;
        }

        /* Toast Notification */
        .toast {
            position: fixed;
            bottom: 20px;
            right: 20px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            padding: 12px 20px;
            border-radius: 10px;
            display: none;
            z-index: 1001;
            animation: slideIn 0.3s ease;
            font-size: 0.9rem;
        }

        .toast.success { border-left: 4px solid var(--green); }
        .toast.error { border-left: 4px solid var(--red); }
        .toast.info { border-left: 4px solid var(--blue); }
        .toast.show { display: block; }

        @keyframes slideIn {
            from { transform: translateX(100px); opacity: 0; }
            to { transform: translateX(0); opacity: 1; }
        }

        /* Refresh button */
        .refresh-btn {
            position: fixed;
            bottom: 20px;
            left: 20px;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            color: var(--text);
            padding: 12px;
            border-radius: 50%;
            cursor: pointer;
            z-index: 100;
            transition: all 0.3s ease;
            font-size: 1.2rem;
        }

        .refresh-btn:hover {
            background: var(--mauve);
            color: var(--crust);
            transform: rotate(180deg);
        }

        .timestamp {
            text-align: center;
            color: var(--subtext0);
            font-size: 0.75rem;
            margin-top: 15px;
            font-family: 'JetBrains Mono', monospace;
        }

        /* Responsive */
        @media (max-width: 1200px) {
            .main-grid {
                grid-template-columns: 1fr;
            }
            .right-panel {
                flex-direction: row;
                flex-wrap: wrap;
            }
            .right-panel .card {
                flex: 1;
                min-width: 280px;
            }
        }

        @media (max-width: 768px) {
            .constellation-grid {
                grid-template-columns: repeat(3, 1fr);
            }
            .quick-actions {
                grid-template-columns: repeat(2, 1fr);
            }
        }

        /* Pod tooltip */
        .pod-tooltip {
            position: fixed;
            background: var(--surface0);
            border: 1px solid var(--surface1);
            padding: 8px 12px;
            border-radius: 8px;
            font-size: 0.75rem;
            z-index: 1000;
            pointer-events: none;
            display: none;
        }

        .pod-tooltip.show {
            display: block;
        }
    </style>
</head>
<body>
    <div class="constellation-bg">
        <canvas id="constellationCanvas" class="constellation-canvas"></canvas>
    </div>

    <div class="container">
        <header>
            <div class="logo">Nova</div>
            <div class="tagline">I see all 13 stars in our constellation.</div>
            <div class="status-bar" id="statusBar"></div>
        </header>

        <div class="main-grid">
            <div class="left-panel">
                <!-- 13 Node Constellation -->
                <div class="card">
                    <div class="card-header">
                        <span class="card-title"><span class="card-icon">&#11088;</span> Node Constellation</span>
                        <span id="nodeCount">0/13 Stars</span>
                    </div>
                    <div class="constellation-grid" id="nodeGrid"></div>
                </div>

                <!-- Pod Distribution Visualization -->
                <div class="card">
                    <div class="card-header">
                        <span class="card-title"><span class="card-icon">&#128230;</span> Pod Distribution</span>
                        <span id="totalPods">0 pods</span>
                    </div>
                    <div class="pod-distribution-visual" id="podDistVisual"></div>
                    <div class="pod-legend">
                        <div class="pod-legend-item"><div class="pod-legend-dot Running"></div> Running</div>
                        <div class="pod-legend-item"><div class="pod-legend-dot Pending"></div> Pending</div>
                        <div class="pod-legend-item"><div class="pod-legend-dot Failed"></div> Failed</div>
                        <div class="pod-legend-item"><div class="pod-legend-dot Succeeded"></div> Succeeded</div>
                    </div>
                    <div class="node-bar-chart" id="nodeBarChart" style="margin-top: 15px;"></div>
                </div>

                <!-- Deployment Management Panel -->
                <div class="card">
                    <div class="card-header">
                        <span class="card-title"><span class="card-icon">&#128640;</span> Deployment Management</span>
                        <span id="deploymentCount">0 deployments</span>
                    </div>
                    <div class="deployment-controls" id="namespaceFilter">
                        <button class="namespace-btn active" data-ns="all">All</button>
                    </div>
                    <div class="deployment-list" id="deploymentList"></div>
                </div>
            </div>

            <div class="right-panel">
                <!-- Cluster Health -->
                <div class="card">
                    <div class="card-header">
                        <span class="card-title"><span class="card-icon">&#127775;</span> Cluster Health</span>
                        <span id="clusterHealth"></span>
                    </div>
                    <div class="cluster-metrics">
                        <div class="metric-row">
                            <span class="metric-label">CPU</span>
                            <div class="metric-bar">
                                <div class="metric-fill cpu" id="cpuBar" style="width: 5%">0%</div>
                            </div>
                        </div>
                        <div class="metric-row">
                            <span class="metric-label">Memory</span>
                            <div class="metric-bar">
                                <div class="metric-fill mem" id="memBar" style="width: 5%">0%</div>
                            </div>
                        </div>
                    </div>
                </div>

                <!-- Quick Actions -->
                <div class="card">
                    <div class="card-header">
                        <span class="card-title"><span class="card-icon">&#9889;</span> Quick Actions</span>
                    </div>
                    <div class="quick-actions">
                        <button class="quick-action scale" onclick="showScaleModal()">
                            <span class="quick-action-icon">&#128200;</span>
                            Scale
                        </button>
                        <button class="quick-action restart" onclick="showRestartModal()">
                            <span class="quick-action-icon">&#128260;</span>
                            Restart
                        </button>
                        <button class="quick-action logs" onclick="showLogsModal()">
                            <span class="quick-action-icon">&#128196;</span>
                            Logs
                        </button>
                        <button class="quick-action refresh" onclick="refreshDashboard()">
                            <span class="quick-action-icon">&#128259;</span>
                            Refresh
                        </button>
                    </div>
                </div>

                <!-- Top Resource Users -->
                <div class="card">
                    <div class="card-header">
                        <span class="card-title"><span class="card-icon">&#128202;</span> Top Resources</span>
                    </div>
                    <div id="topResources"></div>
                </div>
            </div>
        </div>

        <div class="timestamp" id="timestamp">Last updated: --</div>
    </div>

    <!-- Scale Modal -->
    <div class="modal" id="scaleModal">
        <div class="modal-content">
            <div class="modal-header">
                <span class="modal-title">&#128200; Scale Deployment</span>
                <button class="modal-close" onclick="closeModal('scaleModal')">&times;</button>
            </div>
            <div class="form-group">
                <label class="form-label">Deployment</label>
                <select class="form-select" id="scaleDeployment"></select>
            </div>
            <div class="form-group">
                <label class="form-label">Namespace</label>
                <input type="text" class="form-input" id="scaleNamespace" value="holm">
            </div>
            <div class="form-group">
                <label class="form-label">Replicas</label>
                <input type="number" class="form-input" id="scaleReplicas" min="0" max="20" value="1">
            </div>
            <button class="form-btn" onclick="scaleDeployment()">Scale Deployment</button>
        </div>
    </div>

    <!-- Restart Modal -->
    <div class="modal" id="restartModal">
        <div class="modal-content">
            <div class="modal-header">
                <span class="modal-title">&#128260; Restart Deployment</span>
                <button class="modal-close" onclick="closeModal('restartModal')">&times;</button>
            </div>
            <div class="form-group">
                <label class="form-label">Deployment</label>
                <select class="form-select" id="restartDeployment"></select>
            </div>
            <div class="form-group">
                <label class="form-label">Namespace</label>
                <input type="text" class="form-input" id="restartNamespace" value="holm">
            </div>
            <button class="form-btn" onclick="restartDeployment()">Restart Deployment</button>
        </div>
    </div>

    <!-- Logs Modal -->
    <div class="modal" id="logsModal">
        <div class="modal-content" style="max-width: 700px;">
            <div class="modal-header">
                <span class="modal-title">&#128196; Pod Logs</span>
                <button class="modal-close" onclick="closeModal('logsModal')">&times;</button>
            </div>
            <div class="form-group">
                <label class="form-label">Pod</label>
                <select class="form-select" id="logsPod"></select>
            </div>
            <div class="form-group">
                <label class="form-label">Namespace</label>
                <input type="text" class="form-input" id="logsNamespace" value="holm">
            </div>
            <div class="form-group">
                <label class="form-label">Tail Lines</label>
                <input type="number" class="form-input" id="logsTail" value="100" min="10" max="500">
            </div>
            <button class="form-btn" onclick="fetchLogs()" style="margin-bottom: 12px;">Fetch Logs</button>
            <div class="logs-container" id="logsContent">Logs will appear here...</div>
        </div>
    </div>

    <!-- Node Detail Modal -->
    <div class="modal" id="nodeModal">
        <div class="modal-content">
            <div class="modal-header">
                <span class="modal-title">&#11088; Node Details</span>
                <button class="modal-close" onclick="closeModal('nodeModal')">&times;</button>
            </div>
            <div id="nodeDetails"></div>
        </div>
    </div>

    <div class="toast" id="toast"></div>
    <div class="pod-tooltip" id="podTooltip"></div>
    <button class="refresh-btn" onclick="refreshDashboard()">&#128260;</button>

    <script>
        // Animated Star Constellation Background
        const canvas = document.getElementById('constellationCanvas');
        const ctx = canvas.getContext('2d');

        let stars = [];
        let constellationStars = [];
        const numStars = 150;
        const numConstellationStars = 13;

        function resizeCanvas() {
            canvas.width = window.innerWidth;
            canvas.height = window.innerHeight;
            initStars();
        }

        function initStars() {
            stars = [];
            constellationStars = [];

            // Background stars
            for (let i = 0; i < numStars; i++) {
                stars.push({
                    x: Math.random() * canvas.width,
                    y: Math.random() * canvas.height,
                    radius: Math.random() * 1.5 + 0.5,
                    alpha: Math.random(),
                    speed: Math.random() * 0.02 + 0.01,
                    direction: Math.random() > 0.5 ? 1 : -1
                });
            }

            // Constellation stars (13 nodes)
            const constellationPositions = [
                { x: 0.15, y: 0.2 },   // rpi-1 (control plane - center top)
                { x: 0.08, y: 0.35 },  // rpi-2
                { x: 0.22, y: 0.35 },  // rpi-3
                { x: 0.05, y: 0.5 },   // rpi-4
                { x: 0.18, y: 0.5 },   // rpi-5
                { x: 0.12, y: 0.65 },  // rpi-6
                { x: 0.25, y: 0.65 },  // rpi-7
                { x: 0.08, y: 0.8 },   // rpi-8
                { x: 0.2, y: 0.8 },    // rpi-9
                { x: 0.85, y: 0.25 },  // rpi-10
                { x: 0.9, y: 0.4 },    // rpi-11
                { x: 0.82, y: 0.55 },  // rpi-12
                { x: 0.75, y: 0.7 }    // openmediavault
            ];

            constellationPositions.forEach((pos, i) => {
                constellationStars.push({
                    x: pos.x * canvas.width,
                    y: pos.y * canvas.height,
                    radius: i === 0 ? 4 : 3,
                    alpha: 1,
                    pulse: Math.random() * Math.PI * 2,
                    isControlPlane: i === 0
                });
            });
        }

        // Constellation connections
        const connections = [
            [0, 1], [0, 2], [1, 3], [2, 4], [3, 5], [4, 6], [5, 7], [6, 8], [5, 6],
            [9, 10], [10, 11], [11, 12]
        ];

        function drawStars() {
            ctx.clearRect(0, 0, canvas.width, canvas.height);

            // Draw background stars
            stars.forEach(star => {
                star.alpha += star.speed * star.direction;
                if (star.alpha <= 0.1 || star.alpha >= 1) {
                    star.direction *= -1;
                }

                ctx.beginPath();
                ctx.arc(star.x, star.y, star.radius, 0, Math.PI * 2);
                ctx.fillStyle = `rgba(180, 190, 254, ${star.alpha * 0.5})`;
                ctx.fill();
            });

            // Draw constellation connections
            ctx.strokeStyle = 'rgba(203, 166, 247, 0.2)';
            ctx.lineWidth = 1;
            connections.forEach(([i, j]) => {
                if (constellationStars[i] && constellationStars[j]) {
                    ctx.beginPath();
                    ctx.moveTo(constellationStars[i].x, constellationStars[i].y);
                    ctx.lineTo(constellationStars[j].x, constellationStars[j].y);
                    ctx.stroke();
                }
            });

            // Draw constellation stars
            constellationStars.forEach((star, i) => {
                star.pulse += 0.03;
                const pulseAlpha = 0.5 + Math.sin(star.pulse) * 0.3;

                // Glow
                const gradient = ctx.createRadialGradient(
                    star.x, star.y, 0,
                    star.x, star.y, star.radius * 8
                );

                if (star.isControlPlane) {
                    gradient.addColorStop(0, `rgba(203, 166, 247, ${pulseAlpha})`);
                    gradient.addColorStop(1, 'rgba(203, 166, 247, 0)');
                } else {
                    gradient.addColorStop(0, `rgba(137, 180, 250, ${pulseAlpha * 0.8})`);
                    gradient.addColorStop(1, 'rgba(137, 180, 250, 0)');
                }

                ctx.beginPath();
                ctx.arc(star.x, star.y, star.radius * 8, 0, Math.PI * 2);
                ctx.fillStyle = gradient;
                ctx.fill();

                // Core
                ctx.beginPath();
                ctx.arc(star.x, star.y, star.radius, 0, Math.PI * 2);
                ctx.fillStyle = star.isControlPlane ? '#cba6f7' : '#89b4fa';
                ctx.fill();
            });

            requestAnimationFrame(drawStars);
        }

        window.addEventListener('resize', resizeCanvas);
        resizeCanvas();
        drawStars();

        // Dashboard Data
        let clusterData = {};
        let currentNamespace = 'all';

        function showToast(message, type = 'success') {
            const toast = document.getElementById('toast');
            toast.textContent = message;
            toast.className = 'toast show ' + type;
            setTimeout(() => toast.className = 'toast', 3000);
        }

        function showModal(id) {
            document.getElementById(id).classList.add('active');
        }

        function closeModal(id) {
            document.getElementById(id).classList.remove('active');
        }

        function showScaleModal() {
            populateDeploymentSelect('scaleDeployment');
            showModal('scaleModal');
        }

        function showRestartModal() {
            populateDeploymentSelect('restartDeployment');
            showModal('restartModal');
        }

        function showLogsModal() {
            populatePodSelect('logsPod');
            showModal('logsModal');
        }

        function populateDeploymentSelect(selectId) {
            const select = document.getElementById(selectId);
            const deployments = clusterData.deployments || [];
            select.innerHTML = deployments.map(d =>
                `<option value="${d.name}" data-ns="${d.namespace}">${d.namespace}/${d.name}</option>`
            ).join('');

            select.addEventListener('change', function() {
                const selected = this.options[this.selectedIndex];
                const nsInput = document.getElementById(selectId.replace('Deployment', 'Namespace'));
                if (nsInput && selected) {
                    nsInput.value = selected.dataset.ns;
                }
            });

            // Trigger initial change
            if (select.options.length > 0) {
                select.dispatchEvent(new Event('change'));
            }
        }

        function populatePodSelect(selectId) {
            const select = document.getElementById(selectId);
            const pods = (clusterData.pods || []).filter(p => p.status === 'Running');
            select.innerHTML = pods.map(p =>
                `<option value="${p.name}" data-ns="${p.namespace}">${p.namespace}/${p.name}</option>`
            ).join('');

            select.addEventListener('change', function() {
                const selected = this.options[this.selectedIndex];
                const nsInput = document.getElementById('logsNamespace');
                if (nsInput && selected) {
                    nsInput.value = selected.dataset.ns;
                }
            });

            if (select.options.length > 0) {
                select.dispatchEvent(new Event('change'));
            }
        }

        function createGauge(value, type) {
            const circumference = 2 * Math.PI * 17;
            const offset = circumference - (value / 100) * circumference;
            const colorClass = value < 50 ? 'low' : value < 70 ? 'medium' : value < 90 ? 'high' : 'critical';

            return `
                <div class="gauge">
                    <svg viewBox="0 0 40 40">
                        <circle class="gauge-bg" cx="20" cy="20" r="17"/>
                        <circle class="gauge-fill ${type} ${colorClass}" cx="20" cy="20" r="17"
                            stroke-dasharray="${circumference}"
                            stroke-dashoffset="${offset}"/>
                    </svg>
                    <span class="gauge-text ${type}">${value}%</span>
                </div>
            `;
        }

        async function fetchDashboardData() {
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), 10000);  // 10 second timeout

            try {
                const response = await fetch('/api/dashboard', { signal: controller.signal });
                clearTimeout(timeoutId);
                clusterData = await response.json();
                updateDashboard();
            } catch (error) {
                clearTimeout(timeoutId);
                if (error.name === 'AbortError') {
                    showToast('Request timed out after 10s - showing cached data', 'error');
                } else {
                    showToast('Failed to fetch cluster data', 'error');
                }
                // Still update dashboard with whatever data we have (including hardcoded nodes)
                if (!clusterData.nodes || clusterData.nodes.length === 0) {
                    clusterData = { nodes: [], pods: [], deployments: [], metrics: { cpu_avg: 0, mem_avg: 0 }, error: true };
                }
                updateDashboard();
            }
        }

        function updateDashboard() {
            const nodes = clusterData.nodes || [];
            const pods = clusterData.pods || [];
            const deployments = clusterData.deployments || [];

            const nodesHealthy = nodes.filter(n => n.status === 'Ready').length;
            const nodesUnknown = nodes.filter(n => n.status === 'Unknown').length;
            const totalNodes = nodes.length;
            const podsRunning = pods.filter(p => p.status === 'Running').length;
            const totalPods = pods.length;

            // Status bar - show unknown nodes with different styling
            let nodeStatusColor = 'green';
            let nodeStatusText = `${nodesHealthy}/${totalNodes} Nodes`;
            if (nodesHealthy < totalNodes && nodesUnknown > 0) {
                nodeStatusColor = 'yellow';
                nodeStatusText = `${nodesHealthy} Ready, ${nodesUnknown} Unknown / ${totalNodes}`;
            } else if (nodesHealthy < totalNodes) {
                nodeStatusColor = 'yellow';
            }

            document.getElementById('statusBar').innerHTML = `
                <div class="status-pill">
                    <span class="status-dot ${nodeStatusColor}"></span>
                    <span>${nodeStatusText}</span>
                </div>
                <div class="status-pill">
                    <span class="status-dot ${podsRunning === totalPods ? 'green' : 'yellow'}"></span>
                    <span>${podsRunning}/${totalPods} Pods</span>
                </div>
                <div class="status-pill">
                    <span class="status-dot green"></span>
                    <span>${deployments.length} Deployments</span>
                </div>
            `;

            // Cluster health
            let healthStatus;
            if (nodesHealthy === totalNodes) {
                healthStatus = '<span style="color: var(--green); font-size: 1.2rem;">&#10003; Healthy</span>';
            } else if (nodesUnknown === totalNodes) {
                healthStatus = '<span style="color: var(--overlay0); font-size: 1.2rem;">&#8230; Connecting</span>';
            } else if (nodesUnknown > 0) {
                healthStatus = '<span style="color: var(--yellow); font-size: 1.2rem;">&#9888; Partial</span>';
            } else {
                healthStatus = '<span style="color: var(--yellow); font-size: 1.2rem;">&#9888; Degraded</span>';
            }
            document.getElementById('clusterHealth').innerHTML = healthStatus;

            // CPU/Memory bars
            const cpuAvg = clusterData.metrics?.cpu_avg || 0;
            const memAvg = clusterData.metrics?.mem_avg || 0;
            document.getElementById('cpuBar').style.width = Math.max(cpuAvg, 5) + '%';
            document.getElementById('cpuBar').textContent = cpuAvg + '%';
            document.getElementById('memBar').style.width = Math.max(memAvg, 5) + '%';
            document.getElementById('memBar').textContent = memAvg + '%';

            // Node grid with gauges
            const nodeGrid = document.getElementById('nodeGrid');
            nodeGrid.innerHTML = nodes.map(node => {
                const isControlPlane = node.roles?.includes('control-plane');
                let statusClass;
                if (node.status === 'Ready') {
                    statusClass = 'healthy';
                } else if (node.status === 'Unknown') {
                    statusClass = 'unknown';
                } else {
                    statusClass = 'unhealthy';
                }

                return `
                    <div class="node-card ${statusClass} ${isControlPlane ? 'control-plane' : ''}"
                         onclick="showNodeDetails('${node.name}')">
                        <div class="node-name">${node.name}</div>
                        <div class="gauge-container">
                            ${createGauge(node.cpu || 0, 'cpu')}
                            ${createGauge(node.mem || 0, 'mem')}
                        </div>
                        <div style="display: flex; justify-content: center; gap: 15px; margin-bottom: 8px;">
                            <span class="gauge-label">CPU</span>
                            <span class="gauge-label">MEM</span>
                        </div>
                        <span class="node-role ${isControlPlane ? 'control-plane' : ''}">${node.roles || 'worker'}</span>
                        ${node.status === 'Unknown' ? '<div style="font-size: 0.6rem; color: var(--overlay0); margin-top: 4px;">Not in K8s</div>' : ''}
                    </div>
                `;
            }).join('');

            document.getElementById('nodeCount').textContent = `${nodesHealthy}/${totalNodes} Stars`;

            // Pod distribution visualization
            const podDistVisual = document.getElementById('podDistVisual');
            podDistVisual.innerHTML = pods.slice(0, 200).map(pod => {
                return `<div class="pod-dot ${pod.status}"
                    data-name="${pod.name}"
                    data-ns="${pod.namespace}"
                    data-node="${pod.node}"
                    data-status="${pod.status}"
                    onmouseover="showPodTooltip(event, this)"
                    onmouseout="hidePodTooltip()"></div>`;
            }).join('');

            document.getElementById('totalPods').textContent = `${totalPods} pods`;

            // Node bar chart
            const podsByNode = {};
            pods.forEach(pod => {
                const node = pod.node || 'Unknown';
                podsByNode[node] = (podsByNode[node] || 0) + 1;
            });

            const maxPods = Math.max(...Object.values(podsByNode), 1);
            const nodeBarChart = document.getElementById('nodeBarChart');
            nodeBarChart.innerHTML = Object.entries(podsByNode)
                .sort((a, b) => b[1] - a[1])
                .slice(0, 8)
                .map(([node, count]) => `
                    <div class="node-bar-item">
                        <span class="node-bar-label">${node}</span>
                        <div class="node-bar">
                            <div class="node-bar-fill" style="width: ${(count / maxPods) * 100}%">${count}</div>
                        </div>
                    </div>
                `).join('');

            // Namespace filter
            const namespaces = [...new Set(deployments.map(d => d.namespace))].sort();
            document.getElementById('namespaceFilter').innerHTML = `
                <button class="namespace-btn ${currentNamespace === 'all' ? 'active' : ''}"
                    onclick="filterNamespace('all')">All</button>
                ${namespaces.map(ns => `
                    <button class="namespace-btn ${currentNamespace === ns ? 'active' : ''}"
                        onclick="filterNamespace('${ns}')">${ns}</button>
                `).join('')}
            `;

            // Deployments
            updateDeploymentList();
            document.getElementById('deploymentCount').textContent = `${deployments.length} deployments`;

            // Top resources
            const topRes = document.getElementById('topResources');
            topRes.innerHTML = (clusterData.top_pods || []).slice(0, 5).map(pod => `
                <div class="deployment-item healthy">
                    <div class="deployment-info">
                        <div class="deployment-name">${pod.name}</div>
                        <div class="deployment-namespace">${pod.namespace}</div>
                    </div>
                    <span class="deployment-replicas" style="color: var(--peach);">${pod.cpu}</span>
                    <span class="deployment-replicas" style="color: var(--blue);">${pod.memory}</span>
                </div>
            `).join('');

            // Timestamp
            document.getElementById('timestamp').textContent = `Last updated: ${new Date().toLocaleTimeString()}`;
        }

        function filterNamespace(ns) {
            currentNamespace = ns;
            updateDeploymentList();
            document.querySelectorAll('.namespace-btn').forEach(btn => {
                btn.classList.toggle('active', btn.textContent === ns || (ns === 'all' && btn.textContent === 'All'));
            });
        }

        function updateDeploymentList() {
            const deployList = document.getElementById('deploymentList');
            const filtered = (clusterData.deployments || []).filter(d =>
                currentNamespace === 'all' || d.namespace === currentNamespace
            );

            deployList.innerHTML = filtered.map(deploy => {
                const ready = deploy.ready || 0;
                const replicas = deploy.replicas || 0;
                const status = ready === replicas ? 'healthy' : ready === 0 ? 'error' : 'warning';
                const replicaClass = ready === replicas ? 'full' : ready === 0 ? 'zero' : 'partial';

                return `
                    <div class="deployment-item ${status}">
                        <div class="deployment-info">
                            <div class="deployment-name">${deploy.name}</div>
                            <div class="deployment-namespace">${deploy.namespace}</div>
                        </div>
                        <span class="deployment-replicas ${replicaClass}">${ready}/${replicas}</span>
                        <div class="action-btns">
                            <button class="action-btn scale" onclick="quickScale('${deploy.name}', '${deploy.namespace}')">Scale</button>
                            <button class="action-btn restart" onclick="quickRestart('${deploy.name}', '${deploy.namespace}')">Restart</button>
                            <button class="action-btn logs" onclick="quickLogs('${deploy.name}', '${deploy.namespace}')">Logs</button>
                        </div>
                    </div>
                `;
            }).join('');
        }

        function showNodeDetails(nodeName) {
            const node = (clusterData.nodes || []).find(n => n.name === nodeName);
            if (!node) return;

            const nodePods = (clusterData.pods || []).filter(p => p.node === nodeName);

            document.getElementById('nodeDetails').innerHTML = `
                <div style="margin-bottom: 15px;">
                    <div style="font-size: 1.2rem; color: var(--mauve); margin-bottom: 10px;">${node.name}</div>
                    <div style="display: flex; gap: 10px; margin-bottom: 10px;">
                        <span class="node-role ${node.roles?.includes('control-plane') ? 'control-plane' : ''}">${node.roles || 'worker'}</span>
                        <span style="color: ${node.status === 'Ready' ? 'var(--green)' : 'var(--red)'};">${node.status}</span>
                    </div>
                </div>
                <div class="cluster-metrics" style="margin-bottom: 15px;">
                    <div class="metric-row">
                        <span class="metric-label">CPU</span>
                        <div class="metric-bar">
                            <div class="metric-fill cpu" style="width: ${Math.max(node.cpu || 0, 5)}%">${node.cpu || 0}%</div>
                        </div>
                    </div>
                    <div class="metric-row">
                        <span class="metric-label">Memory</span>
                        <div class="metric-bar">
                            <div class="metric-fill mem" style="width: ${Math.max(node.mem || 0, 5)}%">${node.mem || 0}%</div>
                        </div>
                    </div>
                </div>
                <div style="font-size: 0.85rem; color: var(--subtext1); margin-bottom: 8px;">Pods on this node: ${nodePods.length}</div>
                <div style="max-height: 200px; overflow-y: auto;">
                    ${nodePods.slice(0, 20).map(p => `
                        <div style="padding: 6px 10px; background: var(--surface0); border-radius: 6px; margin-bottom: 4px; font-size: 0.75rem;">
                            <span style="color: var(--text);">${p.name}</span>
                            <span style="color: ${p.status === 'Running' ? 'var(--green)' : 'var(--yellow)'}; float: right;">${p.status}</span>
                        </div>
                    `).join('')}
                    ${nodePods.length > 20 ? `<div style="text-align: center; color: var(--subtext0); font-size: 0.75rem;">...and ${nodePods.length - 20} more</div>` : ''}
                </div>
            `;

            showModal('nodeModal');
        }

        function showPodTooltip(event, el) {
            const tooltip = document.getElementById('podTooltip');
            tooltip.innerHTML = `
                <div><strong>${el.dataset.name}</strong></div>
                <div>Namespace: ${el.dataset.ns}</div>
                <div>Node: ${el.dataset.node}</div>
                <div>Status: ${el.dataset.status}</div>
            `;
            tooltip.style.left = (event.pageX + 10) + 'px';
            tooltip.style.top = (event.pageY + 10) + 'px';
            tooltip.classList.add('show');
        }

        function hidePodTooltip() {
            document.getElementById('podTooltip').classList.remove('show');
        }

        function quickScale(name, namespace) {
            document.getElementById('scaleDeployment').value = name;
            document.getElementById('scaleNamespace').value = namespace;
            showScaleModal();
        }

        function quickRestart(name, namespace) {
            document.getElementById('restartDeployment').value = name;
            document.getElementById('restartNamespace').value = namespace;
            showRestartModal();
        }

        function quickLogs(deployName, namespace) {
            const pods = (clusterData.pods || []).filter(p =>
                p.namespace === namespace && p.name.startsWith(deployName) && p.status === 'Running'
            );
            if (pods.length > 0) {
                document.getElementById('logsPod').value = pods[0].name;
                document.getElementById('logsNamespace').value = namespace;
                showLogsModal();
            } else {
                showToast('No running pods found for this deployment', 'error');
            }
        }

        async function scaleDeployment() {
            const selectEl = document.getElementById('scaleDeployment');
            const name = selectEl.value;
            const namespace = document.getElementById('scaleNamespace').value;
            const replicas = parseInt(document.getElementById('scaleReplicas').value);

            try {
                const response = await fetch('/api/scale', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ deployment: name, namespace, replicas })
                });
                const result = await response.json();
                if (result.success) {
                    showToast(`Scaled ${name} to ${replicas} replicas`);
                    closeModal('scaleModal');
                    setTimeout(refreshDashboard, 2000);
                } else {
                    showToast(result.error || 'Scale failed', 'error');
                }
            } catch (error) {
                showToast('Scale operation failed', 'error');
            }
        }

        async function restartDeployment() {
            const selectEl = document.getElementById('restartDeployment');
            const name = selectEl.value;
            const namespace = document.getElementById('restartNamespace').value;

            try {
                const response = await fetch('/api/restart', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ deployment: name, namespace })
                });
                const result = await response.json();
                if (result.success) {
                    showToast(`Restarting ${name}`);
                    closeModal('restartModal');
                    setTimeout(refreshDashboard, 2000);
                } else {
                    showToast(result.error || 'Restart failed', 'error');
                }
            } catch (error) {
                showToast('Restart operation failed', 'error');
            }
        }

        async function fetchLogs() {
            const selectEl = document.getElementById('logsPod');
            const pod = selectEl.value;
            const namespace = document.getElementById('logsNamespace').value;
            const tail = parseInt(document.getElementById('logsTail').value);
            const logsContent = document.getElementById('logsContent');

            logsContent.textContent = 'Loading logs...';

            try {
                const response = await fetch('/api/logs', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ pod, namespace, tail })
                });
                const result = await response.json();
                logsContent.textContent = result.logs || result.error || 'No logs available';
            } catch (error) {
                logsContent.textContent = 'Failed to fetch logs';
            }
        }

        function refreshDashboard() {
            showToast('Refreshing...', 'info');
            fetchDashboardData();
        }

        // Initial load and auto-refresh
        fetchDashboardData();
        setInterval(fetchDashboardData, 15000);
    </script>
</body>
</html>
'''

# Use the kubernetes Python client for better in-cluster support
try:
    from kubernetes import client, config
    from kubernetes.client.rest import ApiException

    # Try to load in-cluster config, fall back to kubeconfig
    try:
        config.load_incluster_config()
        print("Loaded in-cluster Kubernetes config")
    except:
        config.load_kube_config()
        print("Loaded kubeconfig")

    v1 = client.CoreV1Api()
    apps_v1 = client.AppsV1Api()
    USE_K8S_CLIENT = True
except Exception as e:
    print(f"Kubernetes client not available: {e}")
    USE_K8S_CLIENT = False

# All 13 cluster nodes - hardcoded for display even if not in k8s yet
CLUSTER_NODES = [
    {"name": "rpi-1", "roles": "control-plane", "is_control_plane": True},
    {"name": "rpi-2", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-3", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-4", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-5", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-6", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-7", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-8", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-9", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-10", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-11", "roles": "worker", "is_control_plane": False},
    {"name": "rpi-12", "roles": "worker", "is_control_plane": False},
    {"name": "openmediavault", "roles": "storage", "is_control_plane": False},
]

# API timeout in seconds
API_TIMEOUT_SECONDS = 10

def get_nodes_detailed():
    """Get detailed node information with metrics for all 13 cluster nodes."""
    # Start with all known nodes marked as Unknown
    nodes_by_name = {}
    for known_node in CLUSTER_NODES:
        nodes_by_name[known_node["name"]] = {
            "name": known_node["name"],
            "status": "Unknown",
            "roles": known_node["roles"],
            "cpu": 0,
            "mem": 0
        }

    if not USE_K8S_CLIENT:
        # Return all nodes with Unknown status if k8s client unavailable
        return list(nodes_by_name.values())

    try:
        # Set a timeout for the API call
        nodes_list = v1.list_node(_request_timeout=API_TIMEOUT_SECONDS)

        # Try to get metrics
        metrics = {}
        try:
            custom_api = client.CustomObjectsApi()
            node_metrics = custom_api.list_cluster_custom_object(
                "metrics.k8s.io", "v1beta1", "nodes",
                _request_timeout=API_TIMEOUT_SECONDS
            )
            for item in node_metrics.get("items", []):
                name = item["metadata"]["name"]
                # Parse CPU usage
                cpu_str = item["usage"].get("cpu", "0")
                if cpu_str.endswith("n"):
                    cpu_val = int(cpu_str[:-1]) / 1000000000  # nanocores to cores
                elif cpu_str.endswith("m"):
                    cpu_val = int(cpu_str[:-1]) / 1000  # millicores to cores
                else:
                    cpu_val = float(cpu_str)

                # Parse memory usage
                mem_str = item["usage"].get("memory", "0")
                if mem_str.endswith("Ki"):
                    mem_val = int(mem_str[:-2]) * 1024
                elif mem_str.endswith("Mi"):
                    mem_val = int(mem_str[:-2]) * 1024 * 1024
                elif mem_str.endswith("Gi"):
                    mem_val = int(mem_str[:-2]) * 1024 * 1024 * 1024
                else:
                    mem_val = int(mem_str)

                metrics[name] = {"cpu_cores": cpu_val, "mem_bytes": mem_val}
        except Exception as e:
            print(f"Failed to get metrics: {e}")

        for node in nodes_list.items:
            name = node.metadata.name
            labels = node.metadata.labels or {}

            # Get status
            status = "NotReady"
            for condition in node.status.conditions or []:
                if condition.type == "Ready" and condition.status == "True":
                    status = "Ready"
                    break

            # Get roles
            roles = []
            for key in labels:
                if "node-role.kubernetes.io/" in key:
                    roles.append(key.split("/")[1])

            # Get allocatable resources
            allocatable = node.status.allocatable or {}
            cpu_alloc = allocatable.get("cpu", "1")
            if cpu_alloc.endswith("m"):
                cpu_alloc_val = int(cpu_alloc[:-1]) / 1000
            else:
                cpu_alloc_val = float(cpu_alloc)

            mem_alloc = allocatable.get("memory", "1Gi")
            if mem_alloc.endswith("Ki"):
                mem_alloc_val = int(mem_alloc[:-2]) * 1024
            elif mem_alloc.endswith("Mi"):
                mem_alloc_val = int(mem_alloc[:-2]) * 1024 * 1024
            elif mem_alloc.endswith("Gi"):
                mem_alloc_val = int(mem_alloc[:-2]) * 1024 * 1024 * 1024
            else:
                mem_alloc_val = int(mem_alloc)

            node_metrics = metrics.get(name, {})
            cpu_pct = int((node_metrics.get("cpu_cores", 0) / cpu_alloc_val) * 100) if cpu_alloc_val else 0
            mem_pct = int((node_metrics.get("mem_bytes", 0) / mem_alloc_val) * 100) if mem_alloc_val else 0

            # Update the node in our map (or add if it's a new node not in CLUSTER_NODES)
            nodes_by_name[name] = {
                "name": name,
                "status": status,
                "roles": ",".join(roles) if roles else "worker",
                "cpu": min(cpu_pct, 100),
                "mem": min(mem_pct, 100)
            }

        return list(nodes_by_name.values())
    except Exception as e:
        print(f"Failed to get nodes from k8s (timeout or error): {e}")
        # Return all nodes with Unknown status on error/timeout
        return list(nodes_by_name.values())

def get_pods_detailed():
    """Get detailed pod information."""
    if not USE_K8S_CLIENT:
        return []

    try:
        pods_list = v1.list_pod_for_all_namespaces(_request_timeout=API_TIMEOUT_SECONDS)
        pods = []

        for pod in pods_list.items:
            restarts = 0
            for cs in pod.status.container_statuses or []:
                restarts += cs.restart_count

            pods.append({
                "name": pod.metadata.name,
                "namespace": pod.metadata.namespace,
                "status": pod.status.phase,
                "node": pod.spec.node_name or "Unscheduled",
                "restarts": restarts
            })

        return pods
    except Exception as e:
        print(f"Failed to get pods (timeout or error): {e}")
        return []

def get_deployments_detailed():
    """Get detailed deployment information."""
    if not USE_K8S_CLIENT:
        return []

    try:
        deployments_list = apps_v1.list_deployment_for_all_namespaces(_request_timeout=API_TIMEOUT_SECONDS)
        deployments = []

        for deploy in deployments_list.items:
            deployments.append({
                "name": deploy.metadata.name,
                "namespace": deploy.metadata.namespace,
                "replicas": deploy.spec.replicas or 0,
                "ready": deploy.status.ready_replicas or 0,
                "available": deploy.status.available_replicas or 0
            })

        return deployments
    except Exception as e:
        print(f"Failed to get deployments (timeout or error): {e}")
        return []

def get_top_pods():
    """Get top resource consuming pods."""
    if not USE_K8S_CLIENT:
        return []

    try:
        custom_api = client.CustomObjectsApi()
        pod_metrics = custom_api.list_cluster_custom_object(
            "metrics.k8s.io", "v1beta1", "pods",
            _request_timeout=API_TIMEOUT_SECONDS
        )

        pods = []
        for item in pod_metrics.get("items", []):
            namespace = item["metadata"]["namespace"]
            name = item["metadata"]["name"]

            cpu_total = 0
            mem_total = 0

            for container in item.get("containers", []):
                cpu_str = container["usage"].get("cpu", "0")
                if cpu_str.endswith("n"):
                    cpu_total += int(cpu_str[:-1]) / 1000000  # nanocores to millicores
                elif cpu_str.endswith("m"):
                    cpu_total += int(cpu_str[:-1])

                mem_str = container["usage"].get("memory", "0")
                if mem_str.endswith("Ki"):
                    mem_total += int(mem_str[:-2]) / 1024  # Ki to Mi
                elif mem_str.endswith("Mi"):
                    mem_total += int(mem_str[:-2])
                elif mem_str.endswith("Gi"):
                    mem_total += int(mem_str[:-2]) * 1024

            pods.append({
                "namespace": namespace,
                "name": name,
                "cpu": f"{int(cpu_total)}m",
                "memory": f"{int(mem_total)}Mi",
                "cpu_val": cpu_total
            })

        pods.sort(key=lambda x: x["cpu_val"], reverse=True)
        return pods[:10]
    except Exception as e:
        print(f"Failed to get top pods: {e}")
        return []

def get_cluster_metrics():
    """Get overall cluster metrics."""
    nodes = get_nodes_detailed()
    if not nodes:
        return {"cpu_avg": 0, "mem_avg": 0}

    cpu_values = [n["cpu"] for n in nodes if n["cpu"] > 0]
    mem_values = [n["mem"] for n in nodes if n["mem"] > 0]

    return {
        "cpu_avg": sum(cpu_values) // len(cpu_values) if cpu_values else 0,
        "mem_avg": sum(mem_values) // len(mem_values) if mem_values else 0
    }

def scale_deployment_k8s(deployment, namespace, replicas):
    """Scale a deployment using k8s client."""
    if not USE_K8S_CLIENT:
        return {"success": False, "error": "Kubernetes client not available"}

    try:
        body = {"spec": {"replicas": replicas}}
        apps_v1.patch_namespaced_deployment_scale(deployment, namespace, body)
        return {"success": True, "message": f"Scaled {deployment} to {replicas}"}
    except ApiException as e:
        return {"success": False, "error": str(e)}

def restart_deployment_k8s(deployment, namespace):
    """Restart a deployment using k8s client."""
    if not USE_K8S_CLIENT:
        return {"success": False, "error": "Kubernetes client not available"}

    try:
        now = datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")
        body = {
            "spec": {
                "template": {
                    "metadata": {
                        "annotations": {
                            "kubectl.kubernetes.io/restartedAt": now
                        }
                    }
                }
            }
        }
        apps_v1.patch_namespaced_deployment(deployment, namespace, body)
        return {"success": True, "message": f"Restarted {deployment}"}
    except ApiException as e:
        return {"success": False, "error": str(e)}

def get_pod_logs(pod, namespace, tail=100):
    """Get pod logs using k8s client."""
    if not USE_K8S_CLIENT:
        return {"logs": None, "error": "Kubernetes client not available"}

    try:
        logs = v1.read_namespaced_pod_log(pod, namespace, tail_lines=tail)
        return {"logs": logs}
    except ApiException as e:
        return {"logs": None, "error": str(e)}

@app.route("/")
def dashboard():
    """Serve the main dashboard."""
    return render_template_string(DASHBOARD_HTML)

@app.route("/api/dashboard")
def api_dashboard():
    """Get all dashboard data."""
    return jsonify({
        "nodes": get_nodes_detailed(),
        "pods": get_pods_detailed(),
        "deployments": get_deployments_detailed(),
        "top_pods": get_top_pods(),
        "metrics": get_cluster_metrics(),
        "timestamp": datetime.now().isoformat()
    })

@app.route("/api/scale", methods=["POST"])
def api_scale():
    """Scale a deployment."""
    data = request.get_json()
    deployment = data.get("deployment")
    namespace = data.get("namespace", "holm")
    replicas = data.get("replicas", 1)

    result = scale_deployment_k8s(deployment, namespace, replicas)
    return jsonify(result)

@app.route("/api/restart", methods=["POST"])
def api_restart():
    """Restart a deployment."""
    data = request.get_json()
    deployment = data.get("deployment")
    namespace = data.get("namespace", "holm")

    result = restart_deployment_k8s(deployment, namespace)
    return jsonify(result)

@app.route("/api/logs", methods=["POST"])
def api_logs():
    """Get pod logs."""
    data = request.get_json()
    pod = data.get("pod")
    namespace = data.get("namespace", "holm")
    tail = data.get("tail", 100)

    result = get_pod_logs(pod, namespace, tail)
    return jsonify(result)

@app.route("/health", methods=["GET"])
def health():
    """Health check endpoint."""
    return jsonify({"status": "healthy", "agent": NOVA_NAME})

@app.route("/capabilities", methods=["GET"])
def capabilities():
    """List Nova's capabilities."""
    return jsonify({
        "agent": NOVA_NAME,
        "catchphrase": NOVA_CATCHPHRASE,
        "version": "3.0.0",
        "features": [
            "Real-time cluster dashboard",
            "13-node constellation view with animated background",
            "Circular CPU/Memory gauges per node",
            "Pod distribution visualization",
            "Pod status dot matrix",
            "Resource usage monitoring",
            "Deployment management with scale/restart/logs",
            "Namespace filtering",
            "Node detail modal",
            "Live log streaming",
            "Auto-refresh every 15 seconds"
        ]
    })

# Keep the chat endpoint for API compatibility
@app.route("/chat", methods=["POST"])
def chat():
    """Chat endpoint for interacting with Nova."""
    data = request.get_json()
    if not data or "message" not in data:
        return jsonify({"error": "Missing 'message' in request body"}), 400

    message = data["message"].lower()

    if "status" in message or "health" in message or "how" in message:
        nodes = get_nodes_detailed()
        healthy = sum(1 for n in nodes if n["status"] == "Ready")
        total = len(nodes)
        metrics = get_cluster_metrics()

        response = f"""{NOVA_CATCHPHRASE}

Cluster Status:
- Nodes: {healthy}/{total} healthy
- CPU: {metrics['cpu_avg']}% average
- Memory: {metrics['mem_avg']}% average

Visit the dashboard at / for the full constellation view!"""
    else:
        response = f"""I'm Nova, your cluster guardian. {NOVA_CATCHPHRASE}

Check out the visual dashboard at / for:
- Real-time node constellation (13 stars)
- Circular CPU/Memory gauges per node
- Pod distribution across nodes
- Resource usage monitoring
- Deployment management with quick actions

Or ask me about: status, nodes, pods, deployments"""

    return jsonify({
        "response": response,
        "agent": NOVA_NAME
    })

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 80))
    app.run(host="0.0.0.0", port=port)
