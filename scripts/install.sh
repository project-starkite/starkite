#!/bin/sh
# starkite installer script
# Safe, POSIX-compliant, and dry-run capable.
# Usage:
#   curl -fsSL https://starkite.run/install.sh | sh
#   With options:
#   curl -fsSL https://starkite.run/install.sh | PREFIX=/usr/bin sh
#   Dry-run mode:
#   curl -fsSL https://starkite.run/install.sh | INSTALL_DRY_RUN=1 sh

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
    LATEST_TAG="${STARKITE_VERSION:-${VERSION:-}}"
    BASE_URL="${STARKITE_DOWNLOAD_BASE_URL:-}"
    if [ -z "$LATEST_TAG" ]; then
        echo "Checking latest release version..."
        LATEST_TAG=$(curl -fsSL "https://api.github.com/repos/$OWNER/$REPO/releases/latest" 2>/dev/null | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/' || true)
    fi

    if [ -z "$LATEST_TAG" ]; then
        if [ "$DRY_RUN" = "1" ] || [ "$DRY_RUN" = "true" ]; then
            LATEST_TAG="dry-run"
        else
            echo "Error: Could not retrieve latest release tag from GitHub API. Set STARKITE_VERSION to install a specific release." >&2
            exit 1
        fi
    fi

    if [ -n "$BASE_URL" ]; then
        BASE_URL="${BASE_URL%/}"
        DOWNLOAD_URL="$BASE_URL/$BINARY_NAME"
        CHECKSUM_URL="$BASE_URL/checksums.txt"
    else
        DOWNLOAD_URL="https://github.com/$OWNER/$REPO/releases/download/$LATEST_TAG/$BINARY_NAME"
        CHECKSUM_URL="https://github.com/$OWNER/$REPO/releases/download/$LATEST_TAG/checksums.txt"
    fi

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

    if ! command -v awk >/dev/null 2>&1; then
        echo "Error: Required command 'awk' is not installed." >&2
        exit 1
    fi
    if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
        echo "Error: Required checksum tool ('sha256sum' or 'shasum') is not installed." >&2
        exit 1
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
        echo "Error: Failed to download checksums from $CHECKSUM_URL" >&2
        exit 1
    fi

    echo "Verifying checksum..."
    EXPECTED_HASH=$(awk -v bin="$BINARY_NAME" '$2 == bin || $2 == ("*" bin) {print $1; exit}' "$TEMP_DIR/checksums.txt")
    if [ -z "$EXPECTED_HASH" ]; then
        echo "Error: Checksum for $BINARY_NAME not found in checksums.txt." >&2
        exit 1
    fi

    if command -v sha256sum >/dev/null 2>&1; then
        ACTUAL_HASH=$(sha256sum "$TEMP_DIR/kite" | awk '{print $1}')
    else
        ACTUAL_HASH=$(shasum -a 256 "$TEMP_DIR/kite" | awk '{print $1}')
    fi

    if [ -z "$ACTUAL_HASH" ] || [ "$ACTUAL_HASH" != "$EXPECTED_HASH" ]; then
        echo "Error: Checksum validation failed!" >&2
        echo "Expected: $EXPECTED_HASH" >&2
        echo "Actual:   $ACTUAL_HASH" >&2
        exit 1
    fi
    echo "Checksum verified successfully."

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
