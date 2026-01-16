#!/usr/bin/env node
/**
 * HolmOS E2E Notification Hub Tests
 *
 * Tests notification management functionality
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();

// Use NodePort URL for external access
const NOTIF_BASE = config.getServiceUrl('notification-hub', 8080, '').replace(/\/$/, '');

async function testHealthEndpoint(results) {
    console.log('\n  Testing health endpoint...');

    const result = await utils.healthCheck('notification-hub', 8080, '/health');

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

async function testGetNotifications(results) {
    console.log('  Testing get notifications...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${NOTIF_BASE}/api/notifications`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Get notifications', {
                responseTime,
                count: Array.isArray(response.data) ? response.data.length : 0,
            });
            reporter.printResult({
                name: 'Get notifications',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get notifications', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Get notifications',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get notifications', error);
        reporter.printResult({
            name: 'Get notifications',
            status: 'error',
            error: error.message,
        });
    }
}

async function testFilterNotifications(results) {
    console.log('  Testing filter notifications by source...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${NOTIF_BASE}/api/notifications?source=hub`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Filter notifications by source', { responseTime });
            reporter.printResult({
                name: 'Filter notifications',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Filter notifications by source', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Filter notifications',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Filter notifications by source', error);
        reporter.printResult({
            name: 'Filter notifications',
            status: 'error',
            error: error.message,
        });
    }
}

async function testSendNotification(results) {
    console.log('  Testing send notification...');

    const testNotification = utils.generateTestData('notification');

    try {
        const startTime = Date.now();
        const response = await utils.client.post(`${NOTIF_BASE}/api/notifications`, testNotification, {
            headers: { 'Content-Type': 'application/json' },
        });
        const responseTime = Date.now() - startTime;

        if (response.status === 201 || response.status === 200) {
            results.pass('Send notification', {
                responseTime,
                title: testNotification.title,
                hasId: !!response.data.id,
            });
            reporter.printResult({
                name: 'Send notification',
                status: 'pass',
                responseTime,
            });
            return response.data.id;
        } else {
            results.fail('Send notification', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Send notification',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
            return null;
        }
    } catch (error) {
        results.error('Send notification', error);
        reporter.printResult({
            name: 'Send notification',
            status: 'error',
            error: error.message,
        });
        return null;
    }
}

async function testGetStats(results) {
    console.log('  Testing get stats...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${NOTIF_BASE}/api/stats`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200 && response.data) {
            results.pass('Get notification stats', {
                responseTime,
                total: response.data.total,
                bySource: response.data.by_source,
            });
            reporter.printResult({
                name: 'Get stats',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get notification stats', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Get stats',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get notification stats', error);
        reporter.printResult({
            name: 'Get stats',
            status: 'error',
            error: error.message,
        });
    }
}

async function testRefresh(results) {
    console.log('  Testing refresh from services...');

    try {
        const startTime = Date.now();
        const response = await utils.client.post(`${NOTIF_BASE}/api/refresh`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Refresh from services', { responseTime });
            reporter.printResult({
                name: 'Refresh from services',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Refresh from services', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Refresh from services',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Refresh from services', error);
        reporter.printResult({
            name: 'Refresh from services',
            status: 'error',
            error: error.message,
        });
    }
}

async function testGetWebhooks(results) {
    console.log('  Testing get webhooks...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${NOTIF_BASE}/api/webhooks`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Get webhooks', {
                responseTime,
                count: Array.isArray(response.data) ? response.data.length : 0,
            });
            reporter.printResult({
                name: 'Get webhooks',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get webhooks', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Get webhooks',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get webhooks', error);
        reporter.printResult({
            name: 'Get webhooks',
            status: 'error',
            error: error.message,
        });
    }
}

async function testCreateWebhook(results) {
    console.log('  Testing create webhook...');

    const testWebhook = {
        name: `Test Webhook ${utils.randomString(6)}`,
        url: 'https://httpbin.org/post',
        events: ['info', 'error'],
    };

    try {
        const startTime = Date.now();
        const response = await utils.client.post(`${NOTIF_BASE}/api/webhooks`, testWebhook, {
            headers: { 'Content-Type': 'application/json' },
        });
        const responseTime = Date.now() - startTime;

        if (response.status === 201 || response.status === 200) {
            results.pass('Create webhook', {
                responseTime,
                name: testWebhook.name,
                id: response.data.id,
            });
            reporter.printResult({
                name: 'Create webhook',
                status: 'pass',
                responseTime,
            });
            return response.data.id;
        } else {
            results.fail('Create webhook', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Create webhook',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
            return null;
        }
    } catch (error) {
        results.error('Create webhook', error);
        reporter.printResult({
            name: 'Create webhook',
            status: 'error',
            error: error.message,
        });
        return null;
    }
}

async function testDeleteWebhook(results, webhookId) {
    console.log('  Testing delete webhook...');

    if (!webhookId) {
        results.skip('Delete webhook', 'No webhook ID available');
        reporter.printResult({ name: 'Delete webhook', status: 'skip' });
        return;
    }

    try {
        const startTime = Date.now();
        const response = await utils.client.delete(`${NOTIF_BASE}/api/webhooks/${webhookId}`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200 || response.status === 204) {
            results.pass('Delete webhook', { responseTime });
            reporter.printResult({
                name: 'Delete webhook',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Delete webhook', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Delete webhook',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Delete webhook', error);
        reporter.printResult({
            name: 'Delete webhook',
            status: 'error',
            error: error.message,
        });
    }
}

async function testGetEmailSettings(results) {
    console.log('  Testing get email settings...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${NOTIF_BASE}/api/email/settings`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200 && response.data) {
            results.pass('Get email settings', {
                responseTime,
                enabled: response.data.enabled,
                smtpHost: response.data.smtp_host,
            });
            reporter.printResult({
                name: 'Get email settings',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get email settings', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Get email settings',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get email settings', error);
        reporter.printResult({
            name: 'Get email settings',
            status: 'error',
            error: error.message,
        });
    }
}

async function testGetPreferences(results) {
    console.log('  Testing get preferences...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${NOTIF_BASE}/api/preferences`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200 && response.data) {
            results.pass('Get preferences', {
                responseTime,
                emailEnabled: response.data.email_enabled,
                webhookEnabled: response.data.webhook_enabled,
            });
            reporter.printResult({
                name: 'Get preferences',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get preferences', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Get preferences',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get preferences', error);
        reporter.printResult({
            name: 'Get preferences',
            status: 'error',
            error: error.message,
        });
    }
}

async function testDashboardUI(results) {
    console.log('  Testing notification hub UI...');

    try {
        const startTime = Date.now();
        const response = await utils.client.get(`${NOTIF_BASE}/`);
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

async function testRelatedServices(results) {
    console.log('  Testing related notification services...');

    const services = [
        { name: 'notification-email', port: 8080 },
        { name: 'notification-queue', port: 80 },
        { name: 'notification-webhook', port: 8080 },
    ];

    for (const svc of services) {
        const result = await utils.healthCheck(svc.name, svc.port, '/health');

        if (result.status === 'pass') {
            results.pass(`${svc.name} health`, { responseTime: result.responseTime });
        } else {
            results.fail(`${svc.name} health`, result.error);
        }

        reporter.printResult({
            name: `${svc.name} health`,
            status: result.status,
            responseTime: result.responseTime,
        });
    }
}

async function runNotificationHubTests() {
    const results = new TestResults('Notification Hub');

    reporter.printHeader('Notification Hub E2E Tests');
    console.log(`  Target: ${NOTIF_BASE}`);

    // Run tests
    await testHealthEndpoint(results);
    await testGetNotifications(results);
    await testFilterNotifications(results);
    await testSendNotification(results);
    await testGetStats(results);
    await testRefresh(results);
    await testGetWebhooks(results);
    const webhookId = await testCreateWebhook(results);
    await testDeleteWebhook(results, webhookId);
    await testGetEmailSettings(results);
    await testGetPreferences(results);
    await testDashboardUI(results);
    await testRelatedServices(results);

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const json = args.includes('--json');

    const results = await runNotificationHubTests();

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

module.exports = { runNotificationHubTests };
