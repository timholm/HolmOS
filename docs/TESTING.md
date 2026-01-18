# HolmOS Testing Guide

## Overview

HolmOS uses a comprehensive testing strategy with multiple test suites:
- **Smoke Tests** - Quick validation of core services
- **API Tests** - Test individual service endpoints
- **E2E Tests** - Full user journey tests
- **Integration Tests** - Cross-service interaction tests
- **Load Tests** - Performance under load
- **Performance Tests** - Response time benchmarks

## Quick Start

```bash
# Install dependencies
cd tests
npm install

# Run smoke tests (recommended for quick validation)
npm run test:smoke

# Run all tests (excluding load tests)
npm test

# Run all tests including load tests
npm run test:all
```

## Test Commands

### Makefile Commands

```bash
make test              # Run all tests
make smoke             # Run smoke tests (3 core tests)
make smoke-quick       # Quick single service check
make smoke-degraded    # Allow partial failures
```

### NPM Scripts

```bash
npm test               # All tests (skip load)
npm run test:all       # All tests including load
npm run test:quick     # Quick subset
npm run test:smoke     # Smoke tests only
npm run test:smoke:quick     # Fast smoke check
npm run test:smoke:degraded  # Allow failures
npm run test:e2e       # E2E tests only
npm run test:integration     # Integration tests only
npm run test:load      # Load tests only
npm run test:health    # Health check tests
npm run test:auth      # Auth tests
npm run test:files     # File service tests
npm run test:metrics   # Metrics tests
npm run test:notifications   # Notification tests
npm run test:json      # Output as JSON
npm run test:report    # Generate HTML report
npm run report         # View latest report
```

## Test Structure

```
tests/
├── api/                    # API endpoint tests
│   ├── auth.test.js
│   ├── files.test.js
│   ├── gateway.test.js
│   └── ...
├── e2e/                    # End-to-end tests
│   ├── health-checks.test.js
│   ├── auth-gateway.test.js
│   ├── file-services.test.js
│   ├── metrics-dashboard.test.js
│   └── notification-hub.test.js
├── integration/            # Integration tests
│   ├── chat-hub.test.js
│   └── ...
├── load/                   # Load/stress tests
│   └── stress.test.js
├── performance/            # Performance benchmarks
│   ├── baseline/
│   └── reports/
├── smoke/                  # Smoke tests
│   └── smoke-tests.js
├── utils/                  # Test utilities
│   └── helpers.js
├── config.js               # Test configuration
├── runner.js               # Main test runner
├── report-generator.js     # Report generation
└── package.json
```

## Writing Tests

### API Test Example

```javascript
// tests/api/my-service.test.js
const axios = require('axios');
const { BASE_URL } = require('../config');

describe('My Service API', () => {
    const serviceUrl = `${BASE_URL}:30XXX`;

    test('health check returns ok', async () => {
        const response = await axios.get(`${serviceUrl}/health`);
        expect(response.status).toBe(200);
        expect(response.data.status).toBe('ok');
    });

    test('main endpoint works', async () => {
        const response = await axios.get(`${serviceUrl}/api/data`);
        expect(response.status).toBe(200);
        expect(response.data).toBeDefined();
    });
});
```

### E2E Test Example

```javascript
// tests/e2e/user-journey.test.js
const axios = require('axios');
const { BASE_URL, waitFor } = require('../config');

describe('User Journey', () => {
    test('user can login and access dashboard', async () => {
        // Login
        const loginResponse = await axios.post(`${BASE_URL}:30008/auth/login`, {
            username: 'admin',
            password: 'password'
        });
        expect(loginResponse.status).toBe(200);
        const { token } = loginResponse.data;

        // Access dashboard
        const dashboardResponse = await axios.get(`${BASE_URL}:30004/api/status`, {
            headers: { Authorization: `Bearer ${token}` }
        });
        expect(dashboardResponse.status).toBe(200);
    });
});
```

### Smoke Test Structure

```javascript
// tests/smoke/smoke-tests.js
const tests = [
    {
        name: 'holmos-shell',
        url: 'http://192.168.8.197:30000/health',
        critical: true
    },
    {
        name: 'claude-pod',
        url: 'http://192.168.8.197:30001/health',
        critical: true
    },
    {
        name: 'chat-hub',
        url: 'http://192.168.8.197:30003/health',
        critical: true
    }
];
```

## Configuration

### tests/config.js

```javascript
module.exports = {
    BASE_URL: process.env.BASE_URL || 'http://192.168.8.197',
    TIMEOUT: 30000,
    RETRIES: 3,

    services: {
        shell: { port: 30000, critical: true },
        claude: { port: 30001, critical: true },
        chatHub: { port: 30003, critical: true },
        nova: { port: 30004, critical: false },
        // ... more services
    },

    // Test suite configuration
    suites: {
        smoke: ['health-checks'],
        quick: ['health-checks', 'auth'],
        full: ['health-checks', 'auth', 'files', 'metrics', 'notifications']
    }
};
```

## Running in CI/CD

Tests run automatically on:
- Pull request creation
- Push to main branch
- Manual trigger via GitHub Actions

### GitHub Actions Workflow

```yaml
# .github/workflows/test.yml
name: Tests
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
      - run: cd tests && npm install
      - run: cd tests && npm run test:smoke
```

## Test Reports

### Generate Report

```bash
npm run test:report
```

This creates an HTML report in `tests/reports/`.

### View Report

```bash
npm run report
# or open tests/reports/latest.html
```

### JSON Output

```bash
npm run test:json > results.json
```

## Debugging Tests

### Verbose Output

```bash
DEBUG=true npm test
```

### Single Test File

```bash
node tests/e2e/health-checks.test.js
```

### Increase Timeout

```bash
TIMEOUT=60000 npm test
```

## Best Practices

1. **Always run smoke tests** before deploying
2. **Keep tests independent** - don't rely on test order
3. **Use meaningful assertions** - test behavior, not implementation
4. **Handle async properly** - use async/await
5. **Clean up after tests** - delete test data
6. **Mock external services** when appropriate
7. **Test error cases** - not just happy path

## Troubleshooting

### Tests Timing Out

```bash
# Increase timeout
TIMEOUT=60000 npm test

# Check service health
make health
```

### Connection Refused

```bash
# Verify service is running
make pods
kubectl get pods -n holm | grep my-service

# Check logs
make logs S=my-service
```

### Flaky Tests

- Add retries in test config
- Use `waitFor` helper for async operations
- Check for race conditions
- Ensure test isolation
