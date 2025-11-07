#!/bin/bash

set -euo pipefail

echo "MongoDB Database Tools Installer"
echo "=================================="

detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if command -v apt-get >/dev/null 2>&1; then
            echo "ubuntu"
        elif command -v yum >/dev/null 2>&1; then
            echo "centos"
        elif command -v dnf >/dev/null 2>&1; then
            echo "fedora"
        elif command -v pacman >/dev/null 2>&1; then
            echo "arch"
        else
            echo "linux"
        fi
    elif [[ "$OSTYPE" == "darwin"* ]]; then
        echo "macos"
    else
        echo "unknown"
    fi
}

check_tools() {
    if command -v mongodump >/dev/null 2>&1 && command -v mongorestore >/dev/null 2>&1; then
        echo "MongoDB tools already installed:"
        mongodump --version
        return 0
    fi
    return 1
}

install_macos() {
    if ! command -v brew >/dev/null 2>&1; then
        echo "Homebrew is required. Install it first:"
        echo "  /bin/bash -c \"\$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)\""
        exit 1
    fi

    echo "Installing mongodb-database-tools via Homebrew..."
    brew tap mongodb/brew
    brew install mongodb-database-tools
}

install_ubuntu() {
    echo "Installing MongoDB tools for Ubuntu/Debian..."
    wget -qO - https://www.mongodb.org/static/pgp/server-6.0.asc | sudo apt-key add -
    echo "deb [ arch=amd64,arm64 ] https://repo.mongodb.org/apt/ubuntu $(lsb_release -cs)/mongodb-org/6.0 multiverse" | sudo tee /etc/apt/sources.list.d/mongodb-org-6.0.list
    sudo apt-get update
    sudo apt-get install -y mongodb-database-tools
}

install_centos() {
    echo "Installing MongoDB tools for CentOS/RHEL..."
    cat <<'EOF' | sudo tee /etc/yum.repos.d/mongodb-org-6.0.repo >/dev/null
[mongodb-org-6.0]
name=MongoDB Repository
baseurl=https://repo.mongodb.org/yum/redhat/$releasever/mongodb-org/6.0/x86_64/
gpgcheck=1
enabled=1
gpgkey=https://www.mongodb.org/static/pgp/server-6.0.asc
EOF
    sudo yum install -y mongodb-database-tools
}

install_fedora() {
    echo "Installing MongoDB tools for Fedora..."
    cat <<'EOF' | sudo tee /etc/yum.repos.d/mongodb-org-6.0.repo >/dev/null
[mongodb-org-6.0]
name=MongoDB Repository
baseurl=https://repo.mongodb.org/yum/redhat/$releasever/mongodb-org/6.0/x86_64/
gpgcheck=1
enabled=1
gpgkey=https://www.mongodb.org/static/pgp/server-6.0.asc
EOF
    sudo dnf install -y mongodb-database-tools
}

install_arch() {
    echo "Installing MongoDB tools for Arch Linux..."
    sudo pacman -Sy --noconfirm mongodb-tools
}

main() {
    if check_tools; then
        echo "Nothing to do."
        exit 0
    fi

    os=$(detect_os)
    echo "Detected OS: $os"

    case "$os" in
        macos) install_macos ;;
        ubuntu) install_ubuntu ;;
        centos) install_centos ;;
        fedora) install_fedora ;;
        arch) install_arch ;;
        *)
            echo "Unsupported OS. Install mongodb-database-tools manually from https://www.mongodb.com/try/download/database-tools"
            exit 1
            ;;
    esac

    echo
    check_tools && echo "MongoDB tools installed successfully."
}

main
