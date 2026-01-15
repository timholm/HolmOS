/**
 * HolmOS Test Framework
 *
 * Unified exports for all test modules
 */

const config = require('./config');
const { TestUtils, TestResults, ConsoleReporter } = require('./utils/test-utils');

// E2E Tests
const { runHealthChecks, runCategoryHealthChecks, runQuickHealthCheck } = require('./e2e/health-checks.test');
const { runAuthGatewayTests } = require('./e2e/auth-gateway.test');
const { runFileServicesTests } = require('./e2e/file-services.test');
const { runMetricsDashboardTests } = require('./e2e/metrics-dashboard.test');
const { runNotificationHubTests } = require('./e2e/notification-hub.test');

// Integration Tests
const { runAuthFilesIntegrationTests } = require('./integration/auth-files.test');
const { runMetricsNotificationsIntegrationTests } = require('./integration/metrics-notifications.test');
const { runFullStackIntegrationTests } = require('./integration/full-stack.test');

// Load Tests
const {
    LoadTestRunner,
    runHealthEndpointLoad,
    runMetricsDashboardLoad,
    runNotificationHubLoad,
    runFileServiceLoad,
    runAllLoadTests,
} = require('./load/load-test');

// Report Generator
const { ReportGenerator } = require('./report-generator');

// Test Runner
const { TestRunner } = require('./runner');

module.exports = {
    // Configuration
    config,

    // Utilities
    TestUtils,
    TestResults,
    ConsoleReporter,

    // E2E Tests
    e2e: {
        runHealthChecks,
        runCategoryHealthChecks,
        runQuickHealthCheck,
        runAuthGatewayTests,
        runFileServicesTests,
        runMetricsDashboardTests,
        runNotificationHubTests,
    },

    // Integration Tests
    integration: {
        runAuthFilesIntegrationTests,
        runMetricsNotificationsIntegrationTests,
        runFullStackIntegrationTests,
    },

    // Load Tests
    load: {
        LoadTestRunner,
        runHealthEndpointLoad,
        runMetricsDashboardLoad,
        runNotificationHubLoad,
        runFileServiceLoad,
        runAllLoadTests,
    },

    // Report Generator
    ReportGenerator,

    // Test Runner
    TestRunner,
};
