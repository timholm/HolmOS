#!/usr/bin/env node
/**
 * HolmOS E2E Health Check Tests
 *
 * Tests all service health endpoints across the cluster
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();

async function runHealthChecks() {
    const results = new TestResults('Health Checks');

    reporter.printHeader('HolmOS Health Check Tests');
    console.log(`  Cluster: ${config.cluster.host}`);
    console.log(`  Namespace: ${config.cluster.namespace}`);
    console.log(`  Total Services: ${config.getAllServices().length}`);

    // Test by category
    for (const [category, services] of Object.entries(config.services)) {
        console.log(`\n  Testing ${category.toUpperCase()} services...`);

        for (const service of services) {
            const result = await utils.healthCheck(
                service.name,
                service.port,
                service.path
            );

            const testResult = {
                name: `${service.name} (${category})`,
                status: result.status,
                responseTime: result.responseTime,
                statusCode: result.statusCode,
            };

            if (result.status === 'pass') {
                results.pass(testResult.name, testResult);
            } else if (result.status === 'fail') {
                results.fail(testResult.name, result.message, testResult);
            } else {
                results.error(testResult.name, new Error(result.error || result.message));
            }

            reporter.printResult({
                name: service.name,
                status: result.status,
                responseTime: result.responseTime,
                error: result.status !== 'pass' ? (result.error || result.message) : null,
            });
        }
    }

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

async function runCategoryHealthChecks(category) {
    const results = new TestResults(`Health Checks - ${category}`);
    const services = config.services[category];

    if (!services) {
        console.error(`Unknown category: ${category}`);
        process.exit(1);
    }

    reporter.printHeader(`HolmOS Health Checks - ${category.toUpperCase()}`);

    for (const service of services) {
        const result = await utils.healthCheck(
            service.name,
            service.port,
            service.path
        );

        if (result.status === 'pass') {
            results.pass(service.name, { responseTime: result.responseTime });
        } else {
            results.fail(service.name, result.error || result.message);
        }

        reporter.printResult({
            name: service.name,
            status: result.status,
            responseTime: result.responseTime,
            error: result.status !== 'pass' ? result.error : null,
        });
    }

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

async function runQuickHealthCheck() {
    const results = new TestResults('Quick Health Check');
    const criticalServices = [
        { name: 'auth-gateway', port: 8080, path: '/health' },
        { name: 'api-gateway', port: 8080, path: '/health' },
        { name: 'metrics-dashboard', port: 8080, path: '/health' },
        { name: 'test-dashboard', port: 8080, path: '/health' },
        { name: 'notification-hub', port: 8080, path: '/health' },
    ];

    reporter.printHeader('Quick Health Check - Critical Services');

    for (const service of criticalServices) {
        const result = await utils.healthCheck(
            service.name,
            service.port,
            service.path
        );

        if (result.status === 'pass') {
            results.pass(service.name, { responseTime: result.responseTime });
        } else {
            results.fail(service.name, result.error || result.message);
        }

        reporter.printResult({
            name: service.name,
            status: result.status,
            responseTime: result.responseTime,
        });
    }

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const category = args.find(a => !a.startsWith('--'));
    const quick = args.includes('--quick');
    const json = args.includes('--json');

    let results;

    if (quick) {
        results = await runQuickHealthCheck();
    } else if (category) {
        results = await runCategoryHealthChecks(category);
    } else {
        results = await runHealthChecks();
    }

    if (json) {
        console.log(JSON.stringify(results.toJSON(), null, 2));
    }

    // Exit with error code if any tests failed
    const summary = results.getSummary();
    process.exit(summary.failed + summary.errors > 0 ? 1 : 0);
}

// Run if called directly
if (require.main === module) {
    main().catch(err => {
        console.error('Test execution failed:', err);
        process.exit(1);
    });
}

module.exports = {
    runHealthChecks,
    runCategoryHealthChecks,
    runQuickHealthCheck,
};
