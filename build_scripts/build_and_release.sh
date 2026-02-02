#!/bin/bash

# QMediaSync Build and Release Shell Script
# 切换到工作目录
cd ../
echo "已切换工作目录：$(pwd)"
# Function to print colored output
print_colored() {
    local color=$1
    local message=$2
    case $color in
        "green") echo -e "\033[32m$message\033[0m" ;;
        "red") echo -e "\033[31m$message\033[0m" ;;
        "yellow") echo -e "\033[33m$message\033[0m" ;;
        "cyan") echo -e "\033[36m$message\033[0m" ;;
        *) echo "$message" ;;
    esac
}

# Function to check if command exists
command_exists() {
    command -v "$1" >/dev/null 2>&1
}

# Function to show usage
show_usage() {
    echo "Usage: $0 [-v VERSION]"
    echo "Options:"
    echo "  -v VERSION    Specify version (e.g., v1.0.0)"
    echo "  -h           Show this help message"
}

# Parse command line arguments
VERSION=""
while getopts "v:h" opt; do
    case $opt in
        v) VERSION="$OPTARG" ;;
        h) show_usage; exit 0 ;;
        *) show_usage; exit 1 ;;
    esac
done

print_colored "green" "========================================"
print_colored "green" "QMediaSync Build and Release Script"
print_colored "green" "========================================"

# Check if in Git repository
if [ ! -d ".git" ]; then
    print_colored "red" "Error: Not a Git repository"
    exit 1
fi

# Determine tag
if [ -n "$VERSION" ]; then
    # Use provided version parameter
    TAG="$VERSION"
    git tag "$TAG"
    git push origin "$TAG"
    print_colored "cyan" "Using provided version: $TAG"
else
    # Auto-detect existing tag
    TAG=$(git describe --tags --exact-match 2>/dev/null)
    if [ -z "$TAG" ]; then
        print_colored "red" "Error: No Git tag associated with current HEAD"
        print_colored "yellow" "Please create and push a tag: git tag vX.X.X && git push origin vX.X.X"
        print_colored "yellow" "Or use: $0 -v vX.X.X"
        exit 1
    fi
    print_colored "cyan" "Detected tag: $TAG"
fi

# Check if release notes file exists
RELEASE_NOTES_PATH=".changes/$TAG.md"
if [ ! -f "$RELEASE_NOTES_PATH" ]; then
    print_colored "yellow" "Warning: Release notes file $RELEASE_NOTES_PATH not found"
    print_colored "yellow" "Using default release notes"
    RELEASE_BODY="Release $TAG"
else
    print_colored "green" "Found release notes file"
    # Read file with proper encoding handling
    if command_exists "iconv"; then
        RELEASE_BODY=$(iconv -f UTF-8 -t UTF-8 "$RELEASE_NOTES_PATH" 2>/dev/null || cat "$RELEASE_NOTES_PATH")
    else
        RELEASE_BODY=$(cat "$RELEASE_NOTES_PATH")
    fi
fi

echo
print_colored "green" "Starting build..."
print_colored "green" "========================================"

# Create temp directory
if [ -d "temp_build" ]; then
    rm -rf "temp_build"
fi
mkdir -p "temp_build"

echo "安装所有项目依赖"
go mod tidy

# Supported platforms and architectures
PLATFORMS=("windows" "linux")
ARCHS=("amd64" "arm64")

