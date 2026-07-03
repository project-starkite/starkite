#!/bin/sh
# starkite installer script
# Safe, POSIX-compliant, and dry-run capable.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/project-starkite/starkite/main/scripts/install.sh | sh
#   With options:
#   curl -fsSL https://raw.githubusercontent.com/project-starkite/starkite/main/scripts/install.sh | PREFIX=/usr/bin sh
#   Dry-run mode:
#   curl -fsSL https://raw.githubusercontent.com/project-starkite/starkite/main/scripts/install.sh | INSTALL_DRY_RUN=1 sh

set -eu

install_starkite() {
    # Configuration
    OWNER="project-starkite"
    REPO="starkite"
    INSTALL_DIR="${PREFIX:-/usr/local/bin}"
    DRY_RUN="${INSTALL_DRY_RUN:-0}"

    echo "--- Starkite Installer ---"

    # Verify dependencies
    for cmd in curl uname grep sed; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            echo "Error: Required command '$cmd' is not installed." >&2
            exit 1
        fi
    done

    # Detect OS and architecture
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$ARCH" in
        x86_64|amd64)
            TARGET_ARCH="amd64"
            ;;
        arm64|aarch64)
            TARGET_ARCH="arm64"
            ;;
        *)
            echo "Error: Unsupported architecture: $ARCH" >&2
            exit 1
            ;;
    esac

    case "$OS" in
        linux)
            TARGET_OS="linux"
            ;;
        darwin)
            TARGET_OS="darwin"
            ;;
        *)
            echo "Error: Unsupported operating system: $OS" >&2
            exit 1
            ;;
    esac

    BINARY_NAME="kite-$TARGET_OS-$TARGET_ARCH"

    # Fetch latest version tag from GitHub API
    echo "Checking latest release version..."
    LATEST_TAG=$(curl -s "https://api.github.com/repos/$OWNER/$REPO/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$LATEST_TAG" ]; then
        # Fallback default tag if API rate limited or unreachable
        LATEST_TAG="v0.1.0"
        echo "Warning: Could not retrieve latest release tag from API. Using default: $LATEST_TAG" >&2
    fi

    DOWNLOAD_URL="https://github.com/$OWNER/$REPO/releases/download/$LATEST_TAG/$BINARY_NAME"
    CHECKSUM_URL="https://github.com/$OWNER/$REPO/releases/download/$LATEST_TAG/checksums.txt"

    # Dry-Run Mode
    if [ "$DRY_RUN" = "1" ] || [ "$DRY_RUN" = "true" ]; then
        echo "[DRY-RUN] OS:                  $TARGET_OS"
        echo "[DRY-RUN] Architecture:        $TARGET_ARCH"
        echo "[DRY-RUN] Binary Name:         $BINARY_NAME"
        echo "[DRY-RUN] Release Tag:         $LATEST_TAG"
        echo "[DRY-RUN] Download URL:        $DOWNLOAD_URL"
        echo "[DRY-RUN] Checksum URL:        $CHECKSUM_URL"
        echo "[DRY-RUN] Target Directory:    $INSTALL_DIR"
        echo "[DRY-RUN] Dry run completed. No files were downloaded or modified."
        return 0
    fi

    # Create temporary directory
    TEMP_DIR=$(mktemp -d)
    # Ensure cleanup on exit
    trap 'rm -rf "$TEMP_DIR"' EXIT

    echo "Downloading starkite ($LATEST_TAG) for $TARGET_OS/$TARGET_ARCH..."
    if ! curl -sLf -o "$TEMP_DIR/kite" "$DOWNLOAD_URL"; then
        echo "Error: Failed to download binary from $DOWNLOAD_URL" >&2
        exit 1
    fi

    if ! curl -sLf -o "$TEMP_DIR/checksums.txt" "$CHECKSUM_URL"; then
        echo "Warning: Checksum file not available. Proceeding without checksum validation." >&2
    else
        # Verify SHA256 checksum
        echo "Verifying checksum..."
        EXPECTED_HASH=$(grep "$BINARY_NAME" "$TEMP_DIR/checksums.txt" | awk '{print $1}')
        if [ -z "$EXPECTED_HASH" ]; then
            echo "Warning: Checksum for $BINARY_NAME not found in release signatures." >&2
        else
            if command -v sha256sum >/dev/null 2>&1; then
                ACTUAL_HASH=$(sha256sum "$TEMP_DIR/kite" | awk '{print $1}')
            elif command -v shasum >/dev/null 2>&1; then
                ACTUAL_HASH=$(shasum -a 256 "$TEMP_DIR/kite" | awk '{print $1}')
            else
                ACTUAL_HASH=""
                echo "Warning: Neither sha256sum nor shasum was found. Skipping validation." >&2
            fi

            if [ -n "$ACTUAL_HASH" ] && [ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]; then
                echo "Error: Checksum validation failed!" >&2
                echo "Expected: $EXPECTED_HASH" >&2
                echo "Actual:   $ACTUAL_HASH" >&2
                exit 1
            fi
            echo "Checksum verified successfully."
        fi
    fi

    # Install binary
    chmod +x "$TEMP_DIR/kite"
    echo "Installing to $INSTALL_DIR..."

    if [ -w "$INSTALL_DIR" ]; then
        mv "$TEMP_DIR/kite" "$INSTALL_DIR/kite"
    else
        echo "Write access denied to $INSTALL_DIR. Elevating privileges with sudo..."
        if ! command -v sudo >/dev/null 2>&1; then
            echo "Error: Write access denied and 'sudo' is not installed." >&2
            exit 1
        fi
        sudo mv "$TEMP_DIR/kite" "$INSTALL_DIR/kite"
    fi

    echo "Successfully installed starkite to $INSTALL_DIR/kite"
}

install_starkite "$@"
