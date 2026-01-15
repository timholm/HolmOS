// HolmOS Infrastructure Load Test
// Tests devops and monitoring services

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { CONFIG, serviceUrl } from './config.js';

// Custom metrics
const infraLatency = new Trend('infra_latency', true);
const holmGitLatency = new Trend('holm_git_latency', true);
const cicdLatency = new Trend('cicd_latency', true);
const deployLatency = new Trend('deploy_latency', true);
const clusterManagerLatency = new Trend('cluster_manager_latency', true);
const metricsLatency = new Trend('metrics_dashboard_latency', true);
const testDashboardLatency = new Trend('test_dashboard_latency', true);
const infraErrors = new Counter('infra_errors');
const infraSuccess = new Rate('infra_success');

// Get load profile from environment
const LOAD_PROFILE = __ENV.LOAD_PROFILE || 'light';  // Infrastructure defaults to light load

export const options = {
  scenarios: {
    infra_load: {
      executor: 'ramping-vus',
      stages: CONFIG.loadStages[LOAD_PROFILE] || CONFIG.loadStages.light,
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    'infra_latency': ['p(95)<3000', 'avg<1000'],
    'holm_git_latency': ['p(95)<3000', 'avg<1000'],
    'cicd_latency': ['p(95)<3000', 'avg<1000'],
    'deploy_latency': ['p(95)<3000', 'avg<1000'],
    'cluster_manager_latency': ['p(95)<2500', 'avg<800'],
    'metrics_dashboard_latency': ['p(95)<2000', 'avg<500'],
    'test_dashboard_latency': ['p(95)<2000', 'avg<500'],
    'infra_success': ['rate>0.90'],  // Slightly lower threshold for infra
    'http_req_failed': ['rate<0.10'],
  },
  tags: {
    testType: 'infrastructure',
    loadProfile: LOAD_PROFILE,
  },
};

// Infrastructure service configurations
const infraServices = [
  {
    name: 'holm-git',
    latencyMetric: holmGitLatency,
    category: 'devops',
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'cicd-controller',
    latencyMetric: cicdLatency,
    category: 'devops',
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'deploy-controller',
    latencyMetric: deployLatency,
    category: 'devops',
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'cluster-manager',
    latencyMetric: clusterManagerLatency,
    category: 'admin',
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'metrics-dashboard',
    latencyMetric: metricsLatency,
    category: 'monitoring',
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
  {
    name: 'test-dashboard',
    latencyMetric: testDashboardLatency,
    category: 'monitoring',
    endpoints: [
      { path: '/health', name: 'health' },
      { path: '/', name: 'homepage' },
    ],
  },
];

export default function () {
  for (const service of infraServices) {
    group(`infra_${service.name}`, function () {
      for (const endpoint of service.endpoints) {
        const url = serviceUrl(service.name, endpoint.path);

        const response = http.get(url, {
          timeout: '15s',  // Longer timeout for infrastructure
          tags: {
            service: service.name,
            endpoint: endpoint.name,
            category: service.category,
          },
        });

        // Track both service-specific and general latency
        service.latencyMetric.add(response.timings.duration, {
          endpoint: endpoint.name,
        });
        infraLatency.add(response.timings.duration, {
          service: service.name,
          endpoint: endpoint.name,
        });

        const success = check(response, {
          'status is 2xx': (r) => r.status >= 200 && r.status < 300,
          'response time acceptable': (r) => r.timings.duration < 5000,
        });

        if (success) {
          infraSuccess.add(1, { service: service.name });
        } else {
          infraErrors.add(1, { service: service.name });
          infraSuccess.add(0, { service: service.name });
        }

        sleep(0.2);
      }
    });
  }

  sleep(1);
}

export function handleSummary(data) {
  return {
    'stdout': generateInfraReport(data),
    'results/infrastructure-summary.json': JSON.stringify(data, null, 2),
  };
}

function generateInfraReport(data) {
  let report = '\n=== HolmOS Infrastructure Load Test Report ===\n\n';
  report += `Load Profile: ${LOAD_PROFILE}\n\n`;

  // Service-specific metrics
  const serviceMetrics = [
    { name: 'Holm Git', metric: 'holm_git_latency' },
    { name: 'CICD Controller', metric: 'cicd_latency' },
    { name: 'Deploy Controller', metric: 'deploy_latency' },
    { name: 'Cluster Manager', metric: 'cluster_manager_latency' },
    { name: 'Metrics Dashboard', metric: 'metrics_dashboard_latency' },
    { name: 'Test Dashboard', metric: 'test_dashboard_latency' },
  ];

  for (const service of serviceMetrics) {
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

  if (data.metrics.infra_success) {
    report += `Success Rate: ${(data.metrics.infra_success.values.rate * 100).toFixed(2)}%\n`;
  }

  // Threshold results
  report += '\nThreshold Results:\n';
  for (const [name, threshold] of Object.entries(data.thresholds || {})) {
    const status = threshold.ok ? 'PASS' : 'FAIL';
    report += `  ${name}: ${status}\n`;
  }

  return report;
}
