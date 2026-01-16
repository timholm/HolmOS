#!/usr/bin/env python3
"""HolmOS iOS Shell - iPhone SpringBoard-style interface for HolmOS."""

import os
import json
from flask import Flask, jsonify, request, Response, redirect

app = Flask(__name__)

CLUSTER_HOST = os.environ.get('CLUSTER_HOST', '192.168.8.197')

# App Registry - defines all apps, pages, dock, and folders
APP_REGISTRY = {
    "version": "1.0.0",
    "theme": "catppuccin-mocha",
    "pages": [
        {
            "id": "page1",
            "apps": [
                {"id": "calculator", "name": "Calculator", "icon": "calc", "port": 30010, "color": "#333333"},
                {"id": "clock", "name": "Clock", "icon": "clock", "port": 30011, "color": "#1c1c1e"},
                {"id": "audiobook", "name": "Audiobook", "icon": "headphones", "port": 30700, "color": "#fc3c44"},
                {"id": "terminal", "name": "Terminal", "icon": "terminal", "port": 30800, "color": "#1e1e2e"},
                {"id": "vault", "name": "Vault", "icon": "lock", "port": 30870, "color": "#8e8e93"},
                {"id": "scribe", "name": "Scribe", "icon": "pencil", "port": 30860, "color": "#ffd60a"},
                {"id": "backup", "name": "Backup", "icon": "cloud", "port": 30850, "color": "#5ac8fa"},
                {"id": "nova", "name": "Nova", "icon": "sparkles", "port": 30004, "color": "#cba6f7"},
                {"id": "metrics", "name": "Metrics", "icon": "chart", "port": 30950, "color": "#f38ba8"},
                {"id": "registry", "name": "Registry", "icon": "box", "port": 31750, "color": "#89b4fa"},
                {"id": "tests", "name": "Tests", "icon": "check", "port": 30900, "color": "#a6e3a1"},
                {"id": "auth", "name": "Auth", "icon": "key", "port": 30100, "color": "#fab387"}
            ]
        },
        {
            "id": "page2",
            "apps": [
                {"id": "git", "name": "HolmGit", "icon": "git", "port": 30500, "color": "#f05033"},
                {"id": "cicd", "name": "CI/CD", "icon": "gear", "port": 30020, "color": "#45475a"},
                {"id": "deploy", "name": "Deploy", "icon": "rocket", "port": 30021, "color": "#89b4fa"},
                {"id": "cluster", "name": "Cluster", "icon": "server", "port": 30502, "color": "#cba6f7"}
            ]
        }
    ],
    "dock": [
        {"id": "chat", "name": "Chat", "icon": "message", "port": 30003, "color": "#a6e3a1"},
        {"id": "store", "name": "Store", "icon": "bag", "port": 30002, "color": "#89b4fa"},
        {"id": "settings", "name": "Settings", "icon": "settings", "port": 30600, "color": "#6c7086"},
        {"id": "files", "name": "Files", "icon": "folder", "port": 30088, "color": "#89b4fa"}
    ]
}

