#!/usr/bin/env node
/**
 * HolmOS Load Testing Script
 *
 * Performs load testing on key services
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();


class LoadTestRunner {
    constructor(options = {}) {
        this.concurrentUsers = options.concurrentUsers || 10;
        this.requestsPerUser = options.requestsPerUser || 100;
        this.rampUpTime = options.rampUpTime || 5000;
        this.holdTime = options.holdTime || 30000;
        this.targetUrl = options.targetUrl || null;
        this.results = [];
    }

    async runSingleRequest(url) {
        const startTime = Date.now();
        try {
            const response = await utils.client.get(url, { timeout: 10000 });
            const endTime = Date.now();
            return {
                success: response.status >= 200 && response.status < 400,
                statusCode: response.status,
                responseTime: endTime - startTime,
                error: null,
            };
        } catch (error) {
            const endTime = Date.now();
            return {
                success: false,
                statusCode: 0,
                responseTime: endTime - startTime,
                error: error.message,
            };
        }
    }

    async runUserSession(userId, url, requestCount) {
        const sessionResults = [];

        for (let i = 0; i < requestCount; i++) {
            const result = await this.runSingleRequest(url);
            sessionResults.push({
                userId,
                requestId: i,
                ...result,
                timestamp: new Date().toISOString(),
            });

            // Small random delay between requests (10-50ms)
            await utils.sleep(Math.random() * 40 + 10);
        }

        return sessionResults;
    }

    async rampUp(url, targetUsers) {
        console.log(`\n  Ramping up to ${targetUsers} concurrent users over ${this.rampUpTime}ms...`);
        const userStartDelay = this.rampUpTime / targetUsers;
        const userPromises = [];

        for (let i = 0; i < targetUsers; i++) {
            await utils.sleep(userStartDelay);
            userPromises.push(this.runUserSession(i, url, this.requestsPerUser));
            process.stdout.write(`\r    Users started: ${i + 1}/${targetUsers}`);
        }

        console.log('\n  All users started, waiting for completion...');
        const allResults = await Promise.all(userPromises);
        return allResults.flat();
    }

    calculateStats(results) {
        const successful = results.filter(r => r.success);
        const failed = results.filter(r => !r.success);
        const responseTimes = successful.map(r => r.responseTime);

        if (responseTimes.length === 0) {
            return {
                totalRequests: results.length,
                successful: 0,
                failed: results.length,
                successRate: 0,
                avgResponseTime: 0,
                minResponseTime: 0,
                maxResponseTime: 0,
                p50ResponseTime: 0,
                p95ResponseTime: 0,
                p99ResponseTime: 0,
                requestsPerSecond: 0,
            };
        }

        responseTimes.sort((a, b) => a - b);

        const sum = responseTimes.reduce((a, b) => a + b, 0);
        const avg = sum / responseTimes.length;
        const min = responseTimes[0];
        const max = responseTimes[responseTimes.length - 1];

        const percentile = (arr, p) => {
            const idx = Math.ceil(arr.length * p) - 1;
            return arr[Math.max(0, idx)];
        };

        // Calculate duration
        const timestamps = results.map(r => new Date(r.timestamp).getTime());
        const duration = (Math.max(...timestamps) - Math.min(...timestamps)) / 1000;
        const rps = results.length / Math.max(duration, 1);

        return {
            totalRequests: results.length,
            successful: successful.length,
            failed: failed.length,
            successRate: ((successful.length / results.length) * 100).toFixed(2),
            avgResponseTime: avg.toFixed(2),
            minResponseTime: min,
            maxResponseTime: max,
            p50ResponseTime: percentile(responseTimes, 0.5),
            p95ResponseTime: percentile(responseTimes, 0.95),
            p99ResponseTime: percentile(responseTimes, 0.99),
            requestsPerSecond: rps.toFixed(2),
            duration: duration.toFixed(2),
        };
    }

    printStats(stats, serviceName) {
        console.log('\n' + '='.repeat(50));
        console.log(`  Load Test Results: ${serviceName}`);
        console.log('='.repeat(50));
        console.log(`  Total Requests:    ${stats.totalRequests}`);
        console.log(`  Successful:        ${stats.successful}`);
        console.log(`  Failed:            ${stats.failed}`);
        console.log(`  Success Rate:      ${stats.successRate}%`);
        console.log(`  Duration:          ${stats.duration}s`);
        console.log(`  Requests/sec:      ${stats.requestsPerSecond}`);
        console.log('-'.repeat(50));
        console.log('  Response Times:');
        console.log(`    Average:         ${stats.avgResponseTime}ms`);
        console.log(`    Min:             ${stats.minResponseTime}ms`);
        console.log(`    Max:             ${stats.maxResponseTime}ms`);
        console.log(`    P50:             ${stats.p50ResponseTime}ms`);
        console.log(`    P95:             ${stats.p95ResponseTime}ms`);
        console.log(`    P99:             ${stats.p99ResponseTime}ms`);
        console.log('='.repeat(50) + '\n');
    }

    async run(url, serviceName) {
        console.log(`\n  Starting load test for ${serviceName}...`);
        console.log(`  Target URL: ${url}`);
        console.log(`  Concurrent Users: ${this.concurrentUsers}`);
        console.log(`  Requests per User: ${this.requestsPerUser}`);

        const results = await this.rampUp(url, this.concurrentUsers);
        const stats = this.calculateStats(results);
        this.printStats(stats, serviceName);

        return {
            serviceName,
            url,
            config: {
                concurrentUsers: this.concurrentUsers,
                requestsPerUser: this.requestsPerUser,
            },
            stats,
            rawResults: results,
        };
    }
}

async function runHealthEndpointLoad(options = {}) {
    const runner = new LoadTestRunner({
        concurrentUsers: options.users || 10,
        requestsPerUser: options.requests || 50,
        rampUpTime: options.rampUp || 5000,
    });

    const url = config.getServiceUrl('auth-gateway', 8080, '/health');
    return await runner.run(url, 'Auth Gateway Health');
}

async function runMetricsDashboardLoad(options = {}) {
    const runner = new LoadTestRunner({
        concurrentUsers: options.users || 10,
        requestsPerUser: options.requests || 30,
        rampUpTime: options.rampUp || 5000,
    });

    const url = config.getServiceUrl('metrics-dashboard', 8080, '/api/cluster');
    return await runner.run(url, 'Metrics Dashboard Cluster API');
}

async function runNotificationHubLoad(options = {}) {
    const runner = new LoadTestRunner({
        concurrentUsers: options.users || 10,
        requestsPerUser: options.requests || 30,
        rampUpTime: options.rampUp || 5000,
    });

    const url = config.getServiceUrl('notification-hub', 8080, '/api/notifications');
    return await runner.run(url, 'Notification Hub API');
}

async function runFileServiceLoad(options = {}) {
    const runner = new LoadTestRunner({
        concurrentUsers: options.users || 5,
        requestsPerUser: options.requests || 20,
        rampUpTime: options.rampUp || 5000,
    });

    const url = config.getServiceUrl('file-list', 8080, '/health');
    return await runner.run(url, 'File List Service');
}

async function runAllLoadTests(options = {}) {
    reporter.printHeader('HolmOS Load Tests');
    console.log(`  Cluster: ${config.cluster.host}`);
    console.log(`  Namespace: ${config.cluster.namespace}`);

    const results = [];

    console.log('\n  Running Auth Gateway load test...');
    results.push(await runHealthEndpointLoad(options));

    console.log('\n  Running Metrics Dashboard load test...');
    results.push(await runMetricsDashboardLoad(options));

    console.log('\n  Running Notification Hub load test...');
    results.push(await runNotificationHubLoad(options));

    console.log('\n  Running File Service load test...');
    results.push(await runFileServiceLoad(options));

    // Print summary
    console.log('\n' + '='.repeat(60));
    console.log('  Load Test Summary');
    console.log('='.repeat(60));

    for (const result of results) {
        const status = parseFloat(result.stats.successRate) >= 95 ? '\x1b[32mPASS\x1b[0m' : '\x1b[31mFAIL\x1b[0m';
        console.log(`  ${status} ${result.serviceName}: ${result.stats.successRate}% success, ${result.stats.avgResponseTime}ms avg, ${result.stats.requestsPerSecond} req/s`);
    }
    console.log('='.repeat(60) + '\n');

    return results;
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const users = parseInt(args.find(a => a.startsWith('--users='))?.split('=')[1]) || 10;
    const requests = parseInt(args.find(a => a.startsWith('--requests='))?.split('=')[1]) || 50;
    const target = args.find(a => a.startsWith('--target='))?.split('=')[1];
    const json = args.includes('--json');
    const quick = args.includes('--quick');

    const options = {
        users: quick ? 5 : users,
        requests: quick ? 20 : requests,
        rampUp: quick ? 2000 : 5000,
    };

    let results;

    if (target) {
        // Run load test on specific target
        const runner = new LoadTestRunner({
            concurrentUsers: options.users,
            requestsPerUser: options.requests,
            rampUpTime: options.rampUp,
        });
        results = [await runner.run(target, 'Custom Target')];
    } else {
        // Run all load tests
        results = await runAllLoadTests(options);
    }

    if (json) {
        console.log(JSON.stringify(results, null, 2));
    }

    // Check if all tests passed (>= 95% success rate)
    const allPassed = results.every(r => parseFloat(r.stats.successRate) >= 95);
    process.exit(allPassed ? 0 : 1);
}

if (require.main === module) {
    main().catch(err => {
        console.error('Load test execution failed:', err);
        process.exit(1);
    });
}

module.exports = {
    LoadTestRunner,
    runHealthEndpointLoad,
    runMetricsDashboardLoad,
    runNotificationHubLoad,
    runFileServiceLoad,
    runAllLoadTests,
};
