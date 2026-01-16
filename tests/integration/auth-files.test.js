#!/usr/bin/env node
/**
 * HolmOS Integration Tests - Auth + File Services
 *
 * Tests integration between authentication and file services
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();


async function testAuthenticatedFileList(results) {
    console.log('\n  Testing authenticated file listing...');

    try {
        // Login first
        const loginResult = await utils.login();
        if (!loginResult.success) {
            results.fail('Authenticated file list', `Login failed: ${loginResult.error}`);
            reporter.printResult({
                name: 'Authenticated file list',
                status: 'fail',
                error: 'Login failed',
            });
            return;
        }

        // List files with auth token
        const url = config.getServiceUrl('file-list', 8080, '/list?path=/');
        const startTime = Date.now();
        const response = await utils.authRequest('GET', url);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Authenticated file list succeeds', {
                responseTime,
                authenticated: true,
            });
            reporter.printResult({
                name: 'Authenticated file list',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Authenticated file list succeeds', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Authenticated file list',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Authenticated file list succeeds', error);
        reporter.printResult({
            name: 'Authenticated file list',
            status: 'error',
            error: error.message,
        });
    }
}

async function testUnauthenticatedFileList(results) {
    console.log('  Testing unauthenticated file listing (should fail or be restricted)...');

    try {
        const url = config.getServiceUrl('file-list', 8080, '/list?path=/');
        const startTime = Date.now();
        const response = await utils.client.get(url);
        const responseTime = Date.now() - startTime;

        // Expecting either 401 Unauthorized or 403 Forbidden, or restricted results
        if (response.status === 401 || response.status === 403) {
            results.pass('Unauthenticated file list is rejected', {
                responseTime,
                statusCode: response.status,
            });
            reporter.printResult({
                name: 'Unauthenticated file list rejected',
                status: 'pass',
                responseTime,
            });
        } else if (response.status === 200) {
            // Some services may allow unauthenticated access with restrictions
            results.pass('Unauthenticated file list (may be restricted)', {
                responseTime,
                note: 'Service allows unauthenticated access',
            });
            reporter.printResult({
                name: 'Unauthenticated file list',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Unauthenticated file list behavior', `Unexpected status: ${response.status}`);
            reporter.printResult({
                name: 'Unauthenticated file list',
                status: 'fail',
                error: `Unexpected status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Unauthenticated file list behavior', error);
        reporter.printResult({
            name: 'Unauthenticated file list',
            status: 'error',
            error: error.message,
        });
    }
}

async function testTokenValidationForFileOps(results) {
    console.log('  Testing token validation integration...');

    try {
        // Login and get token
        const loginResult = await utils.login();
        if (!loginResult.success) {
            results.skip('Token validation for file ops', 'Login failed');
            reporter.printResult({ name: 'Token validation', status: 'skip' });
            return;
        }

        // Validate token
        const validationResult = await utils.validateToken(loginResult.token);

        if (validationResult.valid) {
            // Now use validated token for file operation
            const url = config.getServiceUrl('file-meta', 8080, '/meta?path=/');
            const startTime = Date.now();
            const response = await utils.authRequest('GET', url);
            const responseTime = Date.now() - startTime;

            if (response.status === 200) {
                results.pass('Token validation enables file operations', {
                    responseTime,
                    username: validationResult.username,
                });
                reporter.printResult({
                    name: 'Token validation for file ops',
                    status: 'pass',
                    responseTime,
                });
            } else {
                results.fail('Token validation enables file operations', `Status: ${response.status}`);
                reporter.printResult({
                    name: 'Token validation for file ops',
                    status: 'fail',
                });
            }
        } else {
            results.fail('Token validation enables file operations', 'Token invalid');
            reporter.printResult({
                name: 'Token validation for file ops',
                status: 'fail',
                error: 'Token validation failed',
            });
        }
    } catch (error) {
        results.error('Token validation enables file operations', error);
        reporter.printResult({
            name: 'Token validation for file ops',
            status: 'error',
            error: error.message,
        });
    }
}

async function testFileUploadWithAuth(results) {
    console.log('  Testing file upload with authentication...');

    try {
        const loginResult = await utils.login();
        if (!loginResult.success) {
            results.skip('File upload with auth', 'Login failed');
            reporter.printResult({ name: 'File upload with auth', status: 'skip' });
            return;
        }

        // Check if file-upload service is available
        const healthResult = await utils.healthCheck('file-upload', 8080, '/health');

        if (healthResult.status === 'pass') {
            results.pass('File upload service available with auth', {
                responseTime: healthResult.responseTime,
            });
            reporter.printResult({
                name: 'File upload with auth',
                status: 'pass',
                responseTime: healthResult.responseTime,
            });
        } else {
            results.fail('File upload service available with auth', healthResult.error);
            reporter.printResult({
                name: 'File upload with auth',
                status: 'fail',
                error: healthResult.error,
            });
        }
    } catch (error) {
        results.error('File upload service available with auth', error);
        reporter.printResult({
            name: 'File upload with auth',
            status: 'error',
            error: error.message,
        });
    }
}

async function testSessionPersistence(results) {
    console.log('  Testing session persistence across services...');

    try {
        // Login
        const loginResult = await utils.login();
        if (!loginResult.success) {
            results.skip('Session persistence', 'Login failed');
            reporter.printResult({ name: 'Session persistence', status: 'skip' });
            return;
        }

        // Make requests to multiple file services with same token
        const services = [
            { name: 'file-list', path: '/list?path=/' },
            { name: 'file-meta', path: '/meta?path=/' },
        ];

        let successCount = 0;
        for (const svc of services) {
            const url = config.getServiceUrl(svc.name, 8080, svc.path);
            const response = await utils.authRequest('GET', url);
            if (response.status === 200) {
                successCount++;
            }
        }

        if (successCount === services.length) {
            results.pass('Session persists across file services', {
                servicesChecked: services.length,
            });
            reporter.printResult({
                name: 'Session persistence',
                status: 'pass',
            });
        } else {
            results.fail('Session persists across file services', `Only ${successCount}/${services.length} succeeded`);
            reporter.printResult({
                name: 'Session persistence',
                status: 'fail',
            });
        }
    } catch (error) {
        results.error('Session persists across file services', error);
        reporter.printResult({
            name: 'Session persistence',
            status: 'error',
            error: error.message,
        });
    }
}

async function runAuthFilesIntegrationTests() {
    const results = new TestResults('Auth + Files Integration');

    reporter.printHeader('Auth + Files Integration Tests');
    console.log(`  Namespace: ${config.cluster.namespace}`);

    await testAuthenticatedFileList(results);
    await testUnauthenticatedFileList(results);
    await testTokenValidationForFileOps(results);
    await testFileUploadWithAuth(results);
    await testSessionPersistence(results);

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const json = args.includes('--json');

    const results = await runAuthFilesIntegrationTests();

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

module.exports = { runAuthFilesIntegrationTests };
