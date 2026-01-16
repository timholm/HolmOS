/**
 * HolmOS Test Utilities
 *
 * Common utilities for all test suites
 */

const axios = require('axios');
const config = require('../config');

class TestUtils {
    constructor() {
        this.client = axios.create({
            timeout: config.timeouts.api,
            validateStatus: () => true, // Don't throw on any status
        });
        this.authToken = null;
    }

    /**
     * Build URL for a service (uses NodePort if available)
     */
    buildServiceUrl(serviceName, path = '/health') {
        return config.getServiceUrl(serviceName, 8080, path);
    }

    /**
     * Build external cluster URL
     */
    buildExternalUrl(path = '/') {
        return `http://${config.cluster.host}:${config.cluster.ingressPort}${path}`;
    }

    /**
     * Perform health check on a service (uses NodePort if available)
     */
    async healthCheck(serviceName, port = 8080, path = '/health') {
        const url = config.getServiceUrl(serviceName, port, path);
        const startTime = Date.now();

        try {
            const response = await this.client.get(url, {
                timeout: config.timeouts.health,
            });

            const responseTime = Date.now() - startTime;

            return {
                service: serviceName,
                status: response.status >= 200 && response.status < 300 ? 'pass' : 'fail',
                statusCode: response.status,
                responseTime,
                message: response.status >= 200 && response.status < 300 ? 'Healthy' : `HTTP ${response.status}`,
                data: response.data,
            };
        } catch (error) {
            const responseTime = Date.now() - startTime;
            return {
                service: serviceName,
                status: 'error',
                statusCode: 0,
                responseTime,
                message: error.code || error.message,
                error: error.message,
            };
        }
    }

    /**
     * Perform external health check through ingress
     */
    async externalHealthCheck(path = '/health') {
        const url = this.buildExternalUrl(path);
        const startTime = Date.now();

        try {
            const response = await this.client.get(url, {
                timeout: config.timeouts.health,
            });

            return {
                url,
                status: response.status >= 200 && response.status < 300 ? 'pass' : 'fail',
                statusCode: response.status,
                responseTime: Date.now() - startTime,
                data: response.data,
            };
        } catch (error) {
            return {
                url,
                status: 'error',
                statusCode: 0,
                responseTime: Date.now() - startTime,
                error: error.message,
            };
        }
    }

    /**
     * Login and get auth token
     */
    async login(username = config.auth.username, password = config.auth.password) {
        const url = config.getServiceUrl('auth-gateway', 8080, '/api/login');

        try {
            const response = await this.client.post(url, {
                username,
                password,
            }, {
                headers: { 'Content-Type': 'application/json' },
            });

            if (response.status === 200 && response.data.access_token) {
                this.authToken = response.data.access_token;
                return {
                    success: true,
                    token: this.authToken,
                    expiresIn: response.data.expires_in,
                };
            }

            return {
                success: false,
                error: response.data.error || 'Login failed',
            };
        } catch (error) {
            return {
                success: false,
                error: error.message,
            };
        }
    }

    /**
     * Make authenticated API request
     */
    async authRequest(method, url, data = null) {
        if (!this.authToken) {
            const loginResult = await this.login();
            if (!loginResult.success) {
                throw new Error(`Authentication failed: ${loginResult.error}`);
            }
        }

        const requestConfig = {
            method,
            url,
            headers: {
                'Authorization': `Bearer ${this.authToken}`,
                'Content-Type': 'application/json',
            },
        };

        if (data) {
            requestConfig.data = data;
        }

        return await this.client(requestConfig);
    }

    /**
     * Validate token
     */
    async validateToken(token = this.authToken) {
        if (!token) {
            return { valid: false, error: 'No token provided' };
        }

        const url = config.getServiceUrl('auth-gateway', 8080, '/api/validate');

        try {
            const response = await this.client.get(url, {
                headers: { 'Authorization': `Bearer ${token}` },
            });

            return response.data;
        } catch (error) {
            return { valid: false, error: error.message };
        }
    }

    /**
     * Sleep utility
     */
    sleep(ms) {
        return new Promise(resolve => setTimeout(resolve, ms));
    }

    /**
     * Retry a function with backoff
     */
    async retry(fn, maxRetries = 3, delay = 1000) {
        let lastError;

        for (let i = 0; i < maxRetries; i++) {
            try {
                return await fn();
            } catch (error) {
                lastError = error;
                if (i < maxRetries - 1) {
                    await this.sleep(delay * Math.pow(2, i));
                }
            }
        }

        throw lastError;
    }

