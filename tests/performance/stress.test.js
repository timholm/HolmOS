// HolmOS Stress Test
// Find breaking points and maximum capacity on Raspberry Pi hardware

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { CONFIG, serviceUrl, getAllServices } from './config.js';

// Custom metrics
const stressLatency = new Trend('stress_latency', true);
const stressErrors = new Counter('stress_errors');
const stressSuccess = new Rate('stress_success');
const breakingPointVUs = new Counter('breaking_point_vus');

// Get target service or test all
const TARGET_SERVICE = __ENV.TARGET_SERVICE || 'all';

export const options = {
  scenarios: {
    stress: {
      executor: 'ramping-vus',
      stages: CONFIG.loadStages.stress,
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    'stress_latency': ['p(95)<5000'],  // Higher threshold for stress test
    'stress_success': ['rate>0.80'],   // Accept more failures under stress
    'http_req_failed': ['rate<0.20'],
  },
  tags: {
    testType: 'stress',
    targetService: TARGET_SERVICE,
  },
};

export default function () {
  const services = TARGET_SERVICE === 'all'
    ? getAllServices()
    : [TARGET_SERVICE];

  for (const serviceName of services) {
    if (!CONFIG.services[serviceName]) {
      console.warn(`Unknown service: ${serviceName}`);
      continue;
    }

    group(`stress_${serviceName}`, function () {
      const healthUrl = serviceUrl(serviceName, '/health');
      const homeUrl = serviceUrl(serviceName, '/');

      // Test health endpoint
      const healthResponse = http.get(healthUrl, {
        timeout: '10s',
        tags: { service: serviceName, endpoint: 'health' },
      });

      stressLatency.add(healthResponse.timings.duration, {
        service: serviceName,
        endpoint: 'health',
      });

      const healthSuccess = check(healthResponse, {
        'health status is 2xx': (r) => r.status >= 200 && r.status < 300,
      });

      // Test homepage
      const homeResponse = http.get(homeUrl, {
        timeout: '10s',
        tags: { service: serviceName, endpoint: 'homepage' },
      });

      stressLatency.add(homeResponse.timings.duration, {
        service: serviceName,
        endpoint: 'homepage',
      });

      const homeSuccess = check(homeResponse, {
        'home status is 2xx': (r) => r.status >= 200 && r.status < 300,
      });

      if (healthSuccess && homeSuccess) {
        stressSuccess.add(1, { service: serviceName });
      } else {
        stressErrors.add(1, { service: serviceName });
        stressSuccess.add(0, { service: serviceName });

        // Track VU count when we see failures
        breakingPointVUs.add(__VU);
      }
    });

    sleep(0.05);  // Minimal delay during stress
  }
}

export function handleSummary(data) {
  return {
    'stdout': generateStressReport(data),
    'results/stress-summary.json': JSON.stringify(data, null, 2),
  };
}

function generateStressReport(data) {
  let report = '\n=== HolmOS Stress Test Report ===\n\n';
  report += `Target: ${TARGET_SERVICE}\n\n`;

  // Stress metrics
  if (data.metrics.stress_latency) {
    const latency = data.metrics.stress_latency.values;
    report += 'Response Times Under Stress:\n';
    report += `  Average: ${latency.avg?.toFixed(2) || 'N/A'}ms\n`;
    report += `  P50: ${latency['p(50)']?.toFixed(2) || 'N/A'}ms\n`;
    report += `  P90: ${latency['p(90)']?.toFixed(2) || 'N/A'}ms\n`;
    report += `  P95: ${latency['p(95)']?.toFixed(2) || 'N/A'}ms\n`;
    report += `  P99: ${latency['p(99)']?.toFixed(2) || 'N/A'}ms\n`;
    report += `  Max: ${latency.max?.toFixed(2) || 'N/A'}ms\n\n`;
  }

  // Error metrics
  if (data.metrics.stress_errors) {
    report += `Total Errors: ${data.metrics.stress_errors.values.count}\n`;
  }

  if (data.metrics.stress_success) {
    report += `Success Rate: ${(data.metrics.stress_success.values.rate * 100).toFixed(2)}%\n`;
  }

  // Breaking point analysis
  if (data.metrics.breaking_point_vus) {
    report += `\nBreaking Point Analysis:\n`;
    report += `  First failures observed at approximately ${data.metrics.breaking_point_vus.values.count} VUs\n`;
  }

  // Throughput
  if (data.metrics.http_reqs) {
    report += `\nThroughput:\n`;
    report += `  Total Requests: ${data.metrics.http_reqs.values.count}\n`;
    report += `  Requests/sec: ${data.metrics.http_reqs.values.rate?.toFixed(2)}\n`;
  }

  // Threshold results
  report += '\nThreshold Results:\n';
  for (const [name, threshold] of Object.entries(data.thresholds || {})) {
    const status = threshold.ok ? 'PASS' : 'FAIL';
    report += `  ${name}: ${status}\n`;
  }

  report += '\n=== Raspberry Pi Hardware Recommendations ===\n';
  report += 'Based on stress test results, adjust resource limits if needed:\n';
  report += '- If P99 > 5s: Increase CPU limits or reduce replicas\n';
  report += '- If error rate > 10%: Reduce concurrent VUs\n';
  report += '- If memory pressure detected: Reduce memory requests\n';

  return report;
}