# SVG Icons
ICONS = {
    "calc": '<path d="M4 2a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V4a2 2 0 0 0-2-2H4zm2 4h12v3H6V6zm0 5h3v3H6v-3zm5 0h3v3h-3v-3zm5 0h3v8h-3v-8zm-10 5h3v3H6v-3zm5 0h3v3h-3v-3z"/>',
    "clock": '<circle cx="12" cy="12" r="10" fill="none" stroke="currentColor" stroke-width="2"/><path d="M12 6v6l4 2" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/>',
    "headphones": '<path d="M3 18v-6a9 9 0 0 1 18 0v6"/><path d="M21 19a2 2 0 0 1-2 2h-1a2 2 0 0 1-2-2v-3a2 2 0 0 1 2-2h3zM3 19a2 2 0 0 0 2 2h1a2 2 0 0 0 2-2v-3a2 2 0 0 0-2-2H3z"/>',
    "terminal": '<rect x="2" y="4" width="20" height="16" rx="2"/><path d="m6 10 4 2-4 2m6 0h4"/>',
    "lock": '<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M7 11V7a5 5 0 0 1 10 0v4"/>',
    "pencil": '<path d="M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4Z"/>',
    "cloud": '<path d="M17.5 19H9a7 7 0 1 1 6.71-9h1.79a4.5 4.5 0 1 1 0 9Z"/>',
    "sparkles": '<path d="m12 3-1.9 5.8a2 2 0 0 1-1.3 1.3L3 12l5.8 1.9a2 2 0 0 1 1.3 1.3L12 21l1.9-5.8a2 2 0 0 1 1.3-1.3L21 12l-5.8-1.9a2 2 0 0 1-1.3-1.3Z"/>',
    "chart": '<path d="M3 3v18h18"/><path d="m19 9-5 5-4-4-3 3"/>',
    "box": '<path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"/><path d="m3.3 7 8.7 5 8.7-5M12 22V12"/>',
    "check": '<path d="M20 6 9 17l-5-5"/>',
    "key": '<path d="m21 2-2 2m-7.61 7.61a5.5 5.5 0 1 1-7.78 7.78 5.5 5.5 0 0 1 7.78-7.78Zm0 0L15.5 7.5m0 0 3 3L22 7l-3-3m-3.5 3.5L19 4"/>',
    "git": '<circle cx="12" cy="18" r="3"/><circle cx="6" cy="6" r="3"/><circle cx="18" cy="6" r="3"/><path d="M18 9v2c0 .6-.4 1-1 1H7c-.6 0-1-.4-1-1V9M12 12v3"/>',
    "gear": '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
    "rocket": '<path d="M4.5 16.5c-1.5 1.26-2 5-2 5s3.74-.5 5-2c.71-.84.7-2.13-.09-2.91a2.18 2.18 0 0 0-2.91-.09z"/><path d="m12 15-3-3a22 22 0 0 1 2-3.95A12.88 12.88 0 0 1 22 2c0 2.72-.78 7.5-6 11a22.35 22.35 0 0 1-4 2z"/><path d="M9 12H4s.55-3.03 2-4c1.62-1.08 5 0 5 0"/><path d="M12 15v5s3.03-.55 4-2c1.08-1.62 0-5 0-5"/>',
    "server": '<rect width="20" height="8" x="2" y="2" rx="2" ry="2"/><rect width="20" height="8" x="2" y="14" rx="2" ry="2"/><line x1="6" x2="6.01" y1="6" y2="6"/><line x1="6" x2="6.01" y1="18" y2="18"/>',
    "message": '<path d="M7.9 20A9 9 0 1 0 4 16.1L2 22Z"/>',
    "bag": '<path d="M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4Z"/><path d="M3 6h18"/><path d="M16 10a4 4 0 0 1-8 0"/>',
    "settings": '<path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>',
    "folder": '<path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.6 3.9A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/>'
}

