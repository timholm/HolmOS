#!/bin/bash
#
# HolmOS Smoke Test Script
# Local testing script for HolmOS service health and functionality
#
# Usage:
#   ./smoke-test.sh              # Test all services
#   ./smoke-test.sh nova pulse   # Test specific services
#   ./smoke-test.sh --quick      # Quick health-only test
#   ./smoke-test.sh --json       # Output as JSON
#   ./smoke-test.sh --help       # Show help
#

set -euo pipefail

# Configuration
CLUSTER_IP="${CLUSTER_IP:-192.168.8.197}"
TIMEOUT="${TIMEOUT:-10}"
CONNECT_TIMEOUT="${CONNECT_TIMEOUT:-5}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Service registry (from services.yaml)
declare -A SERVICES=(
    # Core Entry Points
    ["holmos-shell"]="30000:core"
    ["claude-pod"]="30001:core"
    ["app-store"]="30002:core"
    ["chat-hub"]="30003:core"
    # AI Agents
    ["nova"]="30004:agent"
    ["merchant"]="30005:agent"
    ["pulse"]="30006:agent"
    ["gateway"]="30008:agent"
    ["scribe"]="30860:agent"
    ["vault"]="30870:agent"
    # Apps
    ["clock-app"]="30007:app"
    ["calculator-app"]="30010:app"
    ["file-web-nautilus"]="30088:app"
    ["settings-web"]="30600:app"
    ["audiobook-web"]="30700:app"
    ["terminal-web"]="30800:app"
    # Infrastructure/DevOps
    ["holm-git"]="30009:devops"
    ["cicd-controller"]="30020:devops"
    ["deploy-controller"]="30021:devops"
    ["cluster-manager"]="30502:admin"
    ["backup-dashboard"]="30850:admin"
    ["test-dashboard"]="30900:monitoring"
    ["metrics-dashboard"]="30950:monitoring"
    ["registry-ui"]="31750:devops"
)

# Counters
PASSED=0
FAILED=0
SKIPPED=0

# Options
QUICK_MODE=false
JSON_OUTPUT=false
VERBOSE=false

# Arrays for results
declare -a PASSED_SERVICES=()
declare -a FAILED_SERVICES=()
declare -a SKIPPED_SERVICES=()
declare -a JSON_RESULTS=()

usage() {
    cat << EOF
HolmOS Smoke Test Script

Usage: $(basename "$0") [OPTIONS] [SERVICE...]

Options:
    -h, --help          Show this help message
    -q, --quick         Quick mode: health checks only
    -j, --json          Output results as JSON
    -v, --verbose       Verbose output
    -c, --category CAT  Test only services in category (core|agent|app|devops|admin|monitoring)
    --cluster IP        Override cluster IP (default: $CLUSTER_IP)
    --timeout SEC       Request timeout in seconds (default: $TIMEOUT)

Examples:
    $(basename "$0")                    # Test all services
    $(basename "$0") nova pulse         # Test specific services
    $(basename "$0") -q                 # Quick health check
    $(basename "$0") -c agent           # Test all AI agents
    $(basename "$0") --json             # JSON output for CI/CD

Environment Variables:
    CLUSTER_IP          Cluster IP address (default: 192.168.8.197)
    TIMEOUT             Request timeout (default: 10)
    CONNECT_TIMEOUT     Connection timeout (default: 5)

EOF
    exit 0
}

log() {
    if [ "$JSON_OUTPUT" = false ]; then
        echo -e "$@"
    fi
}

log_verbose() {
    if [ "$VERBOSE" = true ] && [ "$JSON_OUTPUT" = false ]; then
        echo -e "  ${BLUE}[DEBUG]${NC} $@"
    fi
}

# HTTP request helper
http_check() {
    local url="$1"
    local response
    response=$(curl -s -o /dev/null -w "%{http_code}" \
        --connect-timeout "$CONNECT_TIMEOUT" \
        --max-time "$TIMEOUT" \
        "$url" 2>/dev/null || echo "000")
    echo "$response"
}

# Get response body
http_get() {
    local url="$1"
    curl -s --connect-timeout "$CONNECT_TIMEOUT" --max-time "$TIMEOUT" "$url" 2>/dev/null || echo ""
}

# Test health endpoint
test_health() {
    local service="$1"
    local port="$2"
    local url="http://${CLUSTER_IP}:${port}/health"

    local status
    status=$(http_check "$url")

    if [ "$status" = "200" ]; then
        log "  ${GREEN}[PASS]${NC} Health check (HTTP $status)"
        return 0
    else
        log "  ${RED}[FAIL]${NC} Health check (HTTP $status)"
        return 1
    fi
}

# Test basic connectivity
test_connectivity() {
    local service="$1"
    local port="$2"
    local url="http://${CLUSTER_IP}:${port}/"

    local status
    status=$(http_check "$url")

    if [ "$status" != "000" ] && [ "$status" != "502" ] && [ "$status" != "503" ]; then
        log "  ${GREEN}[PASS]${NC} Connectivity (HTTP $status)"
        return 0
    else
        log "  ${RED}[FAIL]${NC} Connectivity (HTTP $status)"
        return 1
    fi
}