# Build loop
for platform in "${PLATFORMS[@]}"; do
    for arch in "${ARCHS[@]}"; do
        echo
        print_colored "cyan" "Building $platform/$arch version..."
        
        # Set environment variables
        export GOOS="$platform"
        export GOARCH="$arch"
        export CGO_ENABLED="0"
        
        # Get current date in format: yyyy-mm-dd HH:MM:ss
        PUBLISH_DATE=$(date "+%Y-%m-%d %H:%M:%S")
        
        # Read API keys from environment variables
        FANART_API_KEY="${FANART_API_KEY:-}"
        DEFAULT_TMDB_ACCESS_TOKEN="${DEFAULT_TMDB_ACCESS_TOKEN:-}"
        DEFAULT_TMDB_API_KEY="${DEFAULT_TMDB_API_KEY:-}"
        DEFAULT_SC_API_KEY="${DEFAULT_SC_API_KEY:-}"
        
        # Check if any API key is empty
        if [ -z "$FANART_API_KEY" ] || [ -z "$DEFAULT_TMDB_ACCESS_TOKEN" ] || [ -z "$DEFAULT_TMDB_API_KEY" ] || [ -z "$DEFAULT_SC_API_KEY" ]; then
            print_colored "red" "Error: One or more API keys are not set"
            print_colored "yellow" "Please set all required environment variables:"
            print_colored "yellow" "  - FANART_API_KEY"
            print_colored "yellow" "  - DEFAULT_TMDB_ACCESS_TOKEN"
            print_colored "yellow" "  - DEFAULT_TMDB_API_KEY"
            print_colored "yellow" "  - DEFAULT_SC_API_KEY"
            exit 1
        fi
        
        # Determine executable name and link flags
        if [ "$platform" = "windows" ]; then
            EXE_NAME="QMediaSync.exe"
            LDFLAGS="-s -w -X main.Version=$TAG -X 'main.PublishDate=$PUBLISH_DATE' -X main.FANART_API_KEY=$FANART_API_KEY -X main.DEFAULT_TMDB_ACCESS_TOKEN=$DEFAULT_TMDB_ACCESS_TOKEN -X main.DEFAULT_TMDB_API_KEY=$DEFAULT_TMDB_API_KEY -X main.DEFAULT_SC_API_KEY=$DEFAULT_SC_API_KEY"
        else
            EXE_NAME="QMediaSync"
            LDFLAGS="-s -w -X main.Version=$TAG -X 'main.PublishDate=$PUBLISH_DATE' -X main.FANART_API_KEY=$FANART_API_KEY -X main.DEFAULT_TMDB_ACCESS_TOKEN=$DEFAULT_TMDB_ACCESS_TOKEN -X main.DEFAULT_TMDB_API_KEY=$DEFAULT_TMDB_API_KEY -X main.DEFAULT_SC_API_KEY=$DEFAULT_SC_API_KEY"
        fi
        
        # Build
        go build -ldflags "$LDFLAGS" -o "temp_build/$EXE_NAME"
        if [ $? -ne 0 ]; then
            print_colored "red" "Error: Build failed for $platform/$arch"
            exit 1
        fi
        
        # For Linux platform, add execute permission
        if [ "$platform" = "linux" ]; then
            print_colored "yellow" "Adding execute permission for Linux executable..."
            chmod +x "temp_build/$EXE_NAME" 2>/dev/null || print_colored "yellow" "Warning: Could not set execute permission (may be running on Windows)"
        fi
        
        # Create archive name
        if [ "$arch" = "amd64" ]; then
            ARCHIVE_NAME="QMediaSync_${platform}_x86_64"
        else
            ARCHIVE_NAME="QMediaSync_${platform}_${arch}"
        fi
        
        # Create release directory
        mkdir -p "temp_build/$ARCHIVE_NAME"
        
        # Copy files
        cp "temp_build/$EXE_NAME" "temp_build/$ARCHIVE_NAME/"
        
        if [ -d "web_statics" ]; then
            cp -r "web_statics" "temp_build/$ARCHIVE_NAME/"
        fi
        
        if [ -d "scripts" ]; then
            cp -r "scripts" "temp_build/$ARCHIVE_NAME/"
        fi
        
        # Windows specific files
        if [ "$platform" = "windows" ] && [ -f "icon.ico" ]; then
            cp "icon.ico" "temp_build/$ARCHIVE_NAME/"
        fi
        
        # PostgreSQL binaries
        POSTGRES_PATH="postgres/$platform/$arch"
        if [ -d "$POSTGRES_PATH" ]; then
            mkdir -p "temp_build/$ARCHIVE_NAME/postgres/$platform/$arch"
            cp -r "$POSTGRES_PATH/"* "temp_build/$ARCHIVE_NAME/postgres/$platform/$arch/" 2>/dev/null || true
        fi
        
        # Create archive
        if [ "$platform" = "windows" ]; then
            print_colored "yellow" "Creating ${ARCHIVE_NAME}.zip"
            if command_exists "zip"; then
                (cd "temp_build/$ARCHIVE_NAME" && zip -r "../../${ARCHIVE_NAME}.zip" .)
            else
                print_colored "red" "Error: zip command not found"
                exit 1
            fi
        else
            print_colored "yellow" "Creating ${ARCHIVE_NAME}.tar.gz"
            tar -czf "${ARCHIVE_NAME}.tar.gz" -C "temp_build" "$ARCHIVE_NAME"
        fi
        
        # Keep temp files for Docker build (do not delete)
        # rm -rf "temp_build/$ARCHIVE_NAME"
        # rm -f "temp_build/$EXE_NAME" 2>/dev/null || true
        # 如果是linux，将可执行文件按照平台架构重命名，方便后续docker打包
        if [ "$platform" = "linux" ]; then
            mv "temp_build/$EXE_NAME" "temp_build/QMediaSync_${platform}_${arch}_exe"
        else
           # 删除windows下的可执行文件
           rm -f "temp_build/$EXE_NAME" 2>/dev/null || true
        fi
        print_colored "green" "✓ Completed $platform/$arch version"
    done
