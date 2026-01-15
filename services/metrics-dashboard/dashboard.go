package main

import (
	"net/http"
)

func serveDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(dashboardHTML))
}

const dashboardHTML = \`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Metrics Dashboard - HolmOS Cluster</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns/dist/chartjs-adapter-date-fns.bundle.min.js"></script>
    <style>
        :root {
            --bg-primary: #0f0f1a;
            --bg-secondary: #1a1a2e;
            --bg-card: #16213e;
            --bg-hover: #1f3460;
            --text-primary: #e4e4e7;
            --text-secondary: #a1a1aa;
            --accent-blue: #3b82f6;
            --accent-green: #10b981;
            --accent-yellow: #f59e0b;
            --accent-red: #ef4444;
            --accent-purple: #8b5cf6;
            --accent-cyan: #06b6d4;
            --border-color: #2d3748;
        }
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'Inter', -apple-system, BlinkMacSystemFont, sans-serif; background: var(--bg-primary); color: var(--text-primary); min-height: 100vh; }
        .app-container { display: flex; min-height: 100vh; }
        .sidebar { width: 240px; background: var(--bg-secondary); border-right: 1px solid var(--border-color); display: flex; flex-direction: column; position: fixed; height: 100vh; z-index: 100; }
        .logo { padding: 20px; border-bottom: 1px solid var(--border-color); display: flex; align-items: center; gap: 12px; }
        .logo-icon { width: 36px; height: 36px; background: linear-gradient(135deg, var(--accent-blue), var(--accent-purple)); border-radius: 8px; display: flex; align-items: center; justify-content: center; font-size: 18px; }
        .logo h1 { font-size: 1.1rem; font-weight: 600; }
        .logo span { color: var(--text-secondary); font-size: 0.75rem; }
        .nav-menu { flex: 1; padding: 16px 0; }
        .nav-item { display: flex; align-items: center; gap: 12px; padding: 12px 20px; color: var(--text-secondary); cursor: pointer; transition: all 0.2s; border-left: 3px solid transparent; }
        .nav-item:hover, .nav-item.active { background: var(--bg-hover); color: var(--text-primary); border-left-color: var(--accent-blue); }
        .nav-item svg { width: 20px; height: 20px; }
        .main-content { flex: 1; margin-left: 240px; padding: 24px; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px; }
        .header h2 { font-size: 1.5rem; font-weight: 600; }
        .time-range-selector { display: flex; gap: 8px; background: var(--bg-secondary); padding: 4px; border-radius: 8px; }
        .time-btn { padding: 8px 16px; border: none; background: transparent; color: var(--text-secondary); border-radius: 6px; cursor: pointer; transition: all 0.2s; font-size: 0.875rem; }
        .time-btn:hover { color: var(--text-primary); }
        .time-btn.active { background: var(--accent-blue); color: white; }
        .summary-grid { display: grid; grid-template-columns: repeat(4, 1fr); gap: 16px; margin-bottom: 24px; }
        .summary-card { background: var(--bg-card); border-radius: 12px; padding: 20px; border: 1px solid var(--border-color); }
        .summary-card .label { color: var(--text-secondary); font-size: 0.875rem; margin-bottom: 8px; display: flex; align-items: center; gap: 8px; }
        .summary-card .value { font-size: 2rem; font-weight: 700; }
        .summary-card .sub { color: var(--text-secondary); font-size: 0.8rem; margin-top: 4px; }
        .summary-card.cpu .value { color: var(--accent-cyan); }
        .summary-card.memory .value { color: var(--accent-green); }
        .summary-card.nodes .value { color: var(--accent-yellow); }
        .summary-card.pods .value { color: var(--accent-purple); }
        .charts-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 16px; margin-bottom: 24px; }
        .chart-card { background: var(--bg-card); border-radius: 12px; padding: 20px; border: 1px solid var(--border-color); }
        .chart-card h3 { font-size: 1rem; margin-bottom: 16px; display: flex; align-items: center; gap: 8px; }
        .chart-card h3::before { content: ''; width: 10px; height: 10px; border-radius: 50%; }
        .chart-card.cpu h3::before { background: var(--accent-cyan); }
        .chart-card.memory h3::before { background: var(--accent-green); }
        .chart-card.network h3::before { background: var(--accent-yellow); }
        .chart-card.disk h3::before { background: var(--accent-purple); }
        .chart-container { height: 200px; position: relative; }
        .section-title { font-size: 1.25rem; font-weight: 600; margin-bottom: 16px; display: flex; align-items: center; gap: 8px; }
        .nodes-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; margin-bottom: 24px; }
        .node-card { background: var(--bg-card); border-radius: 10px; padding: 16px; border: 1px solid var(--border-color); transition: all 0.2s; cursor: pointer; }
        .node-card:hover { border-color: var(--accent-blue); transform: translateY(-2px); }
        .node-card .node-header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px; }
        .node-card .node-name { font-weight: 600; font-size: 0.95rem; }
        .status-badge { padding: 4px 10px; border-radius: 12px; font-size: 0.7rem; font-weight: 600; text-transform: uppercase; }
        .status-badge.ready { background: rgba(16, 185, 129, 0.2); color: var(--accent-green); }
        .status-badge.notready { background: rgba(239, 68, 68, 0.2); color: var(--accent-red); }
        .node-metrics { display: grid; grid-template-columns: repeat(2, 1fr); gap: 12px; }
        .metric-item { display: flex; flex-direction: column; gap: 4px; }
        .metric-item .metric-label { font-size: 0.75rem; color: var(--text-secondary); }
        .metric-item .metric-bar { height: 6px; background: var(--bg-primary); border-radius: 3px; overflow: hidden; }
        .metric-item .metric-bar-fill { height: 100%; border-radius: 3px; transition: width 0.3s; }
        .metric-item .metric-bar-fill.cpu { background: var(--accent-cyan); }
        .metric-item .metric-bar-fill.memory { background: var(--accent-green); }
        .metric-item .metric-value { font-size: 0.85rem; font-weight: 600; }
        .table-container { background: var(--bg-card); border-radius: 12px; border: 1px solid var(--border-color); overflow: hidden; margin-bottom: 24px; }
        .table-header { padding: 16px 20px; border-bottom: 1px solid var(--border-color); display: flex; justify-content: space-between; align-items: center; }
        .table-header h3 { font-size: 1rem; }
        .search-input { background: var(--bg-primary); border: 1px solid var(--border-color); padding: 8px 12px; border-radius: 6px; color: var(--text-primary); width: 200px; }
        .search-input:focus { outline: none; border-color: var(--accent-blue); }
        table { width: 100%; border-collapse: collapse; }
        th, td { padding: 12px 20px; text-align: left; border-bottom: 1px solid var(--border-color); }
        th { background: var(--bg-secondary); color: var(--text-secondary); font-weight: 600; font-size: 0.8rem; text-transform: uppercase; }
        tr:hover { background: var(--bg-hover); }
        .deployment-name { font-weight: 600; }
        .replica-badge { display: inline-flex; align-items: center; gap: 4px; }
        .replica-badge.healthy { color: var(--accent-green); }
        .replica-badge.degraded { color: var(--accent-yellow); }
        .alerts-container { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; margin-bottom: 24px; }
        .alert-rules-card, .triggered-alerts-card { background: var(--bg-card); border-radius: 12px; border: 1px solid var(--border-color); }
        .alert-card-header { padding: 16px 20px; border-bottom: 1px solid var(--border-color); display: flex; justify-content: space-between; align-items: center; }
        .alert-card-header h3 { font-size: 1rem; }
        .btn-add { background: var(--accent-blue); color: white; border: none; padding: 8px 16px; border-radius: 6px; cursor: pointer; font-size: 0.85rem; display: flex; align-items: center; gap: 6px; }
        .btn-add:hover { background: #2563eb; }
        .alert-list { max-height: 300px; overflow-y: auto; }
        .alert-item { padding: 12px 20px; border-bottom: 1px solid var(--border-color); display: flex; justify-content: space-between; align-items: center; }
        .alert-item:last-child { border-bottom: none; }
        .alert-info { display: flex; flex-direction: column; gap: 4px; }
        .alert-name { font-weight: 600; font-size: 0.9rem; }
        .alert-condition { color: var(--text-secondary); font-size: 0.8rem; }
        .alert-actions { display: flex; gap: 8px; }
        .btn-icon { background: transparent; border: none; color: var(--text-secondary); cursor: pointer; padding: 4px; }
        .btn-icon:hover { color: var(--text-primary); }
        .btn-icon.delete:hover { color: var(--accent-red); }
        .triggered-alert { padding: 12px 20px; border-bottom: 1px solid var(--border-color); background: rgba(239, 68, 68, 0.1); }
        .triggered-alert.resolved { background: transparent; opacity: 0.6; }
        .triggered-alert .alert-message { font-weight: 600; color: var(--accent-red); margin-bottom: 4px; }
        .triggered-alert.resolved .alert-message { color: var(--accent-green); }
        .triggered-alert .alert-time { font-size: 0.75rem; color: var(--text-secondary); }
        .modal-overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0, 0, 0, 0.7); display: none; align-items: center; justify-content: center; z-index: 1000; }
        .modal-overlay.active { display: flex; }
        .modal { background: var(--bg-card); border-radius: 12px; width: 400px; max-width: 90%; }
        .modal-header { padding: 20px; border-bottom: 1px solid var(--border-color); display: flex; justify-content: space-between; align-items: center; }
        .modal-header h3 { font-size: 1.1rem; }
        .modal-body { padding: 20px; }
        .form-group { margin-bottom: 16px; }
        .form-group label { display: block; margin-bottom: 6px; font-size: 0.875rem; color: var(--text-secondary); }
        .form-group input, .form-group select { width: 100%; padding: 10px 12px; background: var(--bg-primary); border: 1px solid var(--border-color); border-radius: 6px; color: var(--text-primary); font-size: 0.9rem; }
        .form-group input:focus, .form-group select:focus { outline: none; border-color: var(--accent-blue); }
        .modal-footer { padding: 16px 20px; border-top: 1px solid var(--border-color); display: flex; justify-content: flex-end; gap: 12px; }
        .btn-cancel { background: transparent; border: 1px solid var(--border-color); padding: 8px 16px; border-radius: 6px; color: var(--text-secondary); cursor: pointer; }
        .btn-cancel:hover { color: var(--text-primary); }
        .btn-save { background: var(--accent-blue); border: none; padding: 8px 16px; border-radius: 6px; color: white; cursor: pointer; }
        .btn-save:hover { background: #2563eb; }
        .view-content { display: none; }
        .view-content.active { display: block; }
        @media (max-width: 1200px) { .summary-grid { grid-template-columns: repeat(2, 1fr); } .charts-grid { grid-template-columns: 1fr; } .alerts-container { grid-template-columns: 1fr; } }
        @media (max-width: 768px) { .sidebar { width: 60px; } .sidebar .logo h1, .sidebar .logo span, .sidebar .nav-item span { display: none; } .main-content { margin-left: 60px; } .summary-grid { grid-template-columns: 1fr; } }
        ::-webkit-scrollbar { width: 8px; height: 8px; }
        ::-webkit-scrollbar-track { background: var(--bg-primary); }
        ::-webkit-scrollbar-thumb { background: var(--border-color); border-radius: 4px; }
        ::-webkit-scrollbar-thumb:hover { background: #4a5568; }
        .empty-state { padding: 40px; text-align: center; color: var(--text-secondary); }
        .loading { display: flex; align-items: center; justify-content: center; padding: 40px; }
        .spinner { width: 40px; height: 40px; border: 3px solid var(--border-color); border-top-color: var(--accent-blue); border-radius: 50%; animation: spin 1s linear infinite; }
        @keyframes spin { to { transform: rotate(360deg); } }
    </style>
</head>
<body>
    <div class="app-container">
        <nav class="sidebar">
            <div class="logo">
                <div class="logo-icon">M</div>
                <div>
                    <h1>Metrics</h1>
                    <span>HolmOS Cluster</span>
                </div>
            </div>
            <div class="nav-menu">
                <div class="nav-item active" data-view="overview">
                    <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2V6zM14 6a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2V6zM4 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2H6a2 2 0 01-2-2v-2zM14 16a2 2 0 012-2h2a2 2 0 012 2v2a2 2 0 01-2 2h-2a2 2 0 01-2-2v-2z" /></svg>
                    <span>Overview</span>
                </div>
                <div class="nav-item" data-view="nodes">
                    <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" /></svg>
                    <span>Nodes</span>
                </div>
                <div class="nav-item" data-view="deployments">
                    <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10" /></svg>
                    <span>Deployments</span>
                </div>
                <div class="nav-item" data-view="alerts">
                    <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" /></svg>
                    <span>Alerts</span>
                </div>
            </div>
        </nav>
        <main class="main-content">
            <div id="overview-view" class="view-content active">
                <div class="header">
                    <h2>Cluster Overview</h2>
                    <div class="time-range-selector">
                        <button class="time-btn active" data-range="1h">1H</button>
                        <button class="time-btn" data-range="6h">6H</button>
                        <button class="time-btn" data-range="24h">24H</button>
                        <button class="time-btn" data-range="7d">7D</button>
                        <button class="time-btn" data-range="30d">30D</button>
                    </div>
                </div>
                <div class="summary-grid">
                    <div class="summary-card cpu"><div class="label">CPU Usage</div><div class="value" id="cluster-cpu">--</div><div class="sub" id="cluster-cpu-detail">-- cores used of -- cores</div></div>
                    <div class="summary-card memory"><div class="label">Memory Usage</div><div class="value" id="cluster-memory">--</div><div class="sub" id="cluster-memory-detail">-- GB used of -- GB</div></div>
                    <div class="summary-card nodes"><div class="label">Nodes</div><div class="value" id="cluster-nodes">--</div><div class="sub" id="cluster-nodes-detail">-- ready</div></div>
                    <div class="summary-card pods"><div class="label">Pods</div><div class="value" id="cluster-pods">--</div><div class="sub" id="cluster-deployments">-- deployments</div></div>
                </div>
                <div class="charts-grid">
                    <div class="chart-card cpu"><h3>Cluster CPU</h3><div class="chart-container"><canvas id="cluster-cpu-chart"></canvas></div></div>
                    <div class="chart-card memory"><h3>Cluster Memory</h3><div class="chart-container"><canvas id="cluster-memory-chart"></canvas></div></div>
                    <div class="chart-card network"><h3>Node CPU Comparison</h3><div class="chart-container"><canvas id="node-cpu-chart"></canvas></div></div>
                    <div class="chart-card disk"><h3>Node Memory Comparison</h3><div class="chart-container"><canvas id="node-memory-chart"></canvas></div></div>
                </div>
            </div>
            <div id="nodes-view" class="view-content">
                <div class="header"><h2>Cluster Nodes</h2></div>
                <div class="nodes-grid" id="nodes-grid"><div class="loading"><div class="spinner"></div></div></div>
            </div>
            <div id="deployments-view" class="view-content">
                <div class="header"><h2>Deployments</h2></div>
                <div class="table-container">
                    <div class="table-header"><h3>All Deployments</h3><input type="text" class="search-input" placeholder="Search deployments..." id="deployment-search"></div>
                    <table><thead><tr><th>Name</th><th>Namespace</th><th>Replicas</th><th>CPU</th><th>Memory</th></tr></thead><tbody id="deployments-table"></tbody></table>
                </div>
            </div>
            <div id="alerts-view" class="view-content">
                <div class="header"><h2>Alert Management</h2></div>
                <div class="alerts-container">
                    <div class="alert-rules-card">
                        <div class="alert-card-header"><h3>Alert Rules</h3><button class="btn-add" onclick="openAlertModal()"><svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4"/></svg>Add Rule</button></div>
                        <div class="alert-list" id="alert-rules-list"></div>
                    </div>
                    <div class="triggered-alerts-card">
                        <div class="alert-card-header"><h3>Triggered Alerts</h3></div>
                        <div class="alert-list" id="triggered-alerts-list"></div>
                    </div>
                </div>
            </div>
        </main>
    </div>
    <div class="modal-overlay" id="alert-modal">
        <div class="modal">
            <div class="modal-header"><h3>Create Alert Rule</h3><button class="btn-icon" onclick="closeAlertModal()"><svg width="20" height="20" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12"/></svg></button></div>
            <div class="modal-body">
                <div class="form-group"><label>Rule Name</label><input type="text" id="alert-name" placeholder="High CPU Alert"></div>
                <div class="form-group"><label>Metric</label><select id="alert-metric"><option value="cpu">CPU Usage (%)</option><option value="memory">Memory Usage (%)</option></select></div>
                <div class="form-group"><label>Condition</label><select id="alert-condition"><option value=">">> Greater than</option><option value=">=">>= Greater than or equal</option><option value="<">< Less than</option><option value="<="><= Less than or equal</option></select></div>
                <div class="form-group"><label>Threshold (%)</label><input type="number" id="alert-threshold" placeholder="90" min="0" max="100"></div>
                <div class="form-group"><label>Node (leave empty for all)</label><select id="alert-node"><option value="*">All Nodes</option></select></div>
            </div>
            <div class="modal-footer"><button class="btn-cancel" onclick="closeAlertModal()">Cancel</button><button class="btn-save" onclick="saveAlertRule()">Create Rule</button></div>
        </div>
    </div>
    <script>
        let currentTimeRange = '1h';
        let clusterCPUChart, clusterMemoryChart, nodeCPUChart, nodeMemoryChart;
        let nodes = [];
        let deployments = [];
        let alertRules = [];
        let triggeredAlerts = [];
        const chartConfig = {
            responsive: true, maintainAspectRatio: false,
            interaction: { intersect: false, mode: 'index' },
            scales: {
                x: { type: 'time', time: { unit: 'minute' }, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: '#a1a1aa' } },
                y: { beginAtZero: true, max: 100, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: '#a1a1aa', callback: (v) => v + '%' } }
            },
            plugins: { legend: { display: false }, tooltip: { backgroundColor: '#1a1a2e', borderColor: '#2d3748', borderWidth: 1, titleColor: '#e4e4e7', bodyColor: '#a1a1aa', callbacks: { label: (ctx) => ctx.parsed.y.toFixed(1) + '%' } } },
            elements: { point: { radius: 0 }, line: { tension: 0.3, borderWidth: 2 } }
        };
        const barChartConfig = {
            responsive: true, maintainAspectRatio: false, indexAxis: 'y',
            scales: { x: { beginAtZero: true, max: 100, grid: { color: 'rgba(255,255,255,0.05)' }, ticks: { color: '#a1a1aa', callback: (v) => v + '%' } }, y: { grid: { display: false }, ticks: { color: '#a1a1aa' } } },
            plugins: { legend: { display: false }, tooltip: { backgroundColor: '#1a1a2e', borderColor: '#2d3748', borderWidth: 1, callbacks: { label: (ctx) => ctx.parsed.x.toFixed(1) + '%' } } }
        };
        function initCharts() {
            clusterCPUChart = new Chart(document.getElementById('cluster-cpu-chart').getContext('2d'), { type: 'line', data: { datasets: [{ label: 'CPU', data: [], borderColor: '#06b6d4', backgroundColor: 'rgba(6, 182, 212, 0.1)', fill: true }] }, options: chartConfig });
            clusterMemoryChart = new Chart(document.getElementById('cluster-memory-chart').getContext('2d'), { type: 'line', data: { datasets: [{ label: 'Memory', data: [], borderColor: '#10b981', backgroundColor: 'rgba(16, 185, 129, 0.1)', fill: true }] }, options: chartConfig });
            nodeCPUChart = new Chart(document.getElementById('node-cpu-chart').getContext('2d'), { type: 'bar', data: { labels: [], datasets: [{ label: 'CPU %', data: [], backgroundColor: '#06b6d4' }] }, options: barChartConfig });
            nodeMemoryChart = new Chart(document.getElementById('node-memory-chart').getContext('2d'), { type: 'bar', data: { labels: [], datasets: [{ label: 'Memory %', data: [], backgroundColor: '#10b981' }] }, options: barChartConfig });
        }
        async function fetchClusterSummary() {
            try {
                const r = await fetch('/api/cluster'); const d = await r.json();
                document.getElementById('cluster-cpu').textContent = d.cpu_pct.toFixed(1) + '%';
                document.getElementById('cluster-cpu-detail').textContent = (d.used_cpu_cores/1000).toFixed(1) + ' cores of ' + (d.total_cpu_cores/1000).toFixed(1) + ' cores';
                document.getElementById('cluster-memory').textContent = d.memory_pct.toFixed(1) + '%';
                document.getElementById('cluster-memory-detail').textContent = d.used_memory_gb.toFixed(1) + ' GB of ' + d.total_memory_gb.toFixed(1) + ' GB';
                document.getElementById('cluster-nodes').textContent = d.total_nodes;
                document.getElementById('cluster-nodes-detail').textContent = d.ready_nodes + ' ready';
                document.getElementById('cluster-pods').textContent = d.total_pods;
                document.getElementById('cluster-deployments').textContent = d.total_deployments + ' deployments';
            } catch (e) { console.error('Failed to fetch cluster summary:', e); }
        }
        async function fetchNodes() {
            try { const r = await fetch('/api/nodes'); nodes = await r.json(); updateNodesGrid(); updateNodeCharts(); updateAlertNodeSelector(); } catch (e) { console.error('Failed to fetch nodes:', e); }
        }
        async function fetchDeployments() {
            try { const r = await fetch('/api/deployments?namespace=holm'); deployments = await r.json(); updateDeploymentsTable(); } catch (e) { console.error('Failed to fetch deployments:', e); }
        }
        async function fetchHistory() {
            try {
                const cpuR = await fetch('/api/history?metric=cluster_cpu&range=' + currentTimeRange); const cpuD = await cpuR.json();
                const memR = await fetch('/api/history?metric=cluster_mem&range=' + currentTimeRange); const memD = await memR.json();
                if (cpuD && cpuD.length > 0) { clusterCPUChart.data.datasets[0].data = cpuD.map(p => ({ x: new Date(p.timestamp), y: p.value })); clusterCPUChart.update('none'); }
                if (memD && memD.length > 0) { clusterMemoryChart.data.datasets[0].data = memD.map(p => ({ x: new Date(p.timestamp), y: p.value })); clusterMemoryChart.update('none'); }
            } catch (e) { console.error('Failed to fetch history:', e); }
        }
        async function fetchAlertRules() { try { const r = await fetch('/api/alerts/rules'); alertRules = await r.json(); updateAlertRulesList(); } catch (e) { console.error('Failed to fetch alert rules:', e); } }
        async function fetchTriggeredAlerts() { try { const r = await fetch('/api/alerts/triggered'); triggeredAlerts = await r.json(); updateTriggeredAlertsList(); } catch (e) { console.error('Failed to fetch triggered alerts:', e); } }
        function updateNodesGrid() {
            const g = document.getElementById('nodes-grid');
            if (!nodes || nodes.length === 0) { g.innerHTML = '<div class="empty-state">No nodes found</div>'; return; }
            g.innerHTML = nodes.map(n => '<div class="node-card" onclick="showNodeDetails(\'' + n.name + '\')"><div class="node-header"><span class="node-name">' + n.name + '</span><span class="status-badge ' + (n.status === 'Ready' ? 'ready' : 'notready') + '">' + n.status + '</span></div><div class="node-metrics"><div class="metric-item"><span class="metric-label">CPU</span><div class="metric-bar"><div class="metric-bar-fill cpu" style="width: ' + Math.min(100, n.cpu_pct) + '%"></div></div><span class="metric-value">' + n.cpu_pct.toFixed(1) + '%</span></div><div class="metric-item"><span class="metric-label">Memory</span><div class="metric-bar"><div class="metric-bar-fill memory" style="width: ' + Math.min(100, n.memory_pct) + '%"></div></div><span class="metric-value">' + n.memory_pct.toFixed(1) + '%</span></div></div><div style="margin-top: 12px; color: var(--text-secondary); font-size: 0.8rem;">' + n.pods + ' pods | ' + (n.cpu_cores/1000).toFixed(2) + ' CPU | ' + (n.memory_mb/1024).toFixed(1) + ' GB RAM</div></div>').join('');
        }
        function updateNodeCharts() {
            if (!nodes || nodes.length === 0) return;
            nodeCPUChart.data.labels = nodes.map(n => n.name);
            nodeCPUChart.data.datasets[0].data = nodes.map(n => n.cpu_pct);
            nodeCPUChart.data.datasets[0].backgroundColor = nodes.map(n => n.cpu_pct > 80 ? '#ef4444' : n.cpu_pct > 60 ? '#f59e0b' : '#06b6d4');
            nodeCPUChart.update('none');
            nodeMemoryChart.data.labels = nodes.map(n => n.name);
            nodeMemoryChart.data.datasets[0].data = nodes.map(n => n.memory_pct);
            nodeMemoryChart.data.datasets[0].backgroundColor = nodes.map(n => n.memory_pct > 80 ? '#ef4444' : n.memory_pct > 60 ? '#f59e0b' : '#10b981');
            nodeMemoryChart.update('none');
        }
        function updateDeploymentsTable() {
            const t = document.getElementById('deployments-table'); const s = document.getElementById('deployment-search').value.toLowerCase();
            const f = deployments.filter(d => d.name.toLowerCase().includes(s) || d.namespace.toLowerCase().includes(s));
            if (f.length === 0) { t.innerHTML = '<tr><td colspan="5" class="empty-state">No deployments found</td></tr>'; return; }
            t.innerHTML = f.map(d => '<tr><td class="deployment-name">' + d.name + '</td><td>' + d.namespace + '</td><td><span class="replica-badge ' + (d.ready >= d.replicas ? 'healthy' : 'degraded') + '">' + d.ready + '/' + d.replicas + '</span></td><td>' + d.cpu_total.toFixed(0) + 'm</td><td>' + d.memory_mb.toFixed(0) + ' MB</td></tr>').join('');
        }
        function updateAlertRulesList() {
            const l = document.getElementById('alert-rules-list');
            if (!alertRules || alertRules.length === 0) { l.innerHTML = '<div class="empty-state">No alert rules configured</div>'; return; }
            l.innerHTML = alertRules.map(r => '<div class="alert-item"><div class="alert-info"><span class="alert-name">' + r.name + '</span><span class="alert-condition">' + r.metric.toUpperCase() + ' ' + r.condition + ' ' + r.threshold + '% on ' + (r.node === '*' ? 'all nodes' : r.node) + '</span></div><div class="alert-actions"><button class="btn-icon delete" onclick="deleteAlertRule(\'' + r.id + '\')" title="Delete"><svg width="16" height="16" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16"/></svg></button></div></div>').join('');
        }
        function updateTriggeredAlertsList() {
            const l = document.getElementById('triggered-alerts-list');
            if (!triggeredAlerts || triggeredAlerts.length === 0) { l.innerHTML = '<div class="empty-state">No alerts triggered</div>'; return; }
            const sorted = [...triggeredAlerts].sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
            l.innerHTML = sorted.map(a => '<div class="triggered-alert ' + (a.resolved ? 'resolved' : '') + '"><div class="alert-message">' + a.message + '</div><div class="alert-condition">Value: ' + a.value.toFixed(1) + '% (threshold: ' + a.threshold + '%)</div><div class="alert-time">' + new Date(a.timestamp).toLocaleString() + '</div>' + (!a.resolved ? '<button class="btn-icon" onclick="resolveAlert(\'' + a.id + '\')" style="margin-top: 8px;">Mark Resolved</button>' : '') + '</div>').join('');
        }
        function updateAlertNodeSelector() { const s = document.getElementById('alert-node'); s.innerHTML = '<option value="*">All Nodes</option>'; nodes.forEach(n => { s.innerHTML += '<option value="' + n.name + '">' + n.name + '</option>'; }); }
        function openAlertModal() { document.getElementById('alert-modal').classList.add('active'); }
        function closeAlertModal() { document.getElementById('alert-modal').classList.remove('active'); document.getElementById('alert-name').value = ''; document.getElementById('alert-threshold').value = ''; }
        async function saveAlertRule() {
            const r = { name: document.getElementById('alert-name').value, metric: document.getElementById('alert-metric').value, condition: document.getElementById('alert-condition').value, threshold: parseFloat(document.getElementById('alert-threshold').value), node: document.getElementById('alert-node').value, enabled: true };
            if (!r.name || isNaN(r.threshold)) { alert('Please fill in all required fields'); return; }
            try { await fetch('/api/alerts/rules', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(r) }); closeAlertModal(); fetchAlertRules(); } catch (e) { console.error('Failed to create alert rule:', e); }
        }
        async function deleteAlertRule(id) { if (!confirm('Delete this alert rule?')) return; try { await fetch('/api/alerts/rules?id=' + id, { method: 'DELETE' }); fetchAlertRules(); } catch (e) { console.error('Failed to delete alert rule:', e); } }
        async function resolveAlert(id) { try { await fetch('/api/alerts/resolve?id=' + id, { method: 'POST' }); fetchTriggeredAlerts(); } catch (e) { console.error('Failed to resolve alert:', e); } }
        function showNodeDetails(nodeName) { console.log('Show details for:', nodeName); }
        document.querySelectorAll('.nav-item').forEach(i => { i.addEventListener('click', () => { document.querySelectorAll('.nav-item').forEach(j => j.classList.remove('active')); i.classList.add('active'); const v = i.dataset.view; document.querySelectorAll('.view-content').forEach(c => c.classList.remove('active')); document.getElementById(v + '-view').classList.add('active'); }); });
        document.querySelectorAll('.time-btn').forEach(b => { b.addEventListener('click', () => { document.querySelectorAll('.time-btn').forEach(c => c.classList.remove('active')); b.classList.add('active'); currentTimeRange = b.dataset.range; let u = 'minute'; if (currentTimeRange === '7d' || currentTimeRange === '30d') u = 'day'; else if (currentTimeRange === '24h') u = 'hour'; clusterCPUChart.options.scales.x.time.unit = u; clusterMemoryChart.options.scales.x.time.unit = u; fetchHistory(); }); });
        document.getElementById('deployment-search').addEventListener('input', updateDeploymentsTable);
        function init() { initCharts(); fetchClusterSummary(); fetchNodes(); fetchDeployments(); fetchHistory(); fetchAlertRules(); fetchTriggeredAlerts(); setInterval(() => { fetchClusterSummary(); fetchNodes(); fetchHistory(); fetchTriggeredAlerts(); }, 10000); setInterval(fetchDeployments, 30000); }
        init();
    </script>
</body>
</html>\`
