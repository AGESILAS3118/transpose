#!/usr/bin/env bash
# scripts/test-integration.sh
# End-to-end integration tests for Transpose (xpose) inside a clean Fedora 44 container.
# Validates tests A to F:
#   A: Interactive apply with script -qec (TTY allocated, prompt confirmations)
#   B: Non-interactive apply (--apply --yes)
#   C: Resume execution after interruption (--resume --apply --yes)
#   D: Hermetic dry-run (no packages installed, rpm -qa unchanged)
#   E: Destructive plan rejection (safety guardrail)
#   F: Third-party repo interactive confirmation rejection (n)

set -euo pipefail

RUNTIME="${1:-podman}"
IMAGE="fedora:44"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

echo "=== Transpose Integration Tests ==="
echo "Runtime  : ${RUNTIME}"
echo "Image    : ${IMAGE}"
echo "Root dir : ${ROOT_DIR}"

if ! command -v "${RUNTIME}" >/dev/null 2>&1; then
    echo "ERROR: Runtime '${RUNTIME}' not found on host." >&2
    exit 1
fi

# Ensure xpose binary is built statically
BIN_PATH="${ROOT_DIR}/bin/xpose"
if [ ! -f "${BIN_PATH}" ]; then
    echo "Building xpose statically (CGO_ENABLED=0)..."
    mkdir -p "${ROOT_DIR}/bin"
    (
        cd "${ROOT_DIR}"
        export PATH="${HOME}/.local/go-toolchain/go/bin:${PATH}"
        CGO_ENABLED=0 go build -o bin/xpose .
    )
fi

echo "Running tests A-F inside ${IMAGE} container..."

"${RUNTIME}" run --rm -i \
    -v "${BIN_PATH}:/usr/local/bin/xpose:ro" \
    "${IMAGE}" bash -s << 'CONTAINER_EOF'
set -euo pipefail

echo "--- [Setup] Installing prerequisites inside container ---"
# fedora:44 provides /usr/bin/script in util-linux-script
if ! dnf install -y util-linux-script >/dev/null 2>&1; then
    dnf install -y util-linux >/dev/null 2>&1
fi

if [ ! -x /usr/bin/script ]; then
    echo "ERROR: /usr/bin/script not available in container." >&2
    exit 1
fi

# Clean environment
mkdir -p /tmp/xpose-tests
cd /tmp/xpose-tests

# Ensure target packages (htop, tree) are NOT initially installed
dnf remove -y htop tree >/dev/null 2>&1 || true

# Helper for interactive confirmations
confirm_y() {
    printf "y\ny\ny\ny\ny\n"
}

confirm_n() {
    printf "y\nn\n"
}

# ==============================================================================
# TEST A: Interactive apply via script -qec (reinstalls htop + tree)
# ==============================================================================
echo "=== Running Test A: Interactive apply with script ==="
cat << 'EOF' > plan-a.yaml
source:
  distro: fedora
  version: "44"
target:
  distro: fedora
  family: rpm
steps:
  - order: 1
    manager: dnf
    action: install
    items:
      - source: htop
        target: htop
        status: exact
      - source: tree
        target: tree
        status: exact
EOF

confirm_y | script -qec "/usr/local/bin/xpose apply plan-a.yaml --apply" /dev/null

if rpm -q htop >/dev/null 2>&1 && rpm -q tree >/dev/null 2>&1; then
    echo "✅ Test A OK: Interactive apply successfully installed htop and tree"
else
    echo "❌ Test A FAILED: htop or tree missing after interactive apply" >&2
    exit 1
fi

# Reset installed packages for Test B
dnf remove -y htop tree >/dev/null 2>&1

# ==============================================================================
# TEST B: Non-interactive apply (--apply --yes)
# ==============================================================================
echo "=== Running Test B: Non-interactive apply (--apply --yes) ==="
/usr/local/bin/xpose apply plan-a.yaml --apply --yes >/dev/null

if rpm -q htop >/dev/null 2>&1 && rpm -q tree >/dev/null 2>&1; then
    echo "✅ Test B OK: Non-interactive apply successfully installed htop and tree"
else
    echo "❌ Test B FAILED: htop or tree missing after non-interactive apply" >&2
    exit 1
fi

# ==============================================================================
# TEST C: Resume execution (--resume --apply --yes)
# ==============================================================================
echo "=== Running Test C: Resume execution after partial run ==="
# Remove tree but keep htop
dnf remove -y tree >/dev/null 2>&1
JOURNAL_DIR="/tmp/xpose-journal-c"
mkdir -p "${JOURNAL_DIR}"

