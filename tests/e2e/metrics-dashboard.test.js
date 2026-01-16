#!/usr/bin/env node
/**
 * HolmOS E2E Metrics Dashboard Tests
 *
 * Tests cluster metrics and monitoring functionality
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();

// Use NodePort URL for external access
const METRICS_BASE = config.getServiceUrl('metrics-dashboard', 8080, '').replace(/\/$/, '');

async function testHealthEndpoint(results) {
    console.log('\n  Testing health endpoint...');

    const result = await utils.healthCheck('metrics-dashboard', 8080, '/health');

    if (result.status === 'pass') {
        results.pass('Health endpoint returns OK', { responseTime: result.responseTime });
    } else {
        results.fail('Health endpoint returns OK', result.error);
    }

    reporter.printResult({
        name: 'Health endpoint',
        status: result.status,
        responseTime: result.responseTime,
    });
}

async function testNodesEndpoint(results) {
    console.log('  Testing nodes metrics...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/nodes`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200 && Array.isArray(response.data)) {
            results.pass('Get nodes metrics', {
                responseTime,
                nodeCount: response.data.length,
            });
            reporter.printResult({
                name: 'Nodes metrics',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get nodes metrics', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Nodes metrics',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get nodes metrics', error);
        reporter.printResult({
            name: 'Nodes metrics',
            status: 'error',
            error: error.message,
        });
    }
}

async function testPodsEndpoint(results) {
    console.log('  Testing pods metrics...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/pods`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            const podCount = Array.isArray(response.data) ? response.data.length : 0;
            results.pass('Get pods metrics', { responseTime, podCount });
            reporter.printResult({
                name: 'Pods metrics',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get pods metrics', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Pods metrics',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get pods metrics', error);
        reporter.printResult({
            name: 'Pods metrics',
            status: 'error',
            error: error.message,
        });
    }
}

async function testPodsFilterByNamespace(results) {
    console.log('  Testing pods metrics filtered by namespace...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/pods?namespace=${NAMESPACE}`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Get pods metrics by namespace', { responseTime });
            reporter.printResult({
                name: 'Pods metrics (filtered)',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get pods metrics by namespace', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Pods metrics (filtered)',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get pods metrics by namespace', error);
        reporter.printResult({
            name: 'Pods metrics (filtered)',
            status: 'error',
            error: error.message,
        });
    }
}

async function testDeploymentsEndpoint(results) {
    console.log('  Testing deployments metrics...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/deployments`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            const deploymentCount = Array.isArray(response.data) ? response.data.length : 0;
            results.pass('Get deployments metrics', { responseTime, deploymentCount });
            reporter.printResult({
                name: 'Deployments metrics',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get deployments metrics', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Deployments metrics',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get deployments metrics', error);
        reporter.printResult({
            name: 'Deployments metrics',
            status: 'error',
            error: error.message,
        });
    }
}

async function testClusterSummary(results) {
    console.log('  Testing cluster summary...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/cluster`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200 && response.data) {
            const summary = response.data;
            results.pass('Get cluster summary', {
                responseTime,
                totalNodes: summary.total_nodes,
                totalPods: summary.total_pods,
                cpuPercent: summary.cpu_pct,
                memoryPercent: summary.memory_pct,
            });
            reporter.printResult({
                name: 'Cluster summary',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get cluster summary', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Cluster summary',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get cluster summary', error);
        reporter.printResult({
            name: 'Cluster summary',
            status: 'error',
            error: error.message,
        });
    }
}

async function testHistoryEndpoint(results) {
    console.log('  Testing metrics history...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/history?metric=cluster_cpu&range=1h`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Get metrics history', {
                responseTime,
                dataPoints: Array.isArray(response.data) ? response.data.length : 0,
            });
            reporter.printResult({
                name: 'Metrics history',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get metrics history', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Metrics history',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get metrics history', error);
        reporter.printResult({
            name: 'Metrics history',
            status: 'error',
            error: error.message,
        });
    }
}

async function testAlertRules(results) {
    console.log('  Testing alert rules...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/alerts/rules`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Get alert rules', {
                responseTime,
                ruleCount: Array.isArray(response.data) ? response.data.length : 0,
            });
            reporter.printResult({
                name: 'Alert rules',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get alert rules', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Alert rules',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get alert rules', error);
        reporter.printResult({
            name: 'Alert rules',
            status: 'error',
            error: error.message,
        });
    }
}

async function testTriggeredAlerts(results) {
    console.log('  Testing triggered alerts...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/alerts/triggered`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Get triggered alerts', {
                responseTime,
                alertCount: Array.isArray(response.data) ? response.data.length : 0,
            });
            reporter.printResult({
                name: 'Triggered alerts',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get triggered alerts', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Triggered alerts',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get triggered alerts', error);
        reporter.printResult({
            name: 'Triggered alerts',
            status: 'error',
            error: error.message,
        });
    }
}

async function testLegacyMetrics(results) {
    console.log('  Testing legacy metrics endpoint...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/api/metrics`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200 && response.data) {
            results.pass('Get legacy metrics', {
                responseTime,
                hasCpu: !!response.data.cpu,
                hasMemory: !!response.data.memory,
            });
            reporter.printResult({
                name: 'Legacy metrics',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get legacy metrics', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Legacy metrics',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get legacy metrics', error);
        reporter.printResult({
            name: 'Legacy metrics',
            status: 'error',
            error: error.message,
        });
    }
}

async function testDashboardUI(results) {
    console.log('  Testing dashboard UI...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${METRICS_BASE}/`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            const hasHtml = typeof response.data === 'string' &&
                (response.data.includes('<!DOCTYPE') || response.data.includes('<html'));
            results.pass('Dashboard UI accessible', { responseTime, hasHtml });
            reporter.printResult({
                name: 'Dashboard UI',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Dashboard UI accessible', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Dashboard UI',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Dashboard UI accessible', error);
        reporter.printResult({
            name: 'Dashboard UI',
            status: 'error',
            error: error.message,
        });
    }
}

async function runMetricsDashboardTests() {
    const results = new TestResults('Metrics Dashboard');

    reporter.printHeader('Metrics Dashboard E2E Tests');
    console.log(`  Target: ${METRICS_BASE}`);

    // Run tests
    await testHealthEndpoint(results);
    await testNodesEndpoint(results);
    await testPodsEndpoint(results);
    await testPodsFilterByNamespace(results);
    await testDeploymentsEndpoint(results);
    await testClusterSummary(results);
    await testHistoryEndpoint(results);
    await testAlertRules(results);
    await testTriggeredAlerts(results);
    await testLegacyMetrics(results);
    await testDashboardUI(results);

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const json = args.includes('--json');

    const results = await runMetricsDashboardTests();

    if (json) {
        console.log(JSON.stringify(results.toJSON(), null, 2));
    }

    const summary = results.getSummary();
    process.exit(summary.failed + summary.errors > 0 ? 1 : 0);
}

if (require.main === module) {
    main().catch(err => {
        console.error('Test execution failed:', err);
        process.exit(1);
    });
}

module.exports = { runMetricsDashboardTests };
