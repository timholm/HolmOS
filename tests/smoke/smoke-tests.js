#!/usr/bin/env node
/**
 * HolmOS Smoke Tests
 *
 * Minimal smoke tests to verify core system functionality:
 * - Shell loads (iOS shell / claude-pod at port 30001)
 * - App routes open (service health checks)
 * - Cluster API responds (Nova or k8s API)
 *
 * Usage:
 *   node smoke-tests.js              # Run all smoke tests
 *   node smoke-tests.js --json       # Output as JSON
 *   node smoke-tests.js --offline    # Run in offline/degraded mode
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();

// Smoke test configuration with timeouts suitable for reduced connectivity
const SMOKE_CONFIG = {
    // Short timeout for smoke tests (3 seconds)
    timeout: parseInt(process.env.SMOKE_TIMEOUT) || 3000,
    // Retry count for flaky connections
    retries: parseInt(process.env.SMOKE_RETRIES) || 2,
    // Allow degraded mode where some failures are acceptable
    allowDegraded: process.env.SMOKE_ALLOW_DEGRADED === 'true' || false,
};

// Core services that must pass for smoke test to succeed
const CRITICAL_SERVICES = [
    { name: 'claude-pod', port: 30001, path: '/health', description: 'iOS shell / AI chat interface' },
];

// Important services that should pass but won't fail the entire smoke test
const IMPORTANT_SERVICES = [
    { name: 'holmos-shell', port: 30000, path: '/health', description: 'Main HolmOS shell' },
    { name: 'app-store', port: 30002, path: '/health', description: 'App store' },
    { name: 'gateway', port: 30080, path: '/health', description: 'API gateway' },
];

// Cluster API services
const CLUSTER_SERVICES = [
    { name: 'nova', port: 30004, path: '/health', description: 'Nova AI orchestrator' },
    { name: 'cluster-manager', port: 30502, path: '/health', description: 'Cluster manager' },
];

/**
 * Check if a service is healthy with retries
 */
async function checkServiceWithRetry(service, retries = SMOKE_CONFIG.retries) {
    const url = `http://${config.cluster.host}:${service.port}${service.path}`;
    let lastError = null;

    for (let attempt = 0; attempt <= retries; attempt++) {
        try {
            const startTime = Date.now();
            const response = await utils.client.get(url, {
                timeout: SMOKE_CONFIG.timeout,
            });
            const responseTime = Date.now() - startTime;

            if (response.status >= 200 && response.status < 300) {
                return {
                    service: service.name,
                    status: 'pass',
                    statusCode: response.status,
                    responseTime,
                    description: service.description,
                };
            } else {
                lastError = `HTTP ${response.status}`;
            }
        } catch (error) {
            lastError = error.code || error.message;
            // Wait before retry (exponential backoff)
            if (attempt < retries) {
                await utils.sleep(Math.min(1000 * Math.pow(2, attempt), 3000));
            }
        }
    }

    return {
        service: service.name,
        status: 'fail',
        statusCode: 0,
        responseTime: 0,
        error: lastError,
        description: service.description,
    };
}

/**
 * Test 1: Shell Loads (iOS shell at port 30001)
 */
async function testShellLoads(results) {
    console.log('\n  [Test 1] Shell Loads (iOS shell / claude-pod)');
    console.log('  ' + '-'.repeat(40));

    for (const service of CRITICAL_SERVICES) {
        const result = await checkServiceWithRetry(service);

        if (result.status === 'pass') {
            results.pass(`Shell: ${service.name}`, {
                responseTime: result.responseTime,
                statusCode: result.statusCode,
            });
            reporter.printResult({
                name: `${service.name} - ${service.description}`,
                status: 'pass',
                responseTime: result.responseTime,
            });
        } else {
            results.fail(`Shell: ${service.name}`, result.error, {
                description: service.description,
            });
            reporter.printResult({
                name: `${service.name} - ${service.description}`,
                status: 'fail',
                error: result.error,
            });
        }
    }
}

/**
 * Test 2: App Routes Open (service health checks)
 */
async function testAppRoutesOpen(results) {
    console.log('\n  [Test 2] App Routes Open (service health checks)');
    console.log('  ' + '-'.repeat(40));

    // Run checks in parallel for speed
    const checks = IMPORTANT_SERVICES.map(service => checkServiceWithRetry(service));
    const checkResults = await Promise.all(checks);

    for (const result of checkResults) {
        if (result.status === 'pass') {
            results.pass(`App: ${result.service}`, {
                responseTime: result.responseTime,
                statusCode: result.statusCode,
            });
        } else {
            // Important services failing is a warning, not critical failure
            results.fail(`App: ${result.service}`, result.error, {
                description: result.description,
            });
        }

        reporter.printResult({
            name: `${result.service} - ${result.description}`,
            status: result.status,
            responseTime: result.responseTime,
            error: result.error,
        });
    }
}

