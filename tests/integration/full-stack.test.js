#!/usr/bin/env node
/**
 * HolmOS Integration Tests - Full Stack
 *
 * Tests complete user flows across multiple services
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();


async function testUserJourney(results) {
    console.log('\n  Testing complete user journey...');

    // Step 1: Login
    console.log('    Step 1: Login...');
    const loginResult = await utils.login();
    if (!loginResult.success) {
        results.fail('User journey - Login', `Login failed: ${loginResult.error}`);
        reporter.printResult({
            name: 'User journey - Login',
            status: 'fail',
            error: loginResult.error,
        });
        return;
    }
    results.pass('User journey - Login', { token: !!loginResult.token });
    reporter.printResult({
        name: 'User journey - Login',
        status: 'pass',
    });

    // Step 2: Validate token
    console.log('    Step 2: Validate token...');
    const validation = await utils.validateToken();
    if (!validation.valid) {
        results.fail('User journey - Token validation', validation.error);
        reporter.printResult({
            name: 'User journey - Token validation',
            status: 'fail',
        });
        return;
    }
    results.pass('User journey - Token validation', { username: validation.username });
    reporter.printResult({
        name: 'User journey - Token validation',
        status: 'pass',
    });

    // Step 3: Access file browser
    console.log('    Step 3: Access file browser...');
    try {
        const fileUrl = config.getServiceUrl('file-list', 8080, '/list?path=/');
        const fileResponse = await utils.authRequest('GET', fileUrl);
        if (fileResponse.status === 200) {
            results.pass('User journey - File access', {});
            reporter.printResult({
                name: 'User journey - File access',
                status: 'pass',
            });
        } else {
            results.fail('User journey - File access', `Status: ${fileResponse.status}`);
            reporter.printResult({
                name: 'User journey - File access',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('User journey - File access', error);
        reporter.printResult({
            name: 'User journey - File access',
            status: 'error',
            error: error.message,
        });
    }

    // Step 4: Check metrics dashboard
    console.log('    Step 4: Check metrics dashboard...');
    try {
        const metricsUrl = config.getServiceUrl('metrics-dashboard', 8080, '/api/cluster');
        const metricsResponse = await utils.client.get(metricsUrl);
        if (metricsResponse.status === 200) {
            results.pass('User journey - Metrics access', {
                nodes: metricsResponse.data?.total_nodes,
            });
            reporter.printResult({
                name: 'User journey - Metrics access',
                status: 'pass',
            });
        } else {
            results.fail('User journey - Metrics access', `Status: ${metricsResponse.status}`);
            reporter.printResult({
                name: 'User journey - Metrics access',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('User journey - Metrics access', error);
        reporter.printResult({
            name: 'User journey - Metrics access',
            status: 'error',
            error: error.message,
        });
    }

    // Step 5: Check notifications
    console.log('    Step 5: Check notifications...');
    try {
        const notifUrl = config.getServiceUrl('notification-hub', 8080, '/api/notifications');
        const notifResponse = await utils.client.get(notifUrl);
        if (notifResponse.status === 200) {
            results.pass('User journey - Notifications access', {
                count: Array.isArray(notifResponse.data) ? notifResponse.data.length : 0,
            });
            reporter.printResult({
                name: 'User journey - Notifications access',
                status: 'pass',
            });
        } else {
            results.fail('User journey - Notifications access', `Status: ${notifResponse.status}`);
            reporter.printResult({
                name: 'User journey - Notifications access',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('User journey - Notifications access', error);
        reporter.printResult({
            name: 'User journey - Notifications access',
            status: 'error',
            error: error.message,
        });
    }
}

async function testCoreServicesConnectivity(results) {
    console.log('\n  Testing core services connectivity...');

    const coreServices = [
        { name: 'auth-gateway', port: 8080, path: '/health' },
        { name: 'api-gateway', port: 8080, path: '/health' },
        { name: 'metrics-dashboard', port: 8080, path: '/health' },
        { name: 'notification-hub', port: 8080, path: '/health' },
        { name: 'test-dashboard', port: 8080, path: '/health' },
    ];

    let connectedCount = 0;
    for (const service of coreServices) {
        const result = await utils.healthCheck(service.name, service.port, service.path);
        if (result.status === 'pass') {
            connectedCount++;
        }
        reporter.printResult({
            name: `Core service: ${service.name}`,
            status: result.status,
            responseTime: result.responseTime,
        });
    }

    if (connectedCount === coreServices.length) {
        results.pass('All core services connected', {
            connected: connectedCount,
            total: coreServices.length,
        });
    } else {
        results.fail('All core services connected', `Only ${connectedCount}/${coreServices.length} connected`);
    }
}

async function testDataFlowAcrossServices(results) {
    console.log('\n  Testing data flow across services...');

    // Create a notification
    const testData = {
        type: 'info',
        title: `Integration Test ${utils.randomString(6)}`,
        message: 'Testing data flow across HolmOS services',
        priority: 'normal',
    };

    try {
        // Step 1: Send notification
        console.log('    Sending notification...');
        const notifUrl = config.getServiceUrl('notification-hub', 8080, '/api/notifications');
        const sendResponse = await utils.client.post(notifUrl, testData, {
            headers: { 'Content-Type': 'application/json' },
        });

        if (sendResponse.status !== 200 && sendResponse.status !== 201) {
            results.fail('Data flow - Send notification', `Status: ${sendResponse.status}`);
            reporter.printResult({
                name: 'Data flow - Send notification',
                status: 'fail',
            });
            return;
        }

        results.pass('Data flow - Send notification', { id: sendResponse.data?.id });
        reporter.printResult({
            name: 'Data flow - Send notification',
            status: 'pass',
        });

        // Step 2: Verify in stats
        console.log('    Verifying in stats...');
        await utils.sleep(500); // Wait for processing
        const statsUrl = config.getServiceUrl('notification-hub', 8080, '/api/stats');
        const statsResponse = await utils.client.get(statsUrl);

        if (statsResponse.status === 200 && statsResponse.data?.total > 0) {
            results.pass('Data flow - Stats updated', { total: statsResponse.data.total });
            reporter.printResult({
                name: 'Data flow - Stats updated',
                status: 'pass',
            });
        } else {
            results.fail('Data flow - Stats updated', 'Stats not reflecting new notification');
            reporter.printResult({
                name: 'Data flow - Stats updated',
                status: 'fail',
            });
        }

        // Step 3: Retrieve notification
        console.log('    Retrieving notifications...');
        const listUrl = config.getServiceUrl('notification-hub', 8080, '/api/notifications');
        const listResponse = await utils.client.get(listUrl);

        if (listResponse.status === 200 && Array.isArray(listResponse.data)) {
            const found = listResponse.data.some(n => n.title === testData.title);
            if (found) {
                results.pass('Data flow - Notification retrievable', {});
                reporter.printResult({
                    name: 'Data flow - Notification retrievable',
                    status: 'pass',
                });
            } else {
                results.fail('Data flow - Notification retrievable', 'Notification not found in list');
                reporter.printResult({
                    name: 'Data flow - Notification retrievable',
                    status: 'fail',
                });
            }
        } else {
            results.fail('Data flow - Notification retrievable', `Status: ${listResponse.status}`);
            reporter.printResult({
                name: 'Data flow - Notification retrievable',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('Data flow test', error);
        reporter.printResult({
            name: 'Data flow test',
            status: 'error',
            error: error.message,
        });
    }
}

async function testServiceDiscovery(results) {
    console.log('\n  Testing service discovery...');

    // Check if services can resolve each other via DNS
    const services = [
        'auth-gateway',
        'metrics-dashboard',
        'notification-hub',
        'file-list',
    ];

    let discoveredCount = 0;
    for (const svcName of services) {
        try {
            const url = config.getServiceUrl(svcName, 8080, '/health');
            const response = await utils.client.get(url, { timeout: 3000 });
            if (response.status >= 200 && response.status < 500) {
                discoveredCount++;
            }
        } catch (error) {
            // Service not discoverable
        }
    }

    if (discoveredCount === services.length) {
        results.pass('Service discovery working', {
            discovered: discoveredCount,
            total: services.length,
        });
    } else {
        results.fail('Service discovery working', `Only ${discoveredCount}/${services.length} discovered`);
    }

    reporter.printResult({
        name: 'Service discovery',
        status: discoveredCount === services.length ? 'pass' : 'fail',
    });
}

async function runFullStackIntegrationTests() {
    const results = new TestResults('Full Stack Integration');

    reporter.printHeader('Full Stack Integration Tests');
    console.log(`  Namespace: ${NAMESPACE}`);
    console.log(`  Cluster: ${config.cluster.host}`);

    await testUserJourney(results);
    await testCoreServicesConnectivity(results);
    await testDataFlowAcrossServices(results);
    await testServiceDiscovery(results);

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const json = args.includes('--json');

    const results = await runFullStackIntegrationTests();

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

module.exports = { runFullStackIntegrationTests };
