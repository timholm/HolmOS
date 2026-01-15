// HolmOS AI Agents Load Test
// Tests agent services: nova, merchant, pulse, gateway, scribe, vault

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { CONFIG, serviceUrl } from './config.js';

// Custom metrics per agent
const novaLatency = new Trend('nova_latency', true);
const merchantLatency = new Trend('merchant_latency', true);
const pulseLatency = new Trend('pulse_latency', true);
const gatewayLatency = new Trend('gateway_latency', true);
const scribeLatency = new Trend('scribe_latency', true);
const vaultLatency = new Trend('vault_latency', true);
const agentErrors = new Counter('agent_errors');
const agentSuccess = new Rate('agent_success');

// Get load profile from environment or use standard
const LOAD_PROFILE = __ENV.LOAD_PROFILE || 'standard';

export const options = {
  scenarios: {
    agent_load: {
      executor: 'ramping-vus',
      stages: CONFIG.loadStages[LOAD_PROFILE] || CONFIG.loadStages.standard,
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    'nova_latency': ['p(95)<2000', 'avg<500'],
    'merchant_latency': ['p(95)<3000', 'avg<1000'],  // May process requests
    'pulse_latency': ['p(95)<1500', 'avg<300'],      // Should be fast (metrics)
    'gateway_latency': ['p(95)<1000', 'avg<200'],    // Critical path, must be fast
    'scribe_latency': ['p(95)<2000', 'avg<500'],
    'vault_latency': ['p(95)<2000', 'avg<500'],
    'agent_success': ['rate>0.95'],
    'http_req_failed': ['rate<0.05'],
  },
  tags: {
    testType: 'agents',
    loadProfile: LOAD_PROFILE,
  },
};

// Agent configurations with their specific endpoints
const agents = [
  {
    name: 'nova',
    latencyMetric: novaLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'merchant',
    latencyMetric: merchantLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'pulse',
    latencyMetric: pulseLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'gateway',
    latencyMetric: gatewayLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'scribe',
    latencyMetric: scribeLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'vault',
    latencyMetric: vaultLatency,
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
];

export default function () {
  for (const agent of agents) {
    group(`agent_${agent.name}`, function () {
      for (const endpoint of agent.endpoints) {
        const url = serviceUrl(agent.name, endpoint.path);

        const response = http.get(url, {
          timeout: '10s',
          tags: {
            service: agent.name,
            endpoint: endpoint.name,
            category: 'agent',
          },
        });

        agent.latencyMetric.add(response.timings.duration, {
          endpoint: endpoint.name,
        });

        const success = check(response, {
          'status is 2xx': (r) => r.status >= 200 && r.status < 300,
          'response time acceptable': (r) => r.timings.duration < 3000,
        });

        if (success) {
          agentSuccess.add(1, { agent: agent.name });
        } else {
          agentErrors.add(1, { agent: agent.name });
          agentSuccess.add(0, { agent: agent.name });
        }

        sleep(0.1);
      }
    });
  }

  sleep(0.5);
}

export function handleSummary(data) {
  return {
    'stdout': generateAgentReport(data),
    'results/agents-summary.json': JSON.stringify(data, null, 2),
  };
}

function generateAgentReport(data) {
  let report = '\n=== HolmOS AI Agents Load Test Report ===\n\n';
  report += `Load Profile: ${LOAD_PROFILE}\n\n`;

  // Agent-specific metrics
  const agentMetrics = [
    { name: 'Nova', metric: 'nova_latency' },
    { name: 'Merchant', metric: 'merchant_latency' },
    { name: 'Pulse', metric: 'pulse_latency' },
    { name: 'Gateway', metric: 'gateway_latency' },
    { name: 'Scribe', metric: 'scribe_latency' },
    { name: 'Vault', metric: 'vault_latency' },
  ];

  for (const agent of agentMetrics) {
    const m = data.metrics[agent.metric];
    if (m) {
      report += `${agent.name}:\n`;
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

  if (data.metrics.agent_success) {
    report += `Success Rate: ${(data.metrics.agent_success.values.rate * 100).toFixed(2)}%\n`;
  }

  // Threshold results
  report += '\nThreshold Results:\n';
  for (const [name, threshold] of Object.entries(data.thresholds || {})) {
    const status = threshold.ok ? 'PASS' : 'FAIL';
    report += `  ${name}: ${status}\n`;
  }

  return report;
}
