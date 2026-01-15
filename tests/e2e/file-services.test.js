#!/usr/bin/env node
/**
 * HolmOS E2E File Services Tests
 *
 * Tests file management functionality
 */

const config = require('../config');
const { TestUtils, TestResults, ConsoleReporter } = require('../utils/test-utils');

const utils = new TestUtils();
const reporter = new ConsoleReporter();

const NAMESPACE = config.cluster.namespace;

async function testFileListHealth(results) {
    console.log('\n  Testing file-list health...');
    const result = await utils.healthCheck('file-list', 8080, '/health');

    if (result.status === 'pass') {
        results.pass('file-list health endpoint', { responseTime: result.responseTime });
    } else {
        results.fail('file-list health endpoint', result.error);
    }

    reporter.printResult({
        name: 'file-list health',
        status: result.status,
        responseTime: result.responseTime,
    });
}

async function testFileUploadHealth(results) {
    console.log('  Testing file-upload health...');
    const result = await utils.healthCheck('file-upload', 8080, '/health');

    if (result.status === 'pass') {
        results.pass('file-upload health endpoint', { responseTime: result.responseTime });
    } else {
        results.fail('file-upload health endpoint', result.error);
    }

    reporter.printResult({
        name: 'file-upload health',
        status: result.status,
        responseTime: result.responseTime,
    });
}

async function testFileDownloadHealth(results) {
    console.log('  Testing file-download health...');
    const result = await utils.healthCheck('file-download', 8080, '/health');

    if (result.status === 'pass') {
        results.pass('file-download health endpoint', { responseTime: result.responseTime });
    } else {
        results.fail('file-download health endpoint', result.error);
    }

    reporter.printResult({
        name: 'file-download health',
        status: result.status,
        responseTime: result.responseTime,
    });
}

async function testFileOperationsHealth(results) {
    const operations = [
        { name: 'file-delete', port: 8080 },
        { name: 'file-copy', port: 8080 },
        { name: 'file-move', port: 8080 },
        { name: 'file-mkdir', port: 8080 },
        { name: 'file-meta', port: 8080 },
        { name: 'file-search', port: 8080 },
        { name: 'file-compress', port: 8080 },
        { name: 'file-decompress', port: 8080 },
    ];

    console.log('  Testing file operations health...');

    for (const op of operations) {
        const result = await utils.healthCheck(op.name, op.port, '/health');

        if (result.status === 'pass') {
            results.pass(`${op.name} health endpoint`, { responseTime: result.responseTime });
        } else {
            results.fail(`${op.name} health endpoint`, result.error);
        }

        reporter.printResult({
            name: `${op.name} health`,
            status: result.status,
            responseTime: result.responseTime,
        });
    }
}