# Service-specific functional tests
test_functional() {
    local service="$1"
    local port="$2"
    local url="http://${CLUSTER_IP}:${port}"

    case $service in
        nova|pulse|merchant|gateway|scribe|vault)
            # AI Agents - test API status endpoint
            local api_status
            api_status=$(http_check "${url}/api/status")
            if [ "$api_status" = "000" ]; then
                api_status=$(http_check "${url}/api/info")
            fi
            if [ "$api_status" = "200" ]; then
                log "  ${GREEN}[PASS]${NC} API status (HTTP $api_status)"
                return 0
            else
                log "  ${YELLOW}[WARN]${NC} API status (HTTP $api_status)"
                return 0  # Don't fail on optional endpoints
            fi
            ;;

        calculator-app)
            local calc_response
            calc_response=$(http_get "${url}/api/calculate?expr=2%2B2")
            if echo "$calc_response" | grep -q "4" 2>/dev/null; then
                log "  ${GREEN}[PASS]${NC} Calculator: 2+2=4"
                return 0
            else
                log "  ${YELLOW}[SKIP]${NC} Calculator API not available"
                return 0
            fi
            ;;

        clock-app)
            local time_status
            time_status=$(http_check "${url}/api/time")
            if [ "$time_status" = "200" ]; then
                log "  ${GREEN}[PASS]${NC} Time API (HTTP $time_status)"
            else
                log "  ${YELLOW}[SKIP]${NC} Time API (HTTP $time_status)"
            fi
            return 0
            ;;

        holm-git)
            local repos_status
            repos_status=$(http_check "${url}/api/repos")
            if [ "$repos_status" = "200" ]; then
                log "  ${GREEN}[PASS]${NC} Repos API (HTTP $repos_status)"
            else
                log "  ${YELLOW}[SKIP]${NC} Repos API (HTTP $repos_status)"
            fi
            return 0
            ;;

        metrics-dashboard)
            local metrics_status
            metrics_status=$(http_check "${url}/api/metrics")
            if [ "$metrics_status" = "200" ]; then
                log "  ${GREEN}[PASS]${NC} Metrics API (HTTP $metrics_status)"
            else
                log "  ${YELLOW}[SKIP]${NC} Metrics API (HTTP $metrics_status)"
            fi
            return 0
            ;;

        registry-ui)
            local catalog_status
            catalog_status=$(http_check "${url}/api/catalog")
            if [ "$catalog_status" = "200" ]; then
                log "  ${GREEN}[PASS]${NC} Catalog API (HTTP $catalog_status)"
            else
                log "  ${YELLOW}[SKIP]${NC} Catalog API (HTTP $catalog_status)"
            fi
            return 0
            ;;

        test-dashboard)
            local tests_status
            tests_status=$(http_check "${url}/api/tests")
            if [ "$tests_status" = "200" ]; then
                log "  ${GREEN}[PASS]${NC} Tests API (HTTP $tests_status)"
            else
                log "  ${YELLOW}[SKIP]${NC} Tests API (HTTP $tests_status)"
            fi
            return 0
            ;;

        terminal-web)
            local ws_status
            ws_status=$(http_check "${url}/api/terminals")
            if [ "$ws_status" = "200" ]; then
                log "  ${GREEN}[PASS]${NC} Terminals API (HTTP $ws_status)"
            else
                log "  ${YELLOW}[SKIP]${NC} Terminals API (HTTP $ws_status)"
            fi
            return 0
            ;;

        *)
            log "  ${YELLOW}[SKIP]${NC} No functional tests defined"
            return 0
            ;;
    esac
}

# Run all tests for a service
test_service() {
    local service="$1"
    local port_info="${SERVICES[$service]:-}"

    if [ -z "$port_info" ]; then
        log "${YELLOW}[SKIP]${NC} $service - Unknown service"
        SKIPPED_SERVICES+=("$service")
        ((SKIPPED++))
        return 0
    fi

    local port="${port_info%%:*}"
    local category="${port_info##*:}"

    log ""
    log "${BLUE}========================================${NC}"
    log "Testing: ${BLUE}$service${NC} (port $port, $category)"
    log "${BLUE}========================================${NC}"

    local health_ok=false
    local connect_ok=false

    # Test 1: Health check
    if test_health "$service" "$port"; then
        health_ok=true
    fi

    # Test 2: Connectivity
    if test_connectivity "$service" "$port"; then
        connect_ok=true
    fi

    # Test 3: Functional tests (if not quick mode)
    if [ "$QUICK_MODE" = false ]; then
        test_functional "$service" "$port"
    fi

    # Determine result
    if [ "$health_ok" = true ] && [ "$connect_ok" = true ]; then
        log "  ${GREEN}>>> PASSED${NC}"
        PASSED_SERVICES+=("$service")
        ((PASSED++))

        if [ "$JSON_OUTPUT" = true ]; then
            JSON_RESULTS+=("{\"service\":\"$service\",\"port\":$port,\"category\":\"$category\",\"status\":\"pass\"}")
        fi
        return 0
    else
        log "  ${RED}>>> FAILED${NC}"
        FAILED_SERVICES+=("$service")
        ((FAILED++))

        if [ "$JSON_OUTPUT" = true ]; then
            JSON_RESULTS+=("{\"service\":\"$service\",\"port\":$port,\"category\":\"$category\",\"status\":\"fail\"}")
        fi
        return 1
    fi
}

