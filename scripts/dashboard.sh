#!/usr/bin/env bash
# Stremiarr Interactive Dashboard
# Provides a real-time view of service health and streaming stats.

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
export COMPOSE_FILE="$PROJECT_DIR/compose/docker-compose.yml"
STATS_URL="http://127.0.0.1:9944/api/stats"

# Colors
RED=$(tput setaf 1)
GREEN=$(tput setaf 2)
YELLOW=$(tput setaf 3)
BLUE=$(tput setaf 4)
CYAN=$(tput setaf 6)
BOLD=$(tput bold)
RESET=$(tput sgr0)
CLEAR=$(tput clear)

# Check dependencies
if ! command -v jq >/dev/null 2>&1; then
    echo "${RED}Error: 'jq' is required but not installed.${RESET}"
    exit 1
fi

get_handoff_stats() {
    # Fetch stats from Handoff API (Basic Auth required)
    # Since this is local, we expect the user to have set ADMIN_PASSWORD in .env
    # We'll try to extract it from the .env file
    local pass
    pass=$(grep "ADMIN_PASSWORD=" "$PROJECT_DIR/compose/.env" | cut -d'=' -f2 || echo "admin")
    
    curl -s -u "admin:$pass" "$STATS_URL" || echo "{}"
}

draw_dashboard() {
    echo "${CLEAR}"
    echo "${BOLD}${CYAN}🚀 Stremiarr Production Dashboard${RESET} | $(date)"
    echo "--------------------------------------------------------------------------------"

    # 1. Docker Status
    echo "${BOLD}${BLUE}[1] Service Status (Docker Compose)${RESET}"
    cd "$PROJECT_DIR/compose"
    docker compose ps --format "table {{.Name}}\t{{.Status}}\t{{.Health}}" | sed 's/^/  /'
    echo ""

    # 2. Handoff Metrics
    echo "${BOLD}${BLUE}[2] Handoff Streaming Metrics${RESET}"
    local stats
    stats=$(get_handoff_stats)
    
    if [ "$(echo "$stats" | jq 'empty' 2>&1)" ]; then
        echo "  ${RED}Failed to connect to Handoff API at $STATS_URL${RESET}"
    else
        local total
        total=$(echo "$stats" | jq -r '.totalStreams // 0')
        local valid
        valid=$(echo "$stats" | jq -r '.validStreams // 0')
        local failed
        failed=$(echo "$stats" | jq -r '.failedStreams // 0')
        local cached
        cached=$(echo "$stats" | jq -r '.cachedRequests // 0')
        local recent
        recent=$(echo "$stats" | jq -r '.recentValidations // 0')
        local uptime
        uptime=$(echo "$stats" | jq -r '.uptime // "Unknown"')

        printf "  %-20s %s\n" "Total Streams:" "${CYAN}$total${RESET}"
        printf "  %-20s %s\n" "Valid Streams:" "${GREEN}$valid${RESET}"
        printf "  %-20s %s\n" "Failed/Redacted:" "${RED}$failed${RESET}"
        printf "  %-20s %s\n" "Cached Requests:" "${YELLOW}$cached${RESET}"
        printf "  %-20s %s\n" "Recent Probes:" "${BLUE}$recent${RESET}"
        printf "  %-20s %s\n" "Service Uptime:" "$uptime"
    fi
    echo ""

    # 3. System Load (Brief)
    echo "${BOLD}${BLUE}[3] System Info${RESET}"
    printf "  %-20s %s\n" "Load Average:" "$(uptime | awk -F'load average:' '{ print $2 }')"
    printf "  %-20s %s\n" "Memory Usage:" "$(free -h | awk '/^Mem:/ {print $3 "/" $2}')"
    printf "  %-20s %s\n" "Disk (Cache):" "$(du -sh "$PROJECT_DIR/cache/rclone" 2>/dev/null | awk '{print $1}' || echo "0B")"
    echo ""

    echo "--------------------------------------------------------------------------------"
    echo "Press ${BOLD}Ctrl+C${RESET} to exit. Refreshing every 2s..."
}

# Trap Ctrl+C
trap 'echo -e "\n${YELLOW}Exiting dashboard...${RESET}"; exit' SIGINT

# Main Loop
while true; do
    draw_dashboard
    sleep 2
done
