#!/bin/bash
#
# notarize.sh - Sign and notarize macOS binaries
#
# Signs a macOS binary with Apple Developer ID, creates an installer package,
# and submits it for Apple notarization.
#
# Prerequisites:
#   - Apple Developer ID Application certificate in keychain
#   - Apple Developer ID Installer certificate in keychain
#   - Keychain profile "notarytool" configured with App Store Connect credentials
#     (xcrun notarytool store-credentials "notarytool" --apple-id "..." --team-id "...")
#
# Usage:
#   ./scripts/notarize.sh <executable-path> <executable-name>
#
# Examples:
#   ./scripts/notarize.sh ./build/release/muti-metroo-darwin-arm64 muti-metroo-darwin-arm64
#   ./scripts/notarize.sh ./build/release/muti-metroo-darwin-amd64 muti-metroo-darwin-amd64

set -e  # Exit on error

# Developer credentials
DEVELOPER_NAME="Andris Reinman"
APPLE_ID="andris.reinman@kreata.ee"
TEAM_ID="8JM6VJ352Q"

# Get script directory for relative paths
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Validate arguments
if [ $# -ne 2 ]; then
    echo "Usage: $0 <executable-path> <executable-name>"
    echo "Example: $0 ./build/release/muti-metroo-darwin-arm64 muti-metroo-darwin-arm64"
    exit 1
fi

EXECUTABLE_PATH="$1"
EXECUTABLE_NAME="$2"

# Check if executable exists
if [ ! -f "$EXECUTABLE_PATH" ]; then
    echo "Error: Executable not found at $EXECUTABLE_PATH"
    exit 1
fi

# Get version from git tag or default
VERSION=$(git describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "0.0.0")
echo "Version: $VERSION"

# Check for entitlements file
ENTITLEMENTS_FILE="${PROJECT_ROOT}/entitlements.mac.plist"
if [ ! -f "$ENTITLEMENTS_FILE" ]; then
    echo "Warning: Entitlements file not found at $ENTITLEMENTS_FILE"
    echo "Creating a basic entitlements file..."
    cat > "$ENTITLEMENTS_FILE" << 'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>com.apple.security.cs.allow-unsigned-executable-memory</key>
    <true/>
    <key>com.apple.security.cs.allow-jit</key>
    <true/>
    <key>com.apple.security.cs.disable-library-validation</key>
    <true/>
    <key>com.apple.security.network.client</key>
    <true/>
    <key>com.apple.security.network.server</key>
    <true/>
</dict>
</plist>
EOF
fi

# Mac notarization function
function notarize_pkg {
    local EXEC_PATH="$1"
    local EXEC_NAME="$2"

    echo "Signing and notarizing ${EXEC_NAME}..."

    # Determine the builds directory
    local BUILDS_DIR="$(dirname "$EXEC_PATH")"

    # Create a temporary working directory
    local TEMP_DIR=$(mktemp -d)
    cd "$TEMP_DIR"

    # Clean up any previous attempts
    rm -rf pkgroot
    mkdir -p pkgroot

    # Copy executable to temp directory for signing
    cp "$EXEC_PATH" "./${EXEC_NAME}"

    # Sign the executable
    echo "Signing executable..."
    if ! codesign -vvvv --force \
        --sign "Developer ID Application: ${DEVELOPER_NAME} (${TEAM_ID})" \
        --options runtime \
        --timestamp \
        --entitlements "${ENTITLEMENTS_FILE}" \
        "./${EXEC_NAME}"; then
        echo "Error: Code signing failed"
        rm -rf "$TEMP_DIR"
        exit 1
    fi

    # Copy the signed executable back to builds folder (replace unsigned version)
    echo "Replacing unsigned executable with signed version..."
    cp -f "./${EXEC_NAME}" "$EXEC_PATH"

    # Copy to package root with simplified name for .pkg creation
    # The installer will install as /usr/local/bin/muti-metroo instead of the platform-specific name
    echo "Preparing executable for package (will be installed as 'muti-metroo')..."
    cp "./${EXEC_NAME}" pkgroot/muti-metroo

    # Build the package (using generic identifier since binary is renamed)
    echo "Building package..."
    if ! pkgbuild --root pkgroot \
        --identifier "com.postalsys.muti-metroo" \
        --version "${VERSION}" \
        --install-location "/usr/local/bin" \
        --sign "Developer ID Installer: ${DEVELOPER_NAME} (${TEAM_ID})" \
        "./${EXEC_NAME}.pkg"; then
        echo "Error: Package building failed"
        rm -rf "$TEMP_DIR"
        exit 1
    fi

    rm -rf pkgroot

    # Submit for notarization
    echo "Submitting for notarization..."
    local NOTARIZE_OUTPUT=$(mktemp)

    if ! xcrun notarytool submit \
        --apple-id "${APPLE_ID}" \
        --keychain-profile "notarytool" \
        --team-id "${TEAM_ID}" \
        --wait \
        "./${EXEC_NAME}.pkg" 2>&1 | tee "$NOTARIZE_OUTPUT"; then
        echo "Error: Notarization submission failed"
        cat "$NOTARIZE_OUTPUT"
        rm -f "$NOTARIZE_OUTPUT"
        rm -rf "$TEMP_DIR"
        exit 1
    fi

    # Extract the notarization ID and status
    local NOTARIZE_ID=$(grep "^\s*id:" "$NOTARIZE_OUTPUT" | head -1 | sed 's/.*id: *//')
    local STATUS=$(grep "^\s*status:" "$NOTARIZE_OUTPUT" | tail -1 | sed 's/.*status: *//')

    rm -f "$NOTARIZE_OUTPUT"

    if [[ "$STATUS" != "Accepted" ]]; then
        echo "Error: Notarization failed with status: $STATUS"
        echo "Run the following command to see the log:"
        echo "xcrun notarytool log $NOTARIZE_ID --keychain-profile notarytool"
        rm -rf "$TEMP_DIR"
        exit 1
    fi

    echo "${EXEC_NAME} notarized successfully. ID: ${NOTARIZE_ID}"

    # Staple the notarization ticket
    echo "Stapling notarization ticket..."
    if ! xcrun stapler staple "./${EXEC_NAME}.pkg"; then
        echo "Error: Stapling failed"
        rm -rf "$TEMP_DIR"
        exit 1
    fi

    # Move the notarized .pkg to builds directory
    echo "Moving package to builds directory..."
    mv "./${EXEC_NAME}.pkg" "${BUILDS_DIR}/${EXEC_NAME}.pkg"

    # Clean up temp directory
    cd "$PROJECT_ROOT"
    rm -rf "$TEMP_DIR"

    echo "[+] Notarization complete for ${EXEC_NAME}"
    echo "  Signed executable: ${BUILDS_DIR}/${EXEC_NAME}"
    echo "  Package installer: ${BUILDS_DIR}/${EXEC_NAME}.pkg"
    echo "  Installs as: /usr/local/bin/muti-metroo"
}

# Run the notarization
notarize_pkg "$EXECUTABLE_PATH" "$EXECUTABLE_NAME"
