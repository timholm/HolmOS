// HolmOS Spike Test
// Test system behavior under sudden traffic bursts

import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { CONFIG, serviceUrl } from './config.js';

// Custom metrics
const spikeLatency = new Trend('spike_latency', true);
const preSpikeLantency = new Trend('pre_spike_latency', true);
const duringSpikeLantency = new Trend('during_spike_latency', true);
const postSpikeLantency = new Trend('post_spike_latency', true);
const spikeErrors = new Counter('spike_errors');
const spikeSuccess = new Rate('spike_success');
const recoveryTime = new Trend('recovery_time', true);

// Critical services to test for spike resilience
const CRITICAL_SERVICES = [
  'holmos-shell',
  'gateway',
  'pulse',
  'chat-hub',
];

export const options = {
  scenarios: {
    spike: {
      executor: 'ramping-vus',
      stages: CONFIG.loadStages.spike,
      gracefulRampDown: '30s',
    },
  },
  thresholds: {
    'spike_latency': ['p(95)<5000'],
    'pre_spike_latency': ['p(95)<1000'],
    'during_spike_latency': ['p(95)<8000'],  // Higher during spike
    'post_spike_latency': ['p(95)<2000'],     // Should recover
    'spike_success': ['rate>0.85'],
    'http_req_failed': ['rate<0.15'],
  },
  tags: {
    testType: 'spike',
  },
};

// Track which phase we're in based on VU count and time
let testStartTime = null;

export function setup() {
  return { startTime: Date.now() };
}

export default function (data) {
  if (!testStartTime) {
    testStartTime = data.startTime;
  }

  const elapsedSeconds = (Date.now() - data.startTime) / 1000;

  // Determine phase based on time
  // Phase 1: Pre-spike (0-30s, ~5 VUs)
  // Phase 2: Spike ramp (30-40s, 5->50 VUs)
  // Phase 3: During spike (40-100s, 50 VUs)
  // Phase 4: Spike drop (100-110s, 50->5 VUs)
  // Phase 5: Post-spike recovery (110-140s, ~5 VUs)
  let phase = 'pre_spike';
  let latencyMetric = preSpikeLantency;

  if (elapsedSeconds > 110) {
    phase = 'post_spike';
    latencyMetric = postSpikeLantency;
  } else if (elapsedSeconds > 40) {
    phase = 'during_spike';
    latencyMetric = duringSpikeLantency;
  }

  for (const serviceName of CRITICAL_SERVICES) {
    group(`spike_${serviceName}`, function () {
      const healthUrl = serviceUrl(serviceName, '/health');

      const response = http.get(healthUrl, {
        timeout: '15s',
        tags: {
          service: serviceName,
          phase: phase,
        },
      });

      // Track latency by phase
      latencyMetric.add(response.timings.duration, {
        service: serviceName,
      });
      spikeLatency.add(response.timings.duration, {
        service: serviceName,
        phase: phase,
      });

      const success = check(response, {
        'status is 2xx': (r) => r.status >= 200 && r.status < 300,
        'response time under limit': (r) => {
          // Different limits based on phase
          if (phase === 'during_spike') {
            return r.timings.duration < 10000;  // 10s during spike
          }
          return r.timings.duration < 3000;  // 3s otherwise
        },
      });

      if (success) {
        spikeSuccess.add(1, { service: serviceName, phase: phase });
      } else {
        spikeErrors.add(1, { service: serviceName, phase: phase });
        spikeSuccess.add(0, { service: serviceName, phase: phase });
      }
    });

    sleep(0.05);
  }

  sleep(0.1);
}

export function handleSummary(data) {
  return {
    'stdout': generateSpikeReport(data),
    'results/spike-summary.json': JSON.stringify(data, null, 2),
  };
}

function generateSpikeReport(data) {
  let report = '\n=== HolmOS Spike Test Report ===\n\n';
  report += 'Testing system resilience to sudden traffic bursts\n\n';

  // Phase-by-phase analysis
  const phases = [
    { name: 'Pre-Spike (Baseline)', metric: 'pre_spike_latency' },
    { name: 'During Spike (High Load)', metric: 'during_spike_latency' },
    { name: 'Post-Spike (Recovery)', metric: 'post_spike_latency' },
  ];

  for (const phase of phases) {
    const m = data.metrics[phase.metric];
    if (m) {
      report += `${phase.name}:\n`;
      report += `  Average: ${m.values.avg?.toFixed(2) || 'N/A'}ms\n`;
      report += `  P95: ${m.values['p(95)']?.toFixed(2) || 'N/A'}ms\n`;
      report += `  P99: ${m.values['p(99)']?.toFixed(2) || 'N/A'}ms\n`;
      report += `  Max: ${m.values.max?.toFixed(2) || 'N/A'}ms\n\n`;
    }
  }

  // Calculate degradation factor
  if (data.metrics.pre_spike_latency && data.metrics.during_spike_latency) {
    const baseline = data.metrics.pre_spike_latency.values.avg || 1;
    const spike = data.metrics.during_spike_latency.values.avg || baseline;
    const degradation = spike / baseline;
    report += `Performance Degradation During Spike: ${degradation.toFixed(2)}x\n`;
  }

  // Calculate recovery
  if (data.metrics.pre_spike_latency && data.metrics.post_spike_latency) {
    const baseline = data.metrics.pre_spike_latency.values.avg || 1;
    const recovery = data.metrics.post_spike_latency.values.avg || baseline;
    const recoveryRatio = recovery / baseline;
    report += `Recovery Ratio (should be ~1.0): ${recoveryRatio.toFixed(2)}\n`;

    if (recoveryRatio > 1.5) {
      report += '  WARNING: System may not be fully recovering from spikes\n';
    } else {
      report += '  OK: System recovers well after traffic spikes\n';
    }
  }

  report += '\n';

  // Overall stats
  if (data.metrics.spike_success) {
    report += `Overall Success Rate: ${(data.metrics.spike_success.values.rate * 100).toFixed(2)}%\n`;
  }

  if (data.metrics.spike_errors) {
    report += `Total Errors: ${data.metrics.spike_errors.values.count}\n`;
  }

  // Threshold results
  report += '\nThreshold Results:\n';
  for (const [name, threshold] of Object.entries(data.thresholds || {})) {
    const status = threshold.ok ? 'PASS' : 'FAIL';
    report += `  ${name}: ${status}\n`;
  }

  report += '\n=== Recommendations for Raspberry Pi ===\n';
  report += '- Consider HPA (Horizontal Pod Autoscaler) for critical services\n';
  report += '- Gateway service should scale to 3+ replicas for spike handling\n';
  report += '- Implement request queuing for burst protection\n';

  return report;
}
