#!/bin/sh
set -e

# arkhe-go entrypoint — first-run setup for gateway containers.
#
# If neither config.json nor workspace exists this is a fresh container.
# Run onboard to bootstrap defaults, then exit so the user can configure
# their API key before the gateway actually starts.

if [ ! -d "${HOME}/.picoclaw/workspace" ] && [ ! -f "${HOME}/.picoclaw/config.json" ]; then
    picoclaw onboard
    echo ""
    echo "======================================"
    echo "  arkhe-go — first-run setup complete"
    echo "======================================"
    echo ""
    echo "Edit ${HOME}/.picoclaw/config.json (add your API key, etc.) then restart the container."
    exit 0
fi

exec picoclaw gateway "$@"