/**
 * Test 3: Cluster API Responds (Nova or k8s API)
 */
async function testClusterAPIResponds(results) {
    console.log('\n  [Test 3] Cluster API Responds (Nova / Cluster Manager)');
    console.log('  ' + '-'.repeat(40));

    let anyClusterServiceUp = false;

    // Run checks in parallel
    const checks = CLUSTER_SERVICES.map(service => checkServiceWithRetry(service));
    const checkResults = await Promise.all(checks);

    for (const result of checkResults) {
        if (result.status === 'pass') {
            anyClusterServiceUp = true;
            results.pass(`Cluster: ${result.service}`, {
                responseTime: result.responseTime,
                statusCode: result.statusCode,
            });
        } else {
            results.fail(`Cluster: ${result.service}`, result.error, {
                description: result.description,
            });
        }

        reporter.printResult({
            name: `${result.service} - ${result.description}`,
            status: result.status,
            responseTime: result.responseTime,
            error: result.error,
        });
    }

    // At least one cluster service should be up
    if (anyClusterServiceUp) {
        results.pass('Cluster API: At least one service responding');
    } else if (SMOKE_CONFIG.allowDegraded) {
        console.log('  \x1b[33m[WARN]\x1b[0m All cluster services down but degraded mode allowed');
    }

    return anyClusterServiceUp;
}

/**
 * Run all smoke tests
 */
async function runSmokeTests(options = {}) {
    const results = new TestResults('Smoke Tests');
    const allowDegraded = options.allowDegraded || SMOKE_CONFIG.allowDegraded;

    reporter.printHeader('HolmOS Smoke Tests');
    console.log(`  Cluster: ${config.cluster.host}`);
    console.log(`  Timeout: ${SMOKE_CONFIG.timeout}ms`);
    console.log(`  Retries: ${SMOKE_CONFIG.retries}`);
    console.log(`  Mode: ${allowDegraded ? 'Degraded (partial failures OK)' : 'Strict'}`);

    // Track critical failures
    let criticalFailures = 0;

    // Test 1: Shell loads (critical)
    await testShellLoads(results);
    const shellTests = results.tests.filter(t => t.name.startsWith('Shell:'));
    const shellFailures = shellTests.filter(t => t.status === 'fail').length;
    criticalFailures += shellFailures;

    // Test 2: App routes open (important but not critical)
    await testAppRoutesOpen(results);

    // Test 3: Cluster API responds
    const clusterUp = await testClusterAPIResponds(results);
    if (!clusterUp && !allowDegraded) {
        criticalFailures++;
    }

    results.finish();

    // Print summary
    reporter.printSummary(results.getSummary());

    // Determine exit status
    const summary = results.getSummary();

    if (allowDegraded) {
        // In degraded mode, only shell failures are critical
        console.log(`  Critical failures: ${criticalFailures}`);
        if (shellFailures > 0) {
            console.log('\x1b[31m  SMOKE TEST FAILED: Shell service not responding\x1b[0m');
        } else {
            console.log('\x1b[32m  SMOKE TEST PASSED (degraded mode)\x1b[0m');
        }
    } else {
        if (summary.failed > 0) {
            console.log('\x1b[31m  SMOKE TEST FAILED\x1b[0m');
        } else {
            console.log('\x1b[32m  SMOKE TEST PASSED\x1b[0m');
        }
    }

    return {
        results,
        criticalFailures,
        shellFailures,
        success: allowDegraded ? shellFailures === 0 : summary.failed === 0,
    };
}

/**
 * Quick connectivity check (single service)
 */
async function quickCheck() {
    const service = CRITICAL_SERVICES[0]; // claude-pod
    const result = await checkServiceWithRetry(service, 1);
    return result.status === 'pass';
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const json = args.includes('--json');
    const offline = args.includes('--offline') || args.includes('--degraded');
    const quick = args.includes('--quick');

    if (quick) {
        const isUp = await quickCheck();
        if (json) {
            console.log(JSON.stringify({ status: isUp ? 'pass' : 'fail' }));
        } else {
            console.log(isUp ? 'OK' : 'FAIL');
        }
        process.exit(isUp ? 0 : 1);
    }

    const { results, success } = await runSmokeTests({
        allowDegraded: offline,
    });

    if (json) {
        console.log(JSON.stringify(results.toJSON(), null, 2));
    }

    process.exit(success ? 0 : 1);
}

// Run if called directly
if (require.main === module) {
    main().catch(err => {
        console.error('Smoke test execution failed:', err);
        process.exit(1);
    });
}

module.exports = {
    runSmokeTests,
    quickCheck,
    checkServiceWithRetry,
    CRITICAL_SERVICES,
    IMPORTANT_SERVICES,
    CLUSTER_SERVICES,
};