done

echo 
print_colored "green" "========================================"
print_colored "green" "Build completed!"
print_colored "green" "========================================"

# Docker镜像打包
print_colored "cyan" "Starting Docker image build..."

# Check if Docker is available
if ! command_exists "docker"; then
    print_colored "yellow" "Warning: Docker not found, skipping Docker build"
else
    # Check if docker_build_and_push.sh exists
    if [ -f "build_scripts/docker_build_and_push.sh" ]; then
        print_colored "green" "Found docker build script, starting Docker image build..."
        
        # Set environment variables for Docker build
        export DOCKER_HUB_USERNAME="qicfan"
        
        # # Ask for Docker Hub password if needed
        # echo
        # read -s -p "Enter Docker Hub password (or press Enter to skip push): " DOCKER_HUB_PASSWORD
        # echo
        
        # if [ -n "$DOCKER_HUB_PASSWORD" ]; then
        #     export DOCKER_HUB_PASSWORD
        #     print_colored "cyan" "Docker Hub password provided, will push images"
        # else
        #     print_colored "yellow" "No Docker Hub password provided, will build images locally only"
        # fi
        
        # Run Docker build script
        cd build_scripts
        BUILD_AND_RELEASE_CALL=1 ./docker_build_and_push.sh -v "$TAG"
        cd ..
        
        print_colored "green" "✓ Docker image build completed"
    else
        print_colored "yellow" "Warning: docker_build_and_push.sh not found in build_scripts/"
    fi
    
    # Cleanup temp files after Docker build
    print_colored "yellow" "Cleaning up temporary build files..."
    # rm -rf "temp_build"
    print_colored "green" "✓ Temporary files cleaned up"
fi

echo
print_colored "green" "Creating GitHub Release..."

# Check if GitHub CLI is installed
if ! command_exists "gh"; then
    print_colored "yellow" "Warning: GitHub CLI (gh) not installed"
    print_colored "yellow" "Please manually upload these files to GitHub Release:"
    ls QMediaSync_*.zip QMediaSync_*.tar.gz 2>/dev/null || true
    echo
    print_colored "yellow" "Or install GitHub CLI: https://cli.github.com/"
    
    # Cleanup temp directory
    # rm -rf "temp_build"
    
    echo
    print_colored "green" "========================================"
    print_colored "green" "Build files created successfully!"
    print_colored "green" "========================================"
    echo
    print_colored "cyan" "Release files:"
    ls QMediaSync_*.zip QMediaSync_*.tar.gz 2>/dev/null || true
    
    # Ask user if they want to clean up build files
    echo
    read -p "Do you want to clean up build files? (y/n): " cleanup
    if [ "$cleanup" = "y" ] || [ "$cleanup" = "Y" ]; then
        print_colored "yellow" "Cleaning up build files..."
        rm -f QMediaSync_*.zip QMediaSync_*.tar.gz 2>/dev/null || true
        print_colored "green" "✓ Build files cleaned up"
    else
        print_colored "yellow" "Build files preserved"
    fi
    
    exit 0
