#!/usr/bin/env node
/**
 * HolmOS Test Runner
 *
 * Unified test runner for all test suites
 */

const fs = require('fs');
const path = require('path');
const config = require('./config');

// Import test modules
const { runHealthChecks, runQuickHealthCheck } = require('./e2e/health-checks.test');
const { runAuthGatewayTests } = require('./e2e/auth-gateway.test');
const { runFileServicesTests } = require('./e2e/file-services.test');
const { runMetricsDashboardTests } = require('./e2e/metrics-dashboard.test');
const { runNotificationHubTests } = require('./e2e/notification-hub.test');
const { runAuthFilesIntegrationTests } = require('./integration/auth-files.test');
const { runMetricsNotificationsIntegrationTests } = require('./integration/metrics-notifications.test');
const { runFullStackIntegrationTests } = require('./integration/full-stack.test');
const { runAllLoadTests } = require('./load/load-test');

class TestRunner {
    constructor() {
        this.results = [];
        this.startTime = null;
        this.endTime = null;
    }

    async runE2ETests() {
        console.log('\n========================================');
        console.log('  Running E2E Tests');
        console.log('========================================\n');

        const suites = [
            { name: 'Health Checks', fn: runHealthChecks },
            { name: 'Auth Gateway', fn: runAuthGatewayTests },
            { name: 'File Services', fn: runFileServicesTests },
            { name: 'Metrics Dashboard', fn: runMetricsDashboardTests },
            { name: 'Notification Hub', fn: runNotificationHubTests },
        ];

        for (const suite of suites) {
            try {
                console.log(`\nRunning ${suite.name}...`);
                const result = await suite.fn();
                this.results.push(result.toJSON());
            } catch (error) {
                console.error(`Error running ${suite.name}:`, error.message);
                this.results.push({
                    suite: suite.name,
                    error: error.message,
                    summary: { total: 0, passed: 0, failed: 0, errors: 1 },
                });
            }
        }
    }

    async runIntegrationTests() {
        console.log('\n========================================');
        console.log('  Running Integration Tests');
        console.log('========================================\n');

        const suites = [
            { name: 'Auth + Files Integration', fn: runAuthFilesIntegrationTests },
            { name: 'Metrics + Notifications Integration', fn: runMetricsNotificationsIntegrationTests },
            { name: 'Full Stack Integration', fn: runFullStackIntegrationTests },
        ];

        for (const suite of suites) {
            try {
                console.log(`\nRunning ${suite.name}...`);
                const result = await suite.fn();
                this.results.push(result.toJSON());
            } catch (error) {
                console.error(`Error running ${suite.name}:`, error.message);
                this.results.push({
                    suite: suite.name,
                    error: error.message,
                    summary: { total: 0, passed: 0, failed: 0, errors: 1 },
                });
            }
        }
    }

    async runLoadTests(options = {}) {
        console.log('\n========================================');
        console.log('  Running Load Tests');
        console.log('========================================\n');

        try {
            const loadResults = await runAllLoadTests(options);
            this.results.push({
                suite: 'Load Tests',
                tests: loadResults.map(r => ({
                    name: r.serviceName,
                    status: parseFloat(r.stats.successRate) >= 95 ? 'pass' : 'fail',
                    stats: r.stats,
                })),
                summary: {
                    total: loadResults.length,
                    passed: loadResults.filter(r => parseFloat(r.stats.successRate) >= 95).length,
                    failed: loadResults.filter(r => parseFloat(r.stats.successRate) < 95).length,
                    errors: 0,
                },
            });
        } catch (error) {
            console.error('Error running load tests:', error.message);
            this.results.push({
                suite: 'Load Tests',
                error: error.message,
                summary: { total: 0, passed: 0, failed: 0, errors: 1 },
            });
        }
    }

    async runQuickTests() {
        console.log('\n========================================');
        console.log('  Running Quick Health Checks');
        console.log('========================================\n');

        try {
            const result = await runQuickHealthCheck();
            this.results.push(result.toJSON());
        } catch (error) {
            console.error('Error running quick tests:', error.message);
        }
    }

    async runAll(options = {}) {
        this.startTime = Date.now();

        await this.runE2ETests();
        await this.runIntegrationTests();

        if (!options.skipLoad) {
            await this.runLoadTests({
                users: options.loadUsers || 5,
                requests: options.loadRequests || 20,
            });
        }

        this.endTime = Date.now();
        return this.generateReport();
    }

