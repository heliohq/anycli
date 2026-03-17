#!/bin/sh
# AnyCLI installer - https://anycli.dev
# Usage: curl -fsSL https://anycli.dev/install | sh

set -e

REPO="sheet0/anycli"
ANYCLI_HOME="${ANYCLI_HOME:-$HOME/.anycli}"
INSTALL_DIR="${ANYCLI_INSTALL_DIR:-$ANYCLI_HOME/bin}"

# Detect OS and architecture
detect_platform() {
  OS=$(uname -s | tr '[:upper:]' '[:lower:]')
  ARCH=$(uname -m)

  case "$OS" in
    darwin) OS="darwin" ;;
    linux)  OS="linux" ;;
    *)      echo "error: unsupported OS: $OS"; exit 1 ;;
  esac

  case "$ARCH" in
    x86_64|amd64)  ARCH="amd64" ;;
    arm64|aarch64) ARCH="arm64" ;;
    *)             echo "error: unsupported architecture: $ARCH"; exit 1 ;;
  esac

  echo "${OS}_${ARCH}"
}

# Resolve the download URL for the latest release asset matching the platform
resolve_asset_url() {
  local platform="$1"
  local api_url="https://api.github.com/repos/${REPO}/releases/tags/latest"
  local json

  if command -v curl >/dev/null 2>&1; then
    json=$(curl -fsSL "$api_url")
  elif command -v wget >/dev/null 2>&1; then
    json=$(wget -qO- "$api_url")
  else
    echo "error: curl or wget required" >&2
    exit 1
  fi

  # Find the asset URL matching our platform
  echo "$json" | grep "browser_download_url" | grep "$platform" | head -1 | sed 's/.*"\(https[^"]*\)".*/\1/'
}

# Download and install
install() {
  PLATFORM=$(detect_platform)

  echo "Installing anycli (${PLATFORM})..."

  # Download from the rolling "latest" release tag
  # Asset naming: anycli_<version>_<os>_<arch>.tar.gz — but version varies,
  # so we resolve the actual asset URL from the release API.
  ASSET_URL=$(resolve_asset_url "$PLATFORM")

  if [ -z "$ASSET_URL" ]; then
    echo "error: could not find release asset for ${PLATFORM}"
    exit 1
  fi
  TMP_DIR=$(mktemp -d)
  trap 'rm -rf "$TMP_DIR"' EXIT

  # Download
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL -L "$ASSET_URL" -o "$TMP_DIR/anycli.tar.gz"
  else
    wget -q "$ASSET_URL" -O "$TMP_DIR/anycli.tar.gz"
  fi

  # Extract
  tar -xzf "$TMP_DIR/anycli.tar.gz" -C "$TMP_DIR"

  # Install binary
  if [ -w "$INSTALL_DIR" ]; then
    mv "$TMP_DIR/anycli" "$INSTALL_DIR/anycli"
  else
    echo "Installing to ${INSTALL_DIR} (requires sudo)..."
    sudo mv "$TMP_DIR/anycli" "$INSTALL_DIR/anycli"
  fi

  chmod +x "$INSTALL_DIR/anycli"

  # Create anycli directories
  mkdir -p "$ANYCLI_HOME/bin"
  mkdir -p "$ANYCLI_HOME/registry"
  mkdir -p "$ANYCLI_HOME/credentials"
  mkdir -p "$ANYCLI_HOME/tools"

  # Add ~/.anycli/bin to PATH if not already present
  SHELL_NAME=$(basename "$SHELL")
  BIN_PATH="$ANYCLI_HOME/bin"
  PATH_LINE="export PATH=\"$BIN_PATH:\$PATH\""

  add_to_path() {
    local rc_file="$1"
    if [ -f "$rc_file" ]; then
      if ! grep -q "anycli/bin" "$rc_file" 2>/dev/null; then
        echo "" >> "$rc_file"
        echo "# AnyCLI" >> "$rc_file"
        echo "$PATH_LINE" >> "$rc_file"
      fi
    fi
  }

  case "$SHELL_NAME" in
    zsh)  add_to_path "$HOME/.zshrc" ;;
    bash) add_to_path "$HOME/.bashrc"; add_to_path "$HOME/.bash_profile" ;;
    fish)
      mkdir -p "$HOME/.config/fish"
      if ! grep -q "anycli/bin" "$HOME/.config/fish/config.fish" 2>/dev/null; then
        echo "" >> "$HOME/.config/fish/config.fish"
        echo "# AnyCLI" >> "$HOME/.config/fish/config.fish"
        echo "set -gx PATH $BIN_PATH \$PATH" >> "$HOME/.config/fish/config.fish"
      fi
      ;;
    *)    add_to_path "$HOME/.profile" ;;
  esac

  echo ""
  echo "  anycli installed successfully!"
  echo ""
  echo "  Run 'anycli install gh' to get started."
  echo ""
  echo "  Restart your shell or run:"
  echo "    $PATH_LINE"
  echo ""
}

# Uninstall
uninstall() {
  echo "Uninstalling anycli..."

  # Remove binary
  if [ -f "$INSTALL_DIR/anycli" ]; then
    if [ -w "$INSTALL_DIR" ]; then
      rm -f "$INSTALL_DIR/anycli"
    else
      sudo rm -f "$INSTALL_DIR/anycli"
    fi
  fi

  # Remove anycli home directory
  if [ -d "$ANYCLI_HOME" ]; then
    rm -rf "$ANYCLI_HOME"
  fi

  # Remove PATH entries from shell configs
  remove_from_rc() {
    local rc_file="$1"
    if [ -f "$rc_file" ]; then
      # Remove AnyCLI comment and PATH line
      sed -i.bak '/# AnyCLI/d' "$rc_file"
      sed -i.bak '/anycli\/bin/d' "$rc_file"
      rm -f "${rc_file}.bak"
    fi
  }

  remove_from_rc "$HOME/.zshrc"
  remove_from_rc "$HOME/.bashrc"
  remove_from_rc "$HOME/.bash_profile"
  remove_from_rc "$HOME/.profile"
  if [ -f "$HOME/.config/fish/config.fish" ]; then
    remove_from_rc "$HOME/.config/fish/config.fish"
  fi

  echo ""
  echo "  anycli uninstalled successfully."
  echo "  Restart your shell to apply PATH changes."
  echo ""
}

# Parse command
case "${1:-install}" in
  install)   install ;;
  uninstall) uninstall ;;
  *)
    echo "Usage: install.sh [install|uninstall]"
    exit 1
    ;;
esac