INDEX_HTML = '''<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0, user-scalable=no, viewport-fit=cover, maximum-scale=1">
    <meta name="apple-mobile-web-app-capable" content="yes">
    <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
    <meta name="theme-color" content="#1e1e2e">
    <title>HolmOS</title>
    <link rel="manifest" href="/manifest.json">
    <link rel="apple-touch-icon" href="/icon-192.png">
    <style>
        :root {
            --base: #1e1e2e; --mantle: #181825; --crust: #11111b;
            --text: #cdd6f4; --subtext0: #a6adc8; --subtext1: #bac2de;
            --surface0: #313244; --surface1: #45475a; --surface2: #585b70;
            --overlay0: #6c7086; --blue: #89b4fa; --mauve: #cba6f7;
            --green: #a6e3a1; --red: #f38ba8; --peach: #fab387;
            --safe-top: env(safe-area-inset-top, 20px);
            --safe-bottom: env(safe-area-inset-bottom, 20px);
        }
        * { margin: 0; padding: 0; box-sizing: border-box; -webkit-tap-highlight-color: transparent; touch-action: pan-y; }
        html, body { height: 100%; overflow: hidden; overscroll-behavior: none; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, "SF Pro Display", sans-serif;
            background: var(--base); color: var(--text);
            display: flex; flex-direction: column;
            padding-top: var(--safe-top); padding-bottom: var(--safe-bottom);
        }

        /* Pages Container */
        .pages-wrapper {
            flex: 1; display: flex; overflow: hidden;
            padding-top: var(--safe-top); padding-bottom: 120px;
        }
        .pages {
            display: flex; transition: transform 0.3s cubic-bezier(0.25, 0.1, 0.25, 1);
            width: 100%; height: 100%;
        }
        .page {
            min-width: 100%; height: 100%; padding: 20px;
            display: grid; grid-template-columns: repeat(4, 1fr);
            gap: 24px 16px; align-content: start;
        }

        /* App Icons */
        .app {
            display: flex; flex-direction: column; align-items: center; gap: 6px;
            cursor: pointer; transition: transform 0.15s;
        }
        .app:active { transform: scale(0.9); }
        .app.jiggle { animation: jiggle 0.15s infinite alternate; }
        @keyframes jiggle { 0% { transform: rotate(-2deg); } 100% { transform: rotate(2deg); } }
        .app-icon {
            width: 60px; height: 60px; border-radius: 14px;
            display: flex; align-items: center; justify-content: center;
            box-shadow: 0 4px 12px rgba(0,0,0,0.3);
            position: relative; overflow: hidden;
        }
        .app-icon::after {
            content: ""; position: absolute; top: 0; left: 0; right: 0; height: 50%;
            background: linear-gradient(180deg, rgba(255,255,255,0.25) 0%, transparent 100%);
            border-radius: 14px 14px 0 0;
        }
        .app-icon svg { width: 32px; height: 32px; fill: white; stroke: white; stroke-width: 1.5; z-index: 1; }
        .app-name {
            font-size: 11px; font-weight: 500; text-align: center;
            text-shadow: 0 1px 3px rgba(0,0,0,0.8);
            max-width: 70px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
        }

        /* Page Dots */
        .page-dots {
            display: flex; justify-content: center; gap: 8px;
            padding: 12px; position: fixed; bottom: 100px; left: 0; right: 0;
        }
        .dot {
            width: 8px; height: 8px; border-radius: 50%;
            background: var(--surface2); transition: all 0.2s;
        }
        .dot.active { background: var(--text); transform: scale(1.2); }

        /* Dock */
        .dock {
            position: fixed; bottom: 16px; left: 50%; transform: translateX(-50%);
            background: rgba(30,30,46,0.7); backdrop-filter: blur(20px);
            border-radius: 28px; padding: 12px 20px;
            display: flex; gap: 20px;
            border: 1px solid rgba(255,255,255,0.1);
            box-shadow: 0 8px 32px rgba(0,0,0,0.4);
            margin-bottom: var(--safe-bottom);
        }
        .dock .app { gap: 0; }
        .dock .app-icon { width: 52px; height: 52px; border-radius: 12px; }
        .dock .app-icon svg { width: 28px; height: 28px; }
        .dock .app-name { display: none; }

        /* Spotlight Search */
        .spotlight {
            position: fixed; top: 0; left: 0; right: 0; bottom: 0;
            background: rgba(0,0,0,0.85); backdrop-filter: blur(20px);
            z-index: 200; display: none; flex-direction: column;
            padding: 60px 20px 20px; padding-top: calc(var(--safe-top) + 60px);
        }
        .spotlight.active { display: flex; }
        .spotlight-input {
            width: 100%; padding: 12px 16px; border-radius: 12px;
            background: var(--surface0); border: none; color: var(--text);
            font-size: 16px; outline: none;
        }
        .spotlight-results {
            margin-top: 16px; flex: 1; overflow-y: auto;
        }
        .spotlight-result {
            display: flex; align-items: center; gap: 12px;
            padding: 12px; border-radius: 12px; cursor: pointer;
        }
        .spotlight-result:hover { background: var(--surface0); }
        .spotlight-result .app-icon { width: 44px; height: 44px; border-radius: 10px; }
        .spotlight-result .app-icon svg { width: 24px; height: 24px; }

        /* App Container */
        .app-container {
            position: fixed; top: 0; left: 0; right: 0; bottom: 0;
            background: var(--base); z-index: 300;
            transform: translateY(100%); transition: transform 0.3s cubic-bezier(0.25, 0.1, 0.25, 1);
        }
        .app-container.active { transform: translateY(0); }
        .app-header {
            display: flex; align-items: center; justify-content: space-between;
            padding: 12px 16px; background: var(--mantle);
            border-bottom: 1px solid var(--surface0);
            padding-top: calc(var(--safe-top) + 12px);
        }
        .app-title { font-size: 16px; font-weight: 600; }
        .app-close {
            width: 32px; height: 32px; border-radius: 50%;
            background: var(--surface0); border: none; color: var(--text);
            font-size: 20px; cursor: pointer; display: flex;
            align-items: center; justify-content: center;
        }
        .app-frame { flex: 1; width: 100%; height: calc(100% - 56px - var(--safe-top)); border: none; }

        /* Home Indicator */
        .home-indicator {
            position: fixed; bottom: 8px; left: 50%; transform: translateX(-50%);
            width: 134px; height: 5px; background: var(--text);
            border-radius: 3px; opacity: 0.3;
        }
        .app-container .home-indicator { opacity: 0.5; }

        @media (min-width: 768px) {
            .page { grid-template-columns: repeat(6, 1fr); max-width: 600px; margin: 0 auto; }
            .dock { gap: 24px; padding: 14px 24px; }
        }
    </style>
</head>
<body>
    <div class="pages-wrapper" id="pagesWrapper">
        <div class="pages" id="pages"></div>
    </div>

    <div class="page-dots" id="pageDots"></div>

    <div class="dock" id="dock"></div>

    <div class="spotlight" id="spotlight">
        <input type="text" class="spotlight-input" placeholder="Search apps..." id="searchInput" autocomplete="off">
        <div class="spotlight-results" id="searchResults"></div>
    </div>

    <div class="app-container" id="appContainer">
        <div class="app-header">
            <span class="app-title" id="appTitle">App</span>
            <button class="app-close" onclick="closeApp()">&#10005;</button>
        </div>
        <iframe class="app-frame" id="appFrame"></iframe>
        <div class="home-indicator"></div>
    </div>

    <div class="home-indicator"></div>

    <script>
        const HOST = "''' + CLUSTER_HOST + '''";
        const registry = ''' + json.dumps(APP_REGISTRY) + ''';
        const icons = ''' + json.dumps(ICONS) + ''';

        let currentPage = 0;
        let touchStartX = 0;
        let touchStartY = 0;
        let isDragging = false;
        let isJiggling = false;
        let longPressTimer = null;

        function createIcon(iconName) {
            return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor">${icons[iconName] || icons.sparkles}</svg>`;
        }

        function createApp(app, inDock = false) {
            return `
                <div class="app ${isJiggling ? 'jiggle' : ''}" data-id="${app.id}" data-port="${app.port}"
                     onclick="launchApp('${app.id}', '${app.name}', ${app.port})"
                     ontouchstart="handleTouchStart(event, '${app.id}')"
                     ontouchend="handleTouchEnd(event)">
                    <div class="app-icon" style="background: linear-gradient(135deg, ${app.color}, ${adjustColor(app.color, -30)})">
                        ${createIcon(app.icon)}
                    </div>
                    ${!inDock ? `<span class="app-name">${app.name}</span>` : ''}
                </div>
            `;
        }

        function adjustColor(hex, amount) {
            const num = parseInt(hex.slice(1), 16);
            const r = Math.max(0, Math.min(255, (num >> 16) + amount));
            const g = Math.max(0, Math.min(255, ((num >> 8) & 0xff) + amount));
            const b = Math.max(0, Math.min(255, (num & 0xff) + amount));
            return `#${(r << 16 | g << 8 | b).toString(16).padStart(6, '0')}`;
        }

        function render() {
            // Render pages
            document.getElementById('pages').innerHTML = registry.pages.map(page =>
                `<div class="page">${page.apps.map(app => createApp(app)).join('')}</div>`
            ).join('');

            // Render dock
            document.getElementById('dock').innerHTML = registry.dock.map(app => createApp(app, true)).join('');

            // Render page dots
            document.getElementById('pageDots').innerHTML = registry.pages.map((_, i) =>
                `<div class="dot ${i === currentPage ? 'active' : ''}" onclick="goToPage(${i})"></div>`
            ).join('');

            updatePagePosition();
        }

        function goToPage(index) {
            currentPage = Math.max(0, Math.min(registry.pages.length - 1, index));
            updatePagePosition();
            document.querySelectorAll('.dot').forEach((dot, i) => {
                dot.classList.toggle('active', i === currentPage);
            });
        }

        function updatePagePosition() {
            document.getElementById('pages').style.transform = `translateX(-${currentPage * 100}%)`;
        }

        function launchApp(id, name, port) {
            if (isJiggling) return;
            if (!port) { alert(name + ' coming soon!'); return; }

            document.getElementById('appTitle').textContent = name;
            document.getElementById('appFrame').src = `http://${HOST}:${port}`;
            document.getElementById('appContainer').classList.add('active');
        }

        function closeApp() {
            document.getElementById('appContainer').classList.remove('active');
            setTimeout(() => { document.getElementById('appFrame').src = ''; }, 300);
        }

        // Touch handling for swipe
        const wrapper = document.getElementById('pagesWrapper');
        wrapper.addEventListener('touchstart', e => {
            touchStartX = e.touches[0].clientX;
            touchStartY = e.touches[0].clientY;
        });

        wrapper.addEventListener('touchmove', e => {
            if (Math.abs(e.touches[0].clientY - touchStartY) > 50) return; // Vertical scroll
            const diff = touchStartX - e.touches[0].clientX;
            if (Math.abs(diff) > 20) isDragging = true;
        });

        wrapper.addEventListener('touchend', e => {
            if (!isDragging) return;
            const diff = touchStartX - e.changedTouches[0].clientX;
            if (diff > 50) goToPage(currentPage + 1);
            else if (diff < -50) goToPage(currentPage - 1);
            isDragging = false;
        });

        // Long press for jiggle mode
        function handleTouchStart(e, appId) {
            longPressTimer = setTimeout(() => {
                isJiggling = !isJiggling;
                render();
            }, 500);
        }

        function handleTouchEnd(e) {
            clearTimeout(longPressTimer);
        }

        // Pull down for spotlight
        let pullStartY = 0;
        document.addEventListener('touchstart', e => { pullStartY = e.touches[0].clientY; });
        document.addEventListener('touchend', e => {
            const pullDiff = e.changedTouches[0].clientY - pullStartY;
            if (pullDiff > 100 && pullStartY < 100) {
                document.getElementById('spotlight').classList.add('active');
                document.getElementById('searchInput').focus();
            }
        });

        // Spotlight search
        document.getElementById('searchInput').addEventListener('input', e => {
            const query = e.target.value.toLowerCase();
            const allApps = [...registry.pages.flatMap(p => p.apps), ...registry.dock];
            const results = allApps.filter(app => app.name.toLowerCase().includes(query));

            document.getElementById('searchResults').innerHTML = results.map(app => `
                <div class="spotlight-result" onclick="launchApp('${app.id}', '${app.name}', ${app.port}); closeSpotlight();">
                    <div class="app-icon" style="background: linear-gradient(135deg, ${app.color}, ${adjustColor(app.color, -30)})">
                        ${createIcon(app.icon)}
                    </div>
                    <span>${app.name}</span>
                </div>
            `).join('');
        });

        function closeSpotlight() {
            document.getElementById('spotlight').classList.remove('active');
            document.getElementById('searchInput').value = '';
            document.getElementById('searchResults').innerHTML = '';
        }

        document.getElementById('spotlight').addEventListener('click', e => {
            if (e.target.id === 'spotlight') closeSpotlight();
        });

        // Keyboard shortcuts
        document.addEventListener('keydown', e => {
            if (e.key === 'Escape') {
                closeApp();
                closeSpotlight();
                isJiggling = false;
                render();
            }
        });

        render();
    </script>
</body>
</html>'''

@app.route("/")
def index():
    return Response(INDEX_HTML, mimetype="text/html")

@app.route("/manifest.json")
def manifest():
    return jsonify({
        "name": "HolmOS",
        "short_name": "HolmOS",
        "start_url": "/",
        "display": "standalone",
        "background_color": "#1e1e2e",
        "theme_color": "#1e1e2e",
        "icons": [
            {"src": "/icon-192.png", "sizes": "192x192", "type": "image/png"},
            {"src": "/icon-512.png", "sizes": "512x512", "type": "image/png"}
        ]
    })

@app.route("/api/registry")
def get_registry():
    return jsonify(APP_REGISTRY)

@app.route("/health")
def health():
    return jsonify({"status": "healthy"})

@app.route("/ready")
def ready():
    return jsonify({"status": "ready"})

if __name__ == "__main__":
    port = int(os.environ.get("PORT", 8080))
    app.run(host="0.0.0.0", port=port)