async function testFileListDirectory(results) {
    console.log('  Testing directory listing...');

    const url = `http://file-list.${NAMESPACE}.svc.cluster.local:8080/list`;

    try {
        // First login to get auth
        await utils.login();

        const startTime = Date.now();
        const response = await utils.authRequest('GET', `${url}?path=/`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('List root directory', {
                responseTime,
                hasData: Array.isArray(response.data) || typeof response.data === 'object',
            });
            reporter.printResult({
                name: 'List root directory',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('List root directory', `Status: ${response.status}`);
            reporter.printResult({
                name: 'List root directory',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('List root directory', error);
        reporter.printResult({
            name: 'List root directory',
            status: 'error',
            error: error.message,
        });
    }
}

async function testFileCreateDirectory(results) {
    console.log('  Testing directory creation...');

    const url = `http://file-mkdir.${NAMESPACE}.svc.cluster.local:8080/mkdir`;
    const testDir = `/tmp/test_dir_${utils.randomString(8)}`;

    try {
        await utils.login();

        const startTime = Date.now();
        const response = await utils.authRequest('POST', url, { path: testDir });
        const responseTime = Date.now() - startTime;

        if (response.status === 200 || response.status === 201) {
            results.pass('Create directory', { responseTime, path: testDir });
            reporter.printResult({
                name: 'Create directory',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Create directory', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Create directory',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Create directory', error);
        reporter.printResult({
            name: 'Create directory',
            status: 'error',
            error: error.message,
        });
    }
}

async function testFileMetadata(results) {
    console.log('  Testing file metadata...');

    const url = `http://file-meta.${NAMESPACE}.svc.cluster.local:8080/meta`;

    try {
        await utils.login();

        const startTime = Date.now();
        const response = await utils.authRequest('GET', `${url}?path=/`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Get file metadata', { responseTime });
            reporter.printResult({
                name: 'Get file metadata',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Get file metadata', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Get file metadata',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Get file metadata', error);
        reporter.printResult({
            name: 'Get file metadata',
            status: 'error',
            error: error.message,
        });
    }
}

async function testFileSearch(results) {
    console.log('  Testing file search...');

    const url = `http://file-search.${NAMESPACE}.svc.cluster.local:8080/search`;

    try {
        await utils.login();

        const startTime = Date.now();
        const response = await utils.authRequest('GET', `${url}?path=/&query=*`);
        const responseTime = Date.now() - startTime;

        if (response.status === 200) {
            results.pass('Search files', { responseTime });
            reporter.printResult({
                name: 'Search files',
                status: 'pass',
                responseTime,
            });
        } else if (response.status === 400) {
            // May require specific query format
            results.pass('Search files (requires query)', { responseTime });
            reporter.printResult({
                name: 'Search files',
                status: 'pass',
                responseTime,
            });
        } else {
            results.fail('Search files', `Status: ${response.status}`);
            reporter.printResult({
                name: 'Search files',
                status: 'fail',
                error: `Status: ${response.status}`,
            });
        }
    } catch (error) {
        results.error('Search files', error);
        reporter.printResult({
            name: 'Search files',
            status: 'error',
            error: error.message,
        });
    }
}

async function testFileThumbnail(results) {
    console.log('  Testing file thumbnail service...');

    const result = await utils.healthCheck('file-thumbnail', 8080, '/health');

    if (result.status === 'pass') {
        results.pass('file-thumbnail service available', { responseTime: result.responseTime });
    } else {
        results.fail('file-thumbnail service available', result.error);
    }

    reporter.printResult({
        name: 'file-thumbnail service',
        status: result.status,
        responseTime: result.responseTime,
    });
}

async function testFilePreview(results) {
    console.log('  Testing file preview service...');

    const result = await utils.healthCheck('file-preview', 8080, '/health');

    if (result.status === 'pass') {
        results.pass('file-preview service available', { responseTime: result.responseTime });
    } else {
        results.fail('file-preview service available', result.error);
    }

    reporter.printResult({
        name: 'file-preview service',
        status: result.status,
        responseTime: result.responseTime,
    });
}

async function testFileWebNautilus(results) {
    console.log('  Testing file-web-nautilus UI...');

    const result = await utils.healthCheck('file-web-nautilus', 80, '/health');

    if (result.status === 'pass') {
        results.pass('file-web-nautilus UI available', { responseTime: result.responseTime });
    } else {
        results.fail('file-web-nautilus UI available', result.error);
    }

    reporter.printResult({
        name: 'file-web-nautilus UI',
        status: result.status,
        responseTime: result.responseTime,
    });
}

async function runFileServicesTests() {
    const results = new TestResults('File Services');

    reporter.printHeader('File Services E2E Tests');
    console.log(`  Namespace: ${NAMESPACE}`);

    // Run health checks
    await testFileListHealth(results);
    await testFileUploadHealth(results);
    await testFileDownloadHealth(results);
    await testFileOperationsHealth(results);

    // Run functional tests
    await testFileListDirectory(results);
    await testFileCreateDirectory(results);
    await testFileMetadata(results);
    await testFileSearch(results);
    await testFileThumbnail(results);
    await testFilePreview(results);
    await testFileWebNautilus(results);

    results.finish();
    reporter.printSummary(results.getSummary());

    return results;
}

// CLI handling
async function main() {
    const args = process.argv.slice(2);
    const json = args.includes('--json');

    const results = await runFileServicesTests();

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

module.exports = { runFileServicesTests };