# Print summary
print_summary() {
    if [ "$JSON_OUTPUT" = true ]; then
        local results
        results=$(printf '%s,' "${JSON_RESULTS[@]}" | sed 's/,$//')
        cat << EOF
{
  "cluster": "$CLUSTER_IP",
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "summary": {
    "passed": $PASSED,
    "failed": $FAILED,
    "skipped": $SKIPPED,
    "total": $((PASSED + FAILED + SKIPPED))
  },
  "passed_services": $(printf '%s\n' "${PASSED_SERVICES[@]}" | jq -R -s -c 'split("\n") | map(select(length > 0))'),
  "failed_services": $(printf '%s\n' "${FAILED_SERVICES[@]}" | jq -R -s -c 'split("\n") | map(select(length > 0))'),
  "results": [$results]
}
EOF
        return
    fi

    log ""
    log "${BLUE}========================================"
    log "         SMOKE TEST SUMMARY"
    log "========================================${NC}"
    log ""
    log "Cluster: $CLUSTER_IP"
    log "Time: $(date)"
    log ""
    log "${GREEN}Passed:${NC}  $PASSED"
    log "${RED}Failed:${NC}  $FAILED"
    log "${YELLOW}Skipped:${NC} $SKIPPED"
    log ""

    if [ ${#PASSED_SERVICES[@]} -gt 0 ]; then
        log "${GREEN}Passed services:${NC}"
        for svc in "${PASSED_SERVICES[@]}"; do
            log "  - $svc"
        done
    fi

    if [ ${#FAILED_SERVICES[@]} -gt 0 ]; then
        log ""
        log "${RED}Failed services:${NC}"
        for svc in "${FAILED_SERVICES[@]}"; do
            log "  - $svc"
        done
    fi

    log ""
    if [ $FAILED -eq 0 ]; then
        log "${GREEN}All tests passed!${NC}"
    else
        log "${RED}Some tests failed. Check the failed services above.${NC}"
    fi
}

# Main
main() {
    local category_filter=""
    local services_to_test=()

    # Parse arguments
    while [[ $# -gt 0 ]]; do
        case $1 in
            -h|--help)
                usage
                ;;
            -q|--quick)
                QUICK_MODE=true
                shift
                ;;
            -j|--json)
                JSON_OUTPUT=true
                shift
                ;;
            -v|--verbose)
                VERBOSE=true
                shift
                ;;
            -c|--category)
                category_filter="$2"
                shift 2
                ;;
            --cluster)
                CLUSTER_IP="$2"
                shift 2
                ;;
            --timeout)
                TIMEOUT="$2"
                shift 2
                ;;
            -*)
                echo "Unknown option: $1"
                usage
                ;;
            *)
                services_to_test+=("$1")
                shift
                ;;
        esac
    done

    # Determine services to test
    if [ ${#services_to_test[@]} -eq 0 ]; then
        # Test all services (or filtered by category)
        for svc in "${!SERVICES[@]}"; do
            if [ -n "$category_filter" ]; then
                local port_info="${SERVICES[$svc]}"
                local category="${port_info##*:}"
                if [ "$category" = "$category_filter" ]; then
                    services_to_test+=("$svc")
                fi
            else
                services_to_test+=("$svc")
            fi
        done
    fi

    # Sort services by port number for consistent output
    IFS=$'\n' services_to_test=($(for svc in "${services_to_test[@]}"; do
        port_info="${SERVICES[$svc]:-999999:unknown}"
        port="${port_info%%:*}"
        echo "$port $svc"
    done | sort -n | awk '{print $2}'))
    unset IFS

    if [ "$JSON_OUTPUT" = false ]; then
        log "${BLUE}========================================"
        log "     HolmOS Smoke Test Suite"
        log "========================================${NC}"
        log ""
        log "Cluster IP: $CLUSTER_IP"
        log "Mode: $([ "$QUICK_MODE" = true ] && echo "Quick" || echo "Full")"
        log "Services: ${#services_to_test[@]}"
    fi

    # Run tests
    for svc in "${services_to_test[@]}"; do
        test_service "$svc" || true
    done

    # Print summary
    print_summary

    # Exit with appropriate code
    if [ $FAILED -gt 0 ]; then
        exit 1
    fi
    exit 0
}

main "$@"