    generateReport() {
        const totalTests = this.results.reduce((sum, r) => sum + (r.summary?.total || 0), 0);
        const totalPassed = this.results.reduce((sum, r) => sum + (r.summary?.passed || 0), 0);
        const totalFailed = this.results.reduce((sum, r) => sum + (r.summary?.failed || 0), 0);
        const totalErrors = this.results.reduce((sum, r) => sum + (r.summary?.errors || 0), 0);

        const report = {
            timestamp: new Date().toISOString(),
            duration: this.endTime - this.startTime,
            cluster: {
                host: config.cluster.host,
                namespace: config.cluster.namespace,
            },
            summary: {
                suites: this.results.length,
                totalTests,
                passed: totalPassed,
                failed: totalFailed,
                errors: totalErrors,
                passRate: totalTests > 0 ? ((totalPassed / totalTests) * 100).toFixed(1) : 0,
            },
            results: this.results,
        };

        return report;
    }

    printSummary(report) {
        console.log('\n' + '='.repeat(60));
        console.log('  TEST EXECUTION SUMMARY');
        console.log('='.repeat(60));
        console.log(`  Cluster:        ${report.cluster.host}`);
        console.log(`  Namespace:      ${report.cluster.namespace}`);
        console.log(`  Duration:       ${(report.duration / 1000).toFixed(2)}s`);
        console.log('-'.repeat(60));
        console.log(`  Test Suites:    ${report.summary.suites}`);
        console.log(`  Total Tests:    ${report.summary.totalTests}`);
        console.log(`  \x1b[32mPassed:        ${report.summary.passed}\x1b[0m`);
        console.log(`  \x1b[31mFailed:        ${report.summary.failed}\x1b[0m`);
        console.log(`  \x1b[35mErrors:        ${report.summary.errors}\x1b[0m`);
        console.log(`  Pass Rate:      ${report.summary.passRate}%`);
        console.log('='.repeat(60));

        // Per-suite summary
        console.log('\n  Suite Results:');
        for (const suite of report.results) {
            const status = (suite.summary?.failed || 0) + (suite.summary?.errors || 0) === 0
                ? '\x1b[32mPASS\x1b[0m'
                : '\x1b[31mFAIL\x1b[0m';
            const summary = suite.summary || {};
            console.log(`    ${status} ${suite.suite}: ${summary.passed || 0}/${summary.total || 0} passed`);
        }
        console.log('');
    }

    saveReport(report, outputPath) {
        const reportsDir = path.dirname(outputPath);
        if (!fs.existsSync(reportsDir)) {
            fs.mkdirSync(reportsDir, { recursive: true });
        }

        fs.writeFileSync(outputPath, JSON.stringify(report, null, 2));
        console.log(`  Report saved to: ${outputPath}\n`);
    }
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const suite = args.find(a => a.startsWith('--suite='))?.split('=')[1];
    const quick = args.includes('--quick');
    const json = args.includes('--json');
    const skipLoad = args.includes('--skip-load');
    const saveReport = args.includes('--save-report');

    const runner = new TestRunner();
    let report;

    if (quick) {
        await runner.runQuickTests();
        report = runner.generateReport();
    } else if (suite === 'e2e') {
        await runner.runE2ETests();
        report = runner.generateReport();
    } else if (suite === 'integration') {
        await runner.runIntegrationTests();
        report = runner.generateReport();
    } else if (suite === 'load') {
        await runner.runLoadTests();
        report = runner.generateReport();
    } else {
        report = await runner.runAll({ skipLoad });
    }

    if (json) {
        console.log(JSON.stringify(report, null, 2));
    } else {
        runner.printSummary(report);
    }

    if (saveReport) {
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const outputPath = path.join(__dirname, 'reports', `test-report-${timestamp}.json`);
        runner.saveReport(report, outputPath);
    }

    // Exit with error code if any tests failed
    const exitCode = (report.summary.failed + report.summary.errors) > 0 ? 1 : 0;
    process.exit(exitCode);
}

if (require.main === module) {
    main().catch(err => {
        console.error('Test runner failed:', err);
        process.exit(1);
    });
}

module.exports = { TestRunner };