    /**
     * Measure response time for a function
     */
    async measureTime(fn) {
        const startTime = Date.now();
        const result = await fn();
        const endTime = Date.now();

        return {
            result,
            duration: endTime - startTime,
        };
    }

    /**
     * Generate random string
     */
    randomString(length = 10) {
        const chars = 'abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
        let result = '';
        for (let i = 0; i < length; i++) {
            result += chars.charAt(Math.floor(Math.random() * chars.length));
        }
        return result;
    }

    /**
     * Generate test data
     */
    generateTestData(type = 'user') {
        switch (type) {
            case 'user':
                return {
                    username: `testuser_${this.randomString(6)}`,
                    email: `test_${this.randomString(6)}@holmos.test`,
                    password: this.randomString(12),
                };
            case 'file':
                return {
                    name: `test_file_${this.randomString(8)}.txt`,
                    content: `Test content: ${this.randomString(100)}`,
                };
            case 'notification':
                return {
                    title: `Test Notification ${this.randomString(6)}`,
                    message: `Test message content: ${this.randomString(50)}`,
                    type: 'info',
                    priority: 'normal',
                };
            default:
                return { id: this.randomString(10) };
        }
    }
}

// Test result collector
class TestResults {
    constructor(suiteName) {
        this.suiteName = suiteName;
        this.tests = [];
        this.startTime = Date.now();
        this.endTime = null;
    }

    addResult(testName, status, details = {}) {
        this.tests.push({
            name: testName,
            status, // 'pass', 'fail', 'skip', 'error'
            timestamp: new Date().toISOString(),
            ...details,
        });
    }

    pass(testName, details = {}) {
        this.addResult(testName, 'pass', details);
    }

    fail(testName, error, details = {}) {
        this.addResult(testName, 'fail', { error, ...details });
    }

    skip(testName, reason = 'Skipped') {
        this.addResult(testName, 'skip', { reason });
    }

    error(testName, error) {
        this.addResult(testName, 'error', { error: error.message || error });
    }

    finish() {
        this.endTime = Date.now();
    }

    getSummary() {
        const passed = this.tests.filter(t => t.status === 'pass').length;
        const failed = this.tests.filter(t => t.status === 'fail').length;
        const errors = this.tests.filter(t => t.status === 'error').length;
        const skipped = this.tests.filter(t => t.status === 'skip').length;

        return {
            suite: this.suiteName,
            total: this.tests.length,
            passed,
            failed,
            errors,
            skipped,
            duration: (this.endTime || Date.now()) - this.startTime,
            passRate: this.tests.length > 0
                ? ((passed / this.tests.length) * 100).toFixed(1)
                : 0,
        };
    }

    toJSON() {
        return {
            suite: this.suiteName,
            summary: this.getSummary(),
            tests: this.tests,
            startTime: new Date(this.startTime).toISOString(),
            endTime: this.endTime ? new Date(this.endTime).toISOString() : null,
        };
    }
}

// Console reporter with colors
class ConsoleReporter {
    constructor() {
        this.colors = config.colors;
    }

    formatStatus(status) {
        switch (status) {
            case 'pass': return '\x1b[32mPASS\x1b[0m';
            case 'fail': return '\x1b[31mFAIL\x1b[0m';
            case 'error': return '\x1b[35mERROR\x1b[0m';
            case 'skip': return '\x1b[33mSKIP\x1b[0m';
            default: return status;
        }
    }

    printHeader(title) {
        console.log('\n' + '='.repeat(60));
        console.log(`  ${title}`);
        console.log('='.repeat(60));
    }

    printResult(result) {
        const status = this.formatStatus(result.status);
        const time = result.responseTime ? ` (${result.responseTime}ms)` : '';
        console.log(`  ${status} ${result.name}${time}`);

        if (result.error) {
            console.log(`       Error: ${result.error}`);
        }
    }

    printSummary(summary) {
        console.log('\n' + '-'.repeat(40));
        console.log(`  Suite: ${summary.suite}`);
        console.log(`  Total: ${summary.total} tests`);
        console.log(`  \x1b[32mPassed: ${summary.passed}\x1b[0m`);
        console.log(`  \x1b[31mFailed: ${summary.failed}\x1b[0m`);
        console.log(`  \x1b[35mErrors: ${summary.errors}\x1b[0m`);
        console.log(`  \x1b[33mSkipped: ${summary.skipped}\x1b[0m`);
        console.log(`  Duration: ${summary.duration}ms`);
        console.log(`  Pass Rate: ${summary.passRate}%`);
        console.log('-'.repeat(40) + '\n');
    }
}

module.exports = {
    TestUtils,
    TestResults,
    ConsoleReporter,
};
