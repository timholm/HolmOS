// HolmOS Apps Load Test
// Tests application services: clock, calculator, file-manager, settings, etc.

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { CONFIG, serviceUrl } from './config.js';

// Custom metrics
const appLatency = new Trend('app_latency', true);
const clockAppLatency = new Trend('clock_app_latency', true);
const calculatorAppLatency = new Trend('calculator_app_latency', true);
const fileManagerLatency = new Trend('file_manager_latency', true);
const settingsLatency = new Trend('settings_latency', true);
const terminalLatency = new Trend('terminal_latency', true);
const appErrors = new Counter('app_errors');
const appSuccess = new Rate('app_success');

// Get load profile from environment
const LOAD_PROFILE = __ENV.LOAD_PROFILE || 'standard';

export const options = {
  scenarios: {
    app_load: {
      executor: 'ramping-vus',
      stages: CONFIG.loadStages[LOAD_PROFILE] || CONFIG.loadStages.standard,
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    'app_latency': ['p(95)<2000', 'avg<500'],
    'clock_app_latency': ['p(95)<1500', 'avg<300'],
    'calculator_app_latency': ['p(95)<1500', 'avg<300'],
    'file_manager_latency': ['p(95)<2500', 'avg<800'],  // File ops may be slower
    'settings_latency': ['p(95)<2000', 'avg<500'],
    'terminal_latency': ['p(95)<2000', 'avg<500'],
    'app_success': ['rate>0.95'],
    'http_req_failed': ['rate<0.05'],
  },
  tags: {
    testType: 'apps',
    loadProfile: LOAD_PROFILE,
  },
};

// App configurations
const apps = [
  {
    name: 'clock-app',
    latencyMetric: clockAppLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'calculator-app',
    latencyMetric: calculatorAppLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'file-web-nautilus',
    latencyMetric: fileManagerLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'settings-web',
    latencyMetric: settingsLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'terminal-web',
    latencyMetric: terminalLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
];

export default function () {
  for (const app of apps) {
    group(`app_${app.name}`, function () {
      for (const endpoint of app.endpoints) {
        const url = serviceUrl(app.name, endpoint.path);

        const response = http.get(url, {
          timeout: '10s',
          tags: {
            service: app.name,
            endpoint: endpoint.name,
            category: 'app',
          },
        });

        // Track both app-specific and general latency
        app.latencyMetric.add(response.timings.duration, {
          endpoint: endpoint.name,
        });
        appLatency.add(response.timings.duration, {
          app: app.name,
          endpoint: endpoint.name,
        });

        const success = check(response, {
          'status is 2xx': (r) => r.status >= 200 && r.status < 300,
          'response time acceptable': (r) => r.timings.duration < 3000,
          'body is not empty': (r) => r.body && r.body.length > 0,
        });

        if (success) {
          appSuccess.add(1, { app: app.name });
        } else {
          appErrors.add(1, { app: app.name });
          appSuccess.add(0, { app: app.name });
        }

        sleep(0.1);
      }
    });
  }

  sleep(0.5);
}

export function handleSummary(data) {
  return {
    'stdout': generateAppReport(data),
    'results/apps-summary.json': JSON.stringify(data, null, 2),
  };
}

function generateAppReport(data) {
  let report = '\n=== HolmOS Apps Load Test Report ===\n\n';
  report += `Load Profile: ${LOAD_PROFILE}\n\n`;

  // App-specific metrics
  const appMetrics = [
    { name: 'Clock App', metric: 'clock_app_latency' },
    { name: 'Calculator App', metric: 'calculator_app_latency' },
    { name: 'File Manager', metric: 'file_manager_latency' },
    { name: 'Settings', metric: 'settings_latency' },
    { name: 'Terminal', metric: 'terminal_latency' },
  ];

  for (const app of appMetrics) {
    const m = data.metrics[app.metric];
    if (m) {
      report += `${app.name}:\n`;
      report += `  Avg: ${m.values.avg?.toFixed(2) || 'N/A'}ms\n`;
      report += `  P95: ${m.values['p(95)']?.toFixed(2) || 'N/A'}ms\n`;
      report += `  P99: ${m.values['p(99)']?.toFixed(2) || 'N/A'}ms\n`;
      report += `  Max: ${m.values.max?.toFixed(2) || 'N/A'}ms\n\n`;
    }
  }

  // Overall stats
  if (data.metrics.http_reqs) {
    report += `Total Requests: ${data.metrics.http_reqs.values.count}\n`;
    report += `Requests/sec: ${data.metrics.http_reqs.values.rate?.toFixed(2)}\n`;
  }

  if (data.metrics.app_success) {
    report += `Success Rate: ${(data.metrics.app_success.values.rate * 100).toFixed(2)}%\n`;
  }

  // Threshold results
  report += '\nThreshold Results:\n';
  for (const [name, threshold] of Object.entries(data.thresholds || {})) {
    const status = threshold.ok ? 'PASS' : 'FAIL';
    report += `  ${name}: ${status}\n`;
  }

  return report;
}
