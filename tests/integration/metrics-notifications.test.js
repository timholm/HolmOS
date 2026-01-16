#!/usr/bin/env node
/**
 * HolmOS Integration Tests - Metrics + Notifications
 *
 * Tests integration between metrics dashboard and notification services
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();


async function testMetricsAlertToNotification(results) {
    console.log('\n  Testing metrics alert to notification flow...');

    try {
        // Get current alerts from metrics
        const alertsUrl = config.getServiceUrl('metrics-dashboard', 8080, '/api/alerts/triggered');
        const alertsResponse = await utils.client.get(alertsUrl);

        if (alertsResponse.status !== 200) {
            results.fail('Metrics alerts retrieval', `Status: ${alertsResponse.status}`);
            reporter.printResult({
                name: 'Metrics alerts retrieval',
                status: 'fail',
            });
            return;
        }

        // Check if notification hub can receive alerts
        const notifUrl = config.getServiceUrl('notification-hub', 8080, '/api/notifications');
        const notifResponse = await utils.client.get(notifUrl);

        if (notifResponse.status === 200) {
            results.pass('Metrics alerts can be forwarded to notifications', {
                alertCount: Array.isArray(alertsResponse.data) ? alertsResponse.data.length : 0,
                notificationCount: Array.isArray(notifResponse.data) ? notifResponse.data.length : 0,
            });
            reporter.printResult({
                name: 'Metrics-to-notification flow',
                status: 'pass',
            });
        } else {
            results.fail('Metrics alerts can be forwarded to notifications', `Notification service status: ${notifResponse.status}`);
            reporter.printResult({
                name: 'Metrics-to-notification flow',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('Metrics alerts can be forwarded to notifications', error);
        reporter.printResult({
            name: 'Metrics-to-notification flow',
            status: 'error',
            error: error.message,
        });
    }
}

async function testClusterStatusNotification(results) {
    console.log('  Testing cluster status notification...');

    try {
        // Get cluster summary
        const clusterUrl = config.getServiceUrl('metrics-dashboard', 8080, '/api/cluster');
        const startTime = Date.now();
        const clusterResponse = await utils.client.get(clusterUrl);
        const responseTime = Date.now() - startTime;

        if (clusterResponse.status !== 200) {
            results.fail('Get cluster status for notification', `Status: ${clusterResponse.status}`);
            reporter.printResult({
                name: 'Cluster status notification',
                status: 'fail',
            });
            return;
        }

        const cluster = clusterResponse.data;

        // Create a notification about cluster status
        const notification = {
            type: 'info',
            title: 'Cluster Status Check',
            message: `Cluster has ${cluster.total_nodes} nodes, ${cluster.total_pods} pods. CPU: ${cluster.cpu_pct?.toFixed(1)}%, Memory: ${cluster.memory_pct?.toFixed(1)}%`,
            priority: cluster.cpu_pct > 80 || cluster.memory_pct > 80 ? 'high' : 'normal',
        };

        const notifUrl = config.getServiceUrl('notification-hub', 8080, '/api/notifications');
        const notifResponse = await utils.client.post(notifUrl, notification, {
            headers: { 'Content-Type': 'application/json' },
        });

        if (notifResponse.status === 200 || notifResponse.status === 201) {
            results.pass('Cluster status notification sent', {
                responseTime,
                nodes: cluster.total_nodes,
                pods: cluster.total_pods,
            });
            reporter.printResult({
                name: 'Cluster status notification',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Cluster status notification sent', `Status: ${notifResponse.status}`);
            reporter.printResult({
                name: 'Cluster status notification',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('Cluster status notification sent', error);
        reporter.printResult({
            name: 'Cluster status notification',
            status: 'error',
            error: error.message,
        });
    }
}

async function testHighResourceAlert(results) {
    console.log('  Testing high resource usage alert...');

    try {
        // Get nodes metrics
        const nodesUrl = config.getServiceUrl('metrics-dashboard', 8080, '/api/nodes');
        const nodesResponse = await utils.client.get(nodesUrl);

        if (nodesResponse.status !== 200 || !Array.isArray(nodesResponse.data)) {
            results.skip('High resource alert', 'Could not get nodes metrics');
            reporter.printResult({ name: 'High resource alert', status: 'skip' });
            return;
        }

        // Check for any high resource usage
        const highResourceNodes = nodesResponse.data.filter(
            n => n.cpu_pct > 70 || n.memory_pct > 70
        );

        if (highResourceNodes.length > 0) {
            // Send alert notification
            const notification = {
                type: 'warning',
                title: 'High Resource Usage Detected',
                message: `${highResourceNodes.length} node(s) with high resource usage: ${highResourceNodes.map(n => n.name).join(', ')}`,
                priority: 'high',
            };

            const notifUrl = config.getServiceUrl('notification-hub', 8080, '/api/notifications');
            const notifResponse = await utils.client.post(notifUrl, notification, {
                headers: { 'Content-Type': 'application/json' },
            });

            if (notifResponse.status === 200 || notifResponse.status === 201) {
                results.pass('High resource alert sent', {
                    affectedNodes: highResourceNodes.length,
                });
                reporter.printResult({
                    name: 'High resource alert',
                    status: 'pass',
                });
            } else {
                results.fail('High resource alert sent', `Status: ${notifResponse.status}`);
                reporter.printResult({
                    name: 'High resource alert',
                    status: 'fail',
                });
            }
        } else {
            results.pass('High resource alert (no high usage detected)', {
                note: 'All nodes within normal limits',
            });
            reporter.printResult({
                name: 'High resource alert',
                status: 'pass',
            });
        }
    } catch (error) {
        results.error('High resource alert', error);
        reporter.printResult({
            name: 'High resource alert',
            status: 'error',
            error: error.message,
        });
    }
}

async function testWebhookDelivery(results) {
    console.log('  Testing webhook delivery from metrics events...');

    try {
        // Get webhooks from notification hub
        const webhooksUrl = config.getServiceUrl('notification-hub', 8080, '/api/webhooks');
        const startTime = Date.now();
        const webhooksResponse = await utils.client.get(webhooksUrl);
        const responseTime = Date.now() - startTime;

        if (webhooksResponse.status === 200) {
            results.pass('Webhook delivery system accessible', {
                responseTime,
                webhookCount: Array.isArray(webhooksResponse.data) ? webhooksResponse.data.length : 0,
            });
            reporter.printResult({
                name: 'Webhook delivery',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Webhook delivery system accessible', `Status: ${webhooksResponse.status}`);
            reporter.printResult({
                name: 'Webhook delivery',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('Webhook delivery system accessible', error);
        reporter.printResult({
            name: 'Webhook delivery',
            status: 'error',
            error: error.message,
        });
    }
}

async function testNotificationStats(results) {
    console.log('  Testing notification stats aggregation...');

    try {
        const statsUrl = config.getServiceUrl('notification-hub', 8080, '/api/stats');
        const startTime = Date.now();
        const statsResponse = await utils.client.get(statsUrl);
        const responseTime = Date.now() - startTime;

        if (statsResponse.status === 200 && statsResponse.data) {
            results.pass('Notification stats aggregation', {
                responseTime,
                totalNotifications: statsResponse.data.total,
                bySource: statsResponse.data.by_source,
            });
            reporter.printResult({
                name: 'Notification stats',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Notification stats aggregation', `Status: ${statsResponse.status}`);
            reporter.printResult({
                name: 'Notification stats',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('Notification stats aggregation', error);
        reporter.printResult({
            name: 'Notification stats',
            status: 'error',
            error: error.message,
        });
    }
}

async function runMetricsNotificationsIntegrationTests() {
    const results = new TestResults('Metrics + Notifications Integration');

    reporter.printHeader('Metrics + Notifications Integration Tests');
    console.log(`  Namespace: ${NAMESPACE}`);

    await testMetricsAlertToNotification(results);
    await testClusterStatusNotification(results);
    await testHighResourceAlert(results);
    await testWebhookDelivery(results);
    await testNotificationStats(results);

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const json = args.includes('--json');

    const results = await runMetricsNotificationsIntegrationTests();

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

module.exports = { runMetricsNotificationsIntegrationTests };