fi

print_colored "cyan" "Using GitHub CLI to create release..."

# Create release notes temp file with proper encoding
if command_exists "iconv"; then
    echo "$RELEASE_BODY" | iconv -f UTF-8 -t UTF-8 > "release_body.txt" 2>/dev/null || echo "$RELEASE_BODY" > "release_body.txt"
else
    echo "$RELEASE_BODY" > "release_body.txt"
fi

# Create GitHub Release
if gh release create "$TAG" \
    --repo "qicfan/qmediasync" \
    --title "Release $TAG" \
    --notes-file "release_body.txt" \
    QMediaSync_*.zip \
    QMediaSync_*.tar.gz; then
    
    echo
    print_colored "green" "✓ GitHub Release created successfully in qicfan/qmediasync!"
    
    # Send Telegram notification after successful release
    print_colored "cyan" "Sending release notes to Telegram..."
    TELEGRAM_BOT_TOKEN="${TELEGRAM_BOT_TOKEN:-}"
    TELEGRAM_CHAT_ID="${TELEGRAM_CHAT_ID:-}"
    
    # Escape special characters for JSON
    TELEGRAM_MESSAGE=$(echo "$RELEASE_BODY" | sed 's/\\/\\\\/g' | sed 's/"/\\"/g' | awk '{printf "%s\\n", $0}')
    
    # Send message to Telegram using Markdown format
    TELEGRAM_RESPONSE=$(curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
        -H "Content-Type: application/json" \
        -d "{
            \"chat_id\": \"${TELEGRAM_CHAT_ID}\",
            \"text\": \"${TELEGRAM_MESSAGE}\",
            \"parse_mode\": \"Markdown\"
        }")
    
    if echo "$TELEGRAM_RESPONSE" | grep -q '"ok":true'; then
        print_colored "green" "✓ Release notes sent to Telegram successfully"
        
        # Send MeoW notification after successful Telegram message
        print_colored "cyan" "Sending release notes to MeoW..."
        MEOW_API_URL="${MEOW_API_URL:-}"
        
        # Escape special characters for JSON
        MEOW_MESSAGE=$(echo "$RELEASE_BODY" | sed 's/\\/\\\\/g' | sed 's/"/\\"/g' | awk '{printf "%s\\n", $0}')
        
        # Send message to MeoW
        MEOW_RESPONSE=$(curl -s -X POST "$MEOW_API_URL" \
            -H "Content-Type:  text/plain" \
            -d "${MEOW_MESSAGE}")
        
        if echo "$MEOW_RESPONSE" | grep -q '"success":true'; then
            print_colored "green" "✓ Release notes sent to MeoW successfully"
        else
            print_colored "yellow" "Warning: Failed to send message to MeoW"
            print_colored "yellow" "Response: $MEOW_RESPONSE"
        fi
    else
        print_colored "yellow" "Warning: Failed to send message to Telegram"
        print_colored "yellow" "Response: $TELEGRAM_RESPONSE"
    fi
else
    print_colored "red" "Error: Failed to create GitHub Release"
fi

# Cleanup temp files
# rm -rf "temp_build"
rm -f "release_body.txt" 2>/dev/null || true

echo
print_colored "green" "========================================"
print_colored "green" "All operations completed!"
print_colored "green" "========================================"
echo
print_colored "cyan" "Release files:"
ls QMediaSync_*.zip QMediaSync_*.tar.gz 2>/dev/null || true

# Ask user if they want to clean up build files
echo
read -p "Do you want to clean up build files? (y/n): " cleanup
if [ "$cleanup" = "y" ] || [ "$cleanup" = "Y" ]; then
    print_colored "yellow" "Cleaning up build files..."
    rm -f QMediaSync_*.zip QMediaSync_*.tar.gz 2>/dev/null || true
    print_colored "green" "✓ Build files cleaned up"
else
    print_colored "yellow" "Build files preserved"
fi