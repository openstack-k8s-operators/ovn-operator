#!/usr/bin/env bash
#
# OVN Database Runtime Configuration Script
# Handles election timer, log level, and inactivity probe changes
#
set -e

DB_TYPE="{{ .DB_TYPE }}"
DB_PORT="{{ .DB_PORT }}"
{{- if .TLS }}
DB_SCHEME="pssl"
{{- else }}
DB_SCHEME="ptcp"
{{- end }}
SERVICE_NAME="{{ .SERVICE_NAME }}"

# Set DB_NAME based on DB_TYPE (same logic as setup.sh)
DB_NAME="OVN_Northbound"
if [[ "${DB_TYPE}" == "sb" ]]; then
    DB_NAME="OVN_Southbound"
fi

# Runtime configuration parameters
ELECTION_TIMER="${ELECTION_TIMER:-10000}"
LOG_LEVEL="${LOG_LEVEL:-info}"
INACTIVITY_PROBE="${INACTIVITY_PROBE:-60000}"

# Configuration flags (space-separated list of what to configure)
CONFIG_FLAGS="${CONFIG_FLAGS:-}"

# Set OVN_RUNDIR so ovn-ctl commands use correct paths
export OVN_RUNDIR="/etc/ovn"

SOCKET_PATH="/etc/ovn/ovn${DB_TYPE}_db.ctl"

echo "=== OVN Runtime Configuration ==="
echo "Pod: $(hostname)"
echo "DB Type: ${DB_TYPE}"
echo "Socket: ${SOCKET_PATH}"
echo "Configurations to apply: ${CONFIG_FLAGS}"
echo "=================================="

# Check if control socket exists
if [ ! -S "$SOCKET_PATH" ]; then
    echo "ERROR: Control socket $SOCKET_PATH not available"
    echo "Contents of /etc/ovn directory:"
    ls -la /etc/ovn/ || true
    exit 1
fi

# Configure Election Timer
if echo "$CONFIG_FLAGS" | grep -q "ELECTION_TIMER"; then
    echo "Configuring election timer to ${ELECTION_TIMER}ms..."
    if ovs-appctl -t "$SOCKET_PATH" cluster/change-election-timer "$DB_NAME" "$ELECTION_TIMER"; then
        echo "✓ Successfully configured election timer"
    else
        echo "⚠ Failed to configure election timer (expected if not leader)"
    fi
fi

# Configure Log Level
if echo "$CONFIG_FLAGS" | grep -q "LOG_LEVEL"; then
    echo "Configuring log level to ${LOG_LEVEL}..."
    if ovn-appctl -t "$SOCKET_PATH" vlog/set "console:${LOG_LEVEL}"; then
        echo "✓ Successfully configured log level"
    else
        echo "✗ Failed to configure log level"
        exit 1
    fi
fi

# Configure Inactivity Probe (only on pod-0)
if echo "$CONFIG_FLAGS" | grep -q "INACTIVITY_PROBE"; then
    if [[ "$(hostname)" == "${SERVICE_NAME}-0-config-"* ]]; then
        echo "Configuring inactivity probe to ${INACTIVITY_PROBE}ms on pod-0..."

        # Simple approach - connect directly to running database (OVN_RUNDIR already set)
        if ovn-${DB_TYPE}ctl --no-leader-only set connection . inactivity_probe="$INACTIVITY_PROBE"; then
            echo "✓ Successfully configured inactivity probe"
        else
            echo "✗ Failed to configure inactivity probe"
            exit 1
        fi
    else
        echo "⚠ Skipping inactivity probe configuration (not pod-0)"
    fi
fi

echo "=== Configuration completed successfully ==="
