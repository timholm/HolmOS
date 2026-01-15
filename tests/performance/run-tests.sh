#!/bin/bash

# HolmOS Performance Test Runner
# Runs k6 load tests against the Raspberry Pi cluster

set -e

# Configuration
CLUSTER_IP="${CLUSTER_IP:-192.168.8.197}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
RESULTS_DIR="${SCRIPT_DIR}/results"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Ensure results directory exists
mkdir -p "${RESULTS_DIR}"

# Print banner
echo -e "${BLUE}"
echo "========================================"
echo "   HolmOS Performance Test Suite"
echo "   Cluster: ${CLUSTER_IP}"
echo "   Timestamp: ${TIMESTAMP}"
echo "========================================"
echo -e "${NC}"

# Check if k6 is installed
if ! command -v k6 &> /dev/null; then
    echo -e "${RED}Error: k6 is not installed${NC}"
    echo "Install k6:"
    echo "  macOS: brew install k6"
    echo "  Linux: sudo snap install k6"
    echo "  Docker: docker run --rm -i grafana/k6 run -"
    exit 1
fi

# Check cluster connectivity
echo -e "${YELLOW}Checking cluster connectivity...${NC}"
if ! curl -s --connect-timeout 5 "http://${CLUSTER_IP}:30000/health" > /dev/null 2>&1; then
    echo -e "${RED}Warning: Cannot reach cluster at ${CLUSTER_IP}:30000${NC}"
    echo "Some tests may fail if services are not available"
fi

# Function to run a test
run_test() {
    local test_name=$1
    local test_file=$2
    local load_profile=${3:-standard}
    local extra_args=${4:-}

    echo -e "\n${BLUE}Running: ${test_name} (profile: ${load_profile})${NC}"
    echo "----------------------------------------"

    local output_file="${RESULTS_DIR}/${test_name}_${TIMESTAMP}.json"

    if k6 run \
        --env LOAD_PROFILE="${load_profile}" \
        --out json="${output_file}" \
        ${extra_args} \
        "${SCRIPT_DIR}/${test_file}" 2>&1; then
        echo -e "${GREEN}PASSED${NC}: ${test_name}"
        return 0
    else
        echo -e "${RED}FAILED${NC}: ${test_name}"
        return 1
    fi
}

# Parse command line arguments
TEST_TYPE="${1:-all}"
LOAD_PROFILE="${2:-standard}"

case "${TEST_TYPE}" in
    smoke)
        echo -e "${YELLOW}Running smoke tests...${NC}"
        LOAD_PROFILE="smoke"
        run_test "health-smoke" "health-check.test.js" "smoke"
        ;;

    health)
        echo -e "${YELLOW}Running health check tests...${NC}"
        run_test "health-check" "health-check.test.js" "${LOAD_PROFILE}"
        ;;

    core)
        echo -e "${YELLOW}Running core services tests...${NC}"
        run_test "core-services" "core-services.test.js" "${LOAD_PROFILE}"
        ;;

    agents)
        echo -e "${YELLOW}Running agent tests...${NC}"
        run_test "agents" "agents.test.js" "${LOAD_PROFILE}"
        ;;

    apps)
        echo -e "${YELLOW}Running app tests...${NC}"
        run_test "apps" "apps.test.js" "${LOAD_PROFILE}"
        ;;

    infrastructure)
        echo -e "${YELLOW}Running infrastructure tests...${NC}"
        run_test "infrastructure" "infrastructure.test.js" "${LOAD_PROFILE}"
        ;;

    stress)
        echo -e "${YELLOW}Running stress tests...${NC}"
        run_test "stress" "stress.test.js" "stress"
        ;;

    spike)
        echo -e "${YELLOW}Running spike tests...${NC}"
        run_test "spike" "spike.test.js" "spike"
        ;;

    full)
        echo -e "${YELLOW}Running full test suite...${NC}"
        FAILED=0

        run_test "health-check" "health-check.test.js" "${LOAD_PROFILE}" || FAILED=$((FAILED+1))
        run_test "core-services" "core-services.test.js" "${LOAD_PROFILE}" || FAILED=$((FAILED+1))
        run_test "agents" "agents.test.js" "${LOAD_PROFILE}" || FAILED=$((FAILED+1))
        run_test "apps" "apps.test.js" "${LOAD_PROFILE}" || FAILED=$((FAILED+1))
        run_test "infrastructure" "infrastructure.test.js" "${LOAD_PROFILE}" || FAILED=$((FAILED+1))

        echo -e "\n${BLUE}========================================${NC}"
        if [ ${FAILED} -eq 0 ]; then
            echo -e "${GREEN}All tests passed!${NC}"
        else
            echo -e "${RED}${FAILED} test(s) failed${NC}"
            exit 1
        fi
        ;;

    all)
        echo -e "${YELLOW}Running complete test suite (including stress/spike)...${NC}"
        FAILED=0

        run_test "health-check" "health-check.test.js" "light" || FAILED=$((FAILED+1))
        run_test "core-services" "core-services.test.js" "${LOAD_PROFILE}" || FAILED=$((FAILED+1))
        run_test "agents" "agents.test.js" "${LOAD_PROFILE}" || FAILED=$((FAILED+1))
        run_test "apps" "apps.test.js" "${LOAD_PROFILE}" || FAILED=$((FAILED+1))
        run_test "infrastructure" "infrastructure.test.js" "light" || FAILED=$((FAILED+1))
        run_test "stress" "stress.test.js" "stress" || FAILED=$((FAILED+1))
        run_test "spike" "spike.test.js" "spike" || FAILED=$((FAILED+1))

        echo -e "\n${BLUE}========================================${NC}"
        if [ ${FAILED} -eq 0 ]; then
            echo -e "${GREEN}All tests passed!${NC}"
        else
            echo -e "${RED}${FAILED} test(s) failed${NC}"
            exit 1
        fi
        ;;

    *)
        echo "Usage: $0 [test_type] [load_profile]"
        echo ""
        echo "Test types:"
        echo "  smoke          - Quick functionality check"
        echo "  health         - Health endpoint tests"
        echo "  core           - Core services (shell, claude-pod, etc.)"
        echo "  agents         - AI agent services"
        echo "  apps           - Application services"
        echo "  infrastructure - DevOps and monitoring services"
        echo "  stress         - Stress testing"
        echo "  spike          - Spike/burst testing"
        echo "  full           - All functional tests"
        echo "  all            - Complete suite including stress/spike"
        echo ""
        echo "Load profiles:"
        echo "  smoke    - 1 VU, 30s"
        echo "  light    - 5 VUs, 2m"
        echo "  standard - 10-20 VUs, 4.5m"
        echo "  stress   - 10-50 VUs, 7m"
        echo "  spike    - 5-50-5 VUs, 2.5m"
        echo "  soak     - 10 VUs, 12m"
        echo ""
        echo "Examples:"
        echo "  $0 smoke           # Quick smoke test"
        echo "  $0 core light      # Core services with light load"
        echo "  $0 full standard   # All tests with standard load"
        echo "  $0 stress          # Stress test"
        exit 1
        ;;
esac

# Generate summary report
echo -e "\n${BLUE}Generating summary report...${NC}"
echo "Results saved to: ${RESULTS_DIR}/"

# List result files
ls -la "${RESULTS_DIR}"/*_${TIMESTAMP}*.json 2>/dev/null || true

echo -e "\n${GREEN}Performance testing complete!${NC}"
