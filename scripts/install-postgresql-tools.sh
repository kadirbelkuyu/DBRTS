#!/bin/bash

set -e

echo "PostgreSQL Client Tools Installer"
echo "====================================="

detect_os() {
    if [[ "$OSTYPE" == "linux-gnu"* ]]; then
        if command -v apt-get &> /dev/null; then
            echo "ubuntu"
        elif command -v yum &> /dev/null; then
            echo "centos"
        elif command -v dnf &> /dev/null; then
            echo "fedora"
        elif command -v pacman &> /dev/null; then
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

check_postgresql() {
    echo "Checking for existing PostgreSQL client tools..."

    if command -v pg_dump &> /dev/null && command -v pg_restore &> /dev/null && command -v psql &> /dev/null; then
        echo "PostgreSQL client tools are already installed:"
        pg_dump --version
        echo "Location: $(which pg_dump)"
        return 0
    fi

    echo "PostgreSQL client tools are not available."
    return 1
}

install_macos() {
    echo "Installing PostgreSQL client tools for macOS..."

    if ! command -v brew &> /dev/null; then
        echo "Homebrew is required. Install it first:"
        echo "curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh | bash"
        echo ""
        echo "Alternative: download Postgres.app from https://postgresapp.com and add it to PATH:"
        echo 'export PATH="/Applications/Postgres.app/Contents/Versions/latest/bin:$PATH"'
        exit 1
    fi

    echo "Installation options:"
    echo "1. Client tools only (libpq) - recommended"
    echo "2. Full PostgreSQL server"
    read -p "Choose (1-2) [1]: " choice

    case ${choice:-1} in
        1)
            brew install libpq
            echo ""
            echo "Adding libpq to PATH..."
            {
                echo 'export PATH="/opt/homebrew/opt/libpq/bin:$PATH"'
                echo 'export PATH="/usr/local/opt/libpq/bin:$PATH"'
            } >> ~/.zshrc
            echo "libpq installed. Open a new terminal or run 'source ~/.zshrc'."
            ;;
        2)
            brew install postgresql
            echo "PostgreSQL server installation completed."
            ;;
        *)
            echo "Invalid option."
            exit 1
            ;;
    esac
}

install_ubuntu() {
    echo "Installing PostgreSQL client tools for Ubuntu/Debian..."

    echo "Installation options:"
    echo "1. Client tools only - recommended"
    echo "2. Full PostgreSQL server"
    read -p "Choose (1-2) [1]: " choice

    sudo apt update

    case ${choice:-1} in
        1)
            sudo apt install -y postgresql-client
            echo "PostgreSQL client tools installed."
            ;;
        2)
            sudo apt install -y postgresql postgresql-client
            echo "PostgreSQL server installation completed."
            ;;
        *)
            echo "Invalid option."
            exit 1
            ;;
    esac
}

install_centos() {
    echo "Installing PostgreSQL client tools for CentOS/RHEL..."

    echo "Installation options:"
    echo "1. Client tools only"
    echo "2. Full PostgreSQL server"
    read -p "Choose (1-2) [1]: " choice

    case ${choice:-1} in
        1)
            sudo yum install -y postgresql
            echo "PostgreSQL client tools installed."
            ;;
        2)
            sudo yum install -y postgresql postgresql-server postgresql-contrib
            echo "PostgreSQL server installation completed."
            ;;
        *)
            echo "Invalid option."
            exit 1
            ;;
    esac
}

install_fedora() {
    echo "Installing PostgreSQL client tools for Fedora..."

    echo "Installation options:"
    echo "1. Client tools only"
    echo "2. Full PostgreSQL server"
    read -p "Choose (1-2) [1]: " choice

    case ${choice:-1} in
        1)
            sudo dnf install -y postgresql
            echo "PostgreSQL client tools installed."
            ;;
        2)
            sudo dnf install -y postgresql postgresql-server postgresql-contrib
            echo "PostgreSQL server installation completed."
            ;;
        *)
            echo "Invalid option."
            exit 1
            ;;
    esac
}

install_arch() {
    echo "Installing PostgreSQL client tools for Arch Linux..."
    sudo pacman -Sy --noconfirm postgresql
    echo "PostgreSQL package installed."
}

verify_installation() {
    echo "Verifying installation..."
    check_postgresql || {
        echo "Installation failed. Please review the output above."
        exit 1
    }
    echo "PostgreSQL client tools are ready."
}

main() {
    if check_postgresql; then
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
            echo "Unsupported OS."
            echo "Please install PostgreSQL client tools manually."
            exit 1
            ;;
    esac

    verify_installation
    echo "You can now run the dbrts CLI."
}

main "$@"
