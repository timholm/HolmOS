// HolmOS Health Check Load Test
// Tests all service health endpoints under load

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { CONFIG, serviceUrl, getAllServices } from './config.js';

// Custom metrics
const healthCheckDuration = new Trend('health_check_duration', true);
const healthCheckFailures = new Counter('health_check_failures');
const healthCheckSuccess = new Rate('health_check_success');

// Test configuration
export const options = {
  scenarios: {
    // Continuous health monitoring
    health_monitor: {
      executor: 'constant-vus',
      vus: 5,
      duration: '2m',
    },
  },
  thresholds: {
    'health_check_duration': ['p(95)<500', 'p(99)<1000'],
    'health_check_success': ['rate>0.99'],
    'http_req_failed': ['rate<0.01'],
  },
  tags: {
    testType: 'health-check',
  },
};

export default function () {
  const services = getAllServices();

  for (const serviceName of services) {
    group(`health_${serviceName}`, function () {
      const url = serviceUrl(serviceName, '/health');
      const startTime = Date.now();

      const response = http.get(url, {
        timeout: '5s',
        tags: { service: serviceName, endpoint: 'health' },
      });

      const duration = Date.now() - startTime;
      healthCheckDuration.add(duration, { service: serviceName });

      const isSuccess = check(response, {
        'status is 200': (r) => r.status === 200,
        'response time < 500ms': (r) => r.timings.duration < 500,
        'body contains health indicator': (r) =>
          r.body && (r.body.includes('ok') || r.body.includes('healthy') || r.body.includes('status')),
      });

      if (isSuccess) {
        healthCheckSuccess.add(1, { service: serviceName });
      } else {
        healthCheckFailures.add(1, { service: serviceName });
        healthCheckSuccess.add(0, { service: serviceName });
      }
    });

    // Small delay between service checks
    sleep(0.1);
  }

  // Pause between full health check rounds
  sleep(1);
}

export function handleSummary(data) {
  return {
    'stdout': generateHealthReport(data),
    'results/health-check-summary.json': JSON.stringify(data, null, 2),
  };
}

function generateHealthReport(data) {
  let report = '\n=== HolmOS Health Check Load Test Report ===\n\n';

  // Overall stats
  const totalChecks = data.metrics.http_reqs ? data.metrics.http_reqs.values.count : 0;
  const failedRate = data.metrics.http_req_failed ? data.metrics.http_req_failed.values.rate : 0;

  report += `Total Health Checks: ${totalChecks}\n`;
  report += `Success Rate: ${((1 - failedRate) * 100).toFixed(2)}%\n`;
  report += `Failed Rate: ${(failedRate * 100).toFixed(2)}%\n\n`;

  // Duration stats
  if (data.metrics.health_check_duration) {
    const duration = data.metrics.health_check_duration.values;
    report += `Response Times:\n`;
    report += `  Average: ${duration.avg?.toFixed(2) || 'N/A'}ms\n`;
    report += `  P95: ${duration['p(95)']?.toFixed(2) || 'N/A'}ms\n`;
    report += `  P99: ${duration['p(99)']?.toFixed(2) || 'N/A'}ms\n`;
    report += `  Max: ${duration.max?.toFixed(2) || 'N/A'}ms\n`;
  }

  report += '\n';

  // Threshold results
  report += 'Threshold Results:\n';
  for (const [name, threshold] of Object.entries(data.thresholds || {})) {
    const status = threshold.ok ? 'PASS' : 'FAIL';
    report += `  ${name}: ${status}\n`;
  }

  return report;
}
