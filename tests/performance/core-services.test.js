// HolmOS Core Services Load Test
// Tests core entry points: holmos-shell, claude-pod, app-store, chat-hub

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { CONFIG, serviceUrl, getServicesByCategory } from './config.js';

// Custom metrics per service
const shellLatency = new Trend('holmos_shell_latency', true);
const claudePodLatency = new Trend('claude_pod_latency', true);
const appStoreLatency = new Trend('app_store_latency', true);
const chatHubLatency = new Trend('chat_hub_latency', true);
const coreServiceErrors = new Counter('core_service_errors');
const coreServiceSuccess = new Rate('core_service_success');

// Get load profile from environment or use standard
const LOAD_PROFILE = __ENV.LOAD_PROFILE || 'standard';

export const options = {
  scenarios: {
    core_load: {
      executor: 'ramping-vus',
      stages: CONFIG.loadStages[LOAD_PROFILE] || CONFIG.loadStages.standard,
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    'holmos_shell_latency': ['p(95)<2000', 'avg<500'],
    'claude_pod_latency': ['p(95)<3000', 'avg<1000'],  // AI service may be slower
    'app_store_latency': ['p(95)<2000', 'avg<500'],
    'chat_hub_latency': ['p(95)<2000', 'avg<500'],
    'core_service_success': ['rate>0.95'],
    'http_req_failed': ['rate<0.05'],
    'http_req_duration': ['p(95)<3000'],
  },
  tags: {
    testType: 'core-services',
    loadProfile: LOAD_PROFILE,
  },
};

export default function () {
  // Test HolmOS Shell (main UI)
  group('holmos_shell', function () {
    testService('holmos-shell', shellLatency, [
      { path: '/', name: 'homepage' },
      { path: '/health', name: 'health' },
    ]);
  });

  // Test Claude Pod (AI interface)
  group('claude_pod', function () {
    testService('claude-pod', claudePodLatency, [
      { path: '/', name: 'homepage' },
      { path: '/health', name: 'health' },
    ]);
  });

  // Test App Store
  group('app_store', function () {
    testService('app-store', appStoreLatency, [
      { path: '/', name: 'homepage' },
      { path: '/health', name: 'health' },
    ]);
  });

  // Test Chat Hub
  group('chat_hub', function () {
    testService('chat-hub', chatHubLatency, [
      { path: '/', name: 'homepage' },
      { path: '/health', name: 'health' },
    ]);
  });

  sleep(1);
}

function testService(serviceName, latencyMetric, endpoints) {
  for (const endpoint of endpoints) {
    const url = serviceUrl(serviceName, endpoint.path);

    const response = http.get(url, {
      timeout: '10s',
      tags: {
        service: serviceName,
        endpoint: endpoint.name,
      },
    });

    latencyMetric.add(response.timings.duration, {
      endpoint: endpoint.name,
    });

    const success = check(response, {
      'status is 2xx': (r) => r.status >= 200 && r.status < 300,
      'response time acceptable': (r) => r.timings.duration < 3000,
      'body is not empty': (r) => r.body && r.body.length > 0,
    });

    if (success) {
      coreServiceSuccess.add(1, { service: serviceName });
    } else {
      coreServiceErrors.add(1, { service: serviceName });
      coreServiceSuccess.add(0, { service: serviceName });
    }

    sleep(0.2);
  }
}

export function handleSummary(data) {
  return {
    'stdout': generateCoreReport(data),
    'results/core-services-summary.json': JSON.stringify(data, null, 2),
  };
}

function generateCoreReport(data) {
  let report = '\n=== HolmOS Core Services Load Test Report ===\n\n';
  report += `Load Profile: ${LOAD_PROFILE}\n\n`;

  // Service-specific metrics
  const services = [
    { name: 'HolmOS Shell', metric: 'holmos_shell_latency' },
    { name: 'Claude Pod', metric: 'claude_pod_latency' },
    { name: 'App Store', metric: 'app_store_latency' },
    { name: 'Chat Hub', metric: 'chat_hub_latency' },
  ];

  for (const service of services) {
    const m = data.metrics[service.metric];
    if (m) {
      report += `${service.name}:\n`;
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

  if (data.metrics.core_service_success) {
    report += `Success Rate: ${(data.metrics.core_service_success.values.rate * 100).toFixed(2)}%\n`;
  }

  // Threshold results
  report += '\nThreshold Results:\n';
  for (const [name, threshold] of Object.entries(data.thresholds || {})) {
    const status = threshold.ok ? 'PASS' : 'FAIL';
    report += `  ${name}: ${status}\n`;
  }

  return report;
}
