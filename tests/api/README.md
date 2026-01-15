# HolmOS API Test Suite

Comprehensive API tests for all HolmOS services.

## Quick Start

```bash
# Install dependencies
pip install -r requirements.txt

# Run quick health check
python run_tests.py --quick

# Run all tests
python run_tests.py

# Run tests for specific service
python run_tests.py --service nova

# Generate HTML report
python run_tests.py --quick --html report.html
```

## Services Tested

### Core Services
| Service | Port | Description |
|---------|------|-------------|
| holmos-shell | 30000 | iPhone-style home screen |
| claude-pod | 30001 | AI chat interface |
| app-store | 30002 | AI-powered app generator |
| chat-hub | 30003 | Unified agent messaging |

### AI Agents
| Service | Port | Description |
|---------|------|-------------|
| nova | 30004 | Cluster guardian |
| merchant | 30005 | Request handler |
| pulse | 30006 | Health monitoring |
| gateway | 30008 | Routing |
| scribe | 30860 | Records keeper |
| vault | 30870 | Secret manager |

### Apps
| Service | Port | Description |
|---------|------|-------------|
| clock-app | 30007 | World clock, alarms, timer |
| calculator-app | 30010 | Calculator |
| file-web-nautilus | 30088 | File manager |
| settings-web | 30600 | Settings hub |
| audiobook-web | 30700 | Audiobook TTS |
| terminal-web | 30800 | Web terminal |

### Infrastructure
| Service | Port | Description |
|---------|------|-------------|
| holm-git | 30009 | Git server |
| cicd-controller | 30020 | CI/CD pipeline |
| deploy-controller | 30021 | Auto-deployment |
| cluster-manager | 30502 | Cluster admin |
| backup-dashboard | 30850 | Backup management |
| test-dashboard | 30900 | Service health |
| metrics-dashboard | 30950 | Cluster metrics |
| registry-ui | 31750 | Registry browser |

## Configuration

Set the target host via environment variable:

```bash
export HOLMOS_HOST=192.168.8.197
export HOLMOS_TIMEOUT=10  # Request timeout in seconds
```

## Test Structure

Each service has its own test file following this pattern:

```
test_<service_name>.py
    TestServiceHealth       # Health endpoint tests
    TestServiceAPI          # API endpoint tests
    TestServiceOperations   # Functional tests
    TestServiceUI           # UI/HTML response tests
```

## Running Specific Test Categories

```bash
# Run only health checks
pytest -m health

# Run API tests
pytest -m api

# Run with verbose output
pytest -v

# Run with coverage
pytest --cov=. --cov-report=html
```

## Generating Reports

```bash
# HTML report
python run_tests.py --quick --html health_report.html

# JSON report
python run_tests.py --quick --json health_report.json

# Full pytest HTML report (requires pytest-html)
pytest --html=full_report.html --self-contained-html
```

## CI/CD Integration

The test suite can be integrated into CI/CD pipelines:

```yaml
# Example GitHub Actions workflow
- name: Run HolmOS API Tests
  env:
    HOLMOS_HOST: ${{ secrets.CLUSTER_HOST }}
  run: |
    pip install -r tests/api/requirements.txt
    python tests/api/run_tests.py --quick --json test-results.json
```