cat << 'EOF' > plan-c.yaml
source:
  distro: fedora
  version: "44"
target:
  distro: fedora
  family: rpm
steps:
  - order: 1
    manager: dnf
    action: install
    items:
      - source: htop
        target: htop
        status: exact
  - order: 2
    manager: dnf
    action: install
    items:
      - source: tree
        target: tree
        status: exact
EOF

# Simulate an interrupted run: Step 1 (htop) succeeded, Step 2 (tree) pending
RUN_ID="testc123"
RUN_FILE="${JOURNAL_DIR}/2026-11-02T10-00-00Z-${RUN_ID}.jsonl"
cat << EOF > "${RUN_FILE}"
{"timestamp":"2026-11-02T10:00:00Z","step_order":1,"manager":"dnf","action":"install","command":"dnf install -y htop","items":["htop"],"status":"success"}
{"timestamp":"2026-11-02T10:00:01Z","step_order":2,"manager":"dnf","action":"install","command":"dnf install -y tree","items":["tree"],"status":"pending"}
EOF

/usr/local/bin/xpose apply plan-c.yaml --apply --yes --resume --journal-dir "${JOURNAL_DIR}" >/dev/null

if rpm -q tree >/dev/null 2>&1 && rpm -q htop >/dev/null 2>&1; then
    echo "✅ Test C OK: Resume skipped successful htop and replayed pending tree"
else
    echo "❌ Test C FAILED: htop or tree missing after resume execution" >&2
    exit 1
fi

# ==============================================================================
# TEST D: Hermetic dry-run (rpm -qa unchanged)
# ==============================================================================
echo "=== Running Test D: Hermetic dry-run ==="
dnf remove -y tree >/dev/null 2>&1
BEFORE_RPM=$(rpm -qa | sort)

/usr/local/bin/xpose apply plan-a.yaml >/dev/null

AFTER_RPM=$(rpm -qa | sort)
if [ "${BEFORE_RPM}" = "${AFTER_RPM}" ]; then
    echo "✅ Test D OK: Dry-run left system completely unchanged"
else
    echo "❌ Test D FAILED: System packages changed during dry-run" >&2
    exit 1
fi

# ==============================================================================
# TEST E: Destructive safety guardrail
# ==============================================================================
echo "=== Running Test E: Destructive action guardrail ==="
cat << 'EOF' > plan-destructive.yaml
source:
  distro: fedora
  version: "44"
target:
  distro: fedora
  family: rpm
steps:
  - order: 1
    manager: dnf
    action: remove
    items:
      - source: htop
        target: htop
        status: exact
EOF

set +e
ERR_OUTPUT=$(/usr/local/bin/xpose apply plan-destructive.yaml --apply --yes 2>&1)
ERR_CODE=$?
set -e

if [ "${ERR_CODE}" -ne 0 ] && echo "${ERR_OUTPUT}" | grep -qiE "destructive|forbidden|prohibited"; then
    echo "✅ Test E OK: Destructive plan correctly blocked with safety error"
else
    echo "❌ Test E FAILED: Expected failure on destructive plan, got code ${ERR_CODE}: ${ERR_OUTPUT}" >&2
    exit 1
fi

# ==============================================================================
# TEST F: Third-party repo interactive confirmation declined
# ==============================================================================
echo "=== Running Test F: Third-party repo prompt declined ==="
REPO_FILE="/etc/yum.repos.d/fake-test.repo"
rm -f "${REPO_FILE}"

cat << 'EOF' > plan-repo.yaml
source:
  distro: fedora
  version: "44"
target:
  distro: fedora
  family: rpm
steps:
  - order: 1
    manager: repos
    action: restore-repos
    repo_items:
      - repo:
          kind: rpm
          name: fake-test
          file_name: fake-test.repo
          third_party: true
          content: |
            [fake-test]
            name=fake-test
            baseurl=https://example.com/repo
            enabled=1
EOF

# Global confirm 'y', but repo confirm 'n'
confirm_n | script -qec "/usr/local/bin/xpose apply plan-repo.yaml --apply" /dev/null || true

if [ ! -f "${REPO_FILE}" ]; then
    echo "✅ Test F OK: Third-party repo declined was not created"
else
    echo "❌ Test F FAILED: Third-party repo file was created despite rejection" >&2
    rm -f "${REPO_FILE}"
    exit 1
fi

echo ""
echo "🎉 TOUS LES TESTS D'INTÉGRATION PASSENT (A-F)"
CONTAINER_EOF

echo "All container integration tests completed successfully with ${RUNTIME}."
