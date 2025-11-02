#!/bin/bash

# Test script to simulate Kubernetes ConfigMap behavior

set -e

echo "=== Testing Flush Manager with ConfigMap-like scenario ==="

# Create test directories
TEST_DIR="/tmp/flush-manager-test-$$"
mkdir -p "$TEST_DIR/conf"
cd "$TEST_DIR"

echo "Test directory: $TEST_DIR"

# Create initial config file
echo "version=1" > "$TEST_DIR/conf/test.conf"
echo "Created initial config file"

# Create a simple test script that the manager will run
cat > "$TEST_DIR/test-app.sh" << 'EOF'
#!/bin/bash
echo "Test app starting (PID: $$)"
trap 'echo "Test app received SIGTERM, exiting..."; exit 0' TERM
while true; do
    sleep 1
done
EOF
chmod +x "$TEST_DIR/test-app.sh"

# Start the manager in the background
echo ""
echo "Starting manager..."
bin/manager -command "$TEST_DIR/test-app.sh" -config "$TEST_DIR/conf/test.conf" &
MANAGER_PID=$!

echo "Manager started with PID: $MANAGER_PID"
echo ""

# Wait for manager to start
sleep 2

echo "=== Manager logs should show initial startup ==="
echo ""

# Simulate ConfigMap update by creating symlink scenario
echo "Simulating ConfigMap update (creating symlink scenario)..."
sleep 2

# Create a new version of the config
mkdir -p "$TEST_DIR/conf/..data_v2"
echo "version=2" > "$TEST_DIR/conf/..data_v2/test.conf"

# Backup original
mv "$TEST_DIR/conf/test.conf" "$TEST_DIR/conf/test.conf.bak"

# Create symlink (simulating ConfigMap behavior)
ln -s "..data_v2/test.conf" "$TEST_DIR/conf/test.conf"

echo "Created symlink for config file"
echo ""

# Wait to see if manager detects the change
echo "Waiting 10 seconds for manager to detect change..."
sleep 10

echo ""
echo "=== Manager should have detected the config change and restarted the process ==="
echo ""

# Clean up
echo "Stopping manager..."
kill -TERM $MANAGER_PID 2>/dev/null || true
wait $MANAGER_PID 2>/dev/null || true

echo ""
echo "=== Test complete ==="
echo "Check the logs above for:"
echo "  1. [flush-manager] prefixes on all log lines"
echo "  2. Detection of symlink"
echo "  3. File change detection"
echo "  4. Process restart"
echo ""
echo "Cleaning up test directory: $TEST_DIR"
rm -rf "$TEST_DIR"
