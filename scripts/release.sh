#!/usr/bin/env bash
#
# release.sh - Muti Metroo Release Script
#
# This script automates the release process:
#   1. Version management (auto-increment or explicit)
#   2. Git commit and tag
#   3. Push to origin
#   4. Create Gitea release with AI-generated notes
#   5. Cross-platform Docker builds
#   6. Mac binary signing and notarization (Developer ID + Apple notarization)
#   7. Upload binaries to Gitea release
#   8. Build and deploy Docusaurus documentation to web server
#   9. Upload binaries to web server for public download
#
# Usage:
#   ./scripts/release.sh [options] [version]
#
# Options:
#   -t, --token TOKEN  Gitea API token (alternative to env var)
#   -h, --help         Show this help
#
# Examples:
#   ./scripts/release.sh           # Auto-increment patch version (1.2.3 -> 1.2.4)
#   ./scripts/release.sh 2.0.0     # Set explicit version
#   ./scripts/release.sh -t TOKEN 1.0.0  # Use explicit token
#
# Environment:
#   GITEA_AUTH_TOKEN - Gitea API token (can also use --token or ~/.gitea_token file)
#   GITEA_URL        - Gitea instance URL (default: https://git.aiateibad.ee)
#   GITEA_OWNER      - Repository owner (default: andris)
#   GITEA_REPO       - Repository name (default: Muti-Metroo-v4)
#   WEB_SERVER       - Documentation web server (default: srv-04.emailengine.dev)
#   WEB_SERVER_USER  - SSH user for web server (default: andris)
#   WEB_ROOT         - Web root directory (default: /var/www/muti-metroo)
#   SKIP_TESTS       - Set to 1 to skip tests
#   SKIP_PUSH        - Set to 1 to skip git push
#   DRY_RUN          - Set to 1 for dry run (no actual changes)

set -euo pipefail

# Token can be passed via CLI
CLI_TOKEN=""

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BUILD_DIR="$PROJECT_DIR/build/release"
BINARY_NAME="muti-metroo"

# Gitea configuration
GITEA_URL="${GITEA_URL:-https://git.aiateibad.ee}"
GITEA_OWNER="${GITEA_OWNER:-andris}"
GITEA_REPO="${GITEA_REPO:-Muti-Metroo-v4}"
GITEA_API="$GITEA_URL/api/v1"

# Web server configuration for documentation and downloads
WEB_SERVER="${WEB_SERVER:-srv-04.emailengine.dev}"
WEB_SERVER_USER="${WEB_SERVER_USER:-andris}"
WEB_ROOT="${WEB_ROOT:-/var/www/muti-metroo}"
DOCS_DIR="$PROJECT_DIR/docs"

# Get token from multiple sources (in order of priority):
# 1. CLI argument (--token)
# 2. Environment variable GITEA_AUTH_TOKEN
# 3. Token file ~/.gitea_token
get_gitea_token() {
    # Try CLI token first
    if [[ -n "$CLI_TOKEN" ]]; then
        echo "$CLI_TOKEN"
        return 0
    fi

    # Try environment variable using printenv
    local env_token
    env_token=$(printenv GITEA_AUTH_TOKEN 2>/dev/null || true)
    if [[ -n "$env_token" ]]; then
        echo "$env_token"
        return 0
    fi

    # Try token file
    local token_file="$HOME/.gitea_token"
    if [[ -f "$token_file" ]]; then
        cat "$token_file" | tr -d '\n'
        return 0
    fi

    return 1
}

# Build targets: os/arch
BUILD_TARGETS=(
    "darwin/arm64"      # macOS Apple Silicon
    "darwin/amd64"      # macOS Intel
    "linux/amd64"       # Linux x86_64
    "linux/arm64"       # Linux ARM64
    "windows/amd64"     # Windows x86_64
    "windows/arm64"     # Windows ARM64
)

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Logging functions
log_info() { echo -e "${BLUE}[INFO]${NC} $*" >&2; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $*" >&2; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $*" >&2; }
log_error() { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_step() { echo -e "${CYAN}==>${NC} $*" >&2; }

# Check prerequisites
check_prerequisites() {
    log_step "Checking prerequisites..."

    local missing=()

    # Required tools
    command -v git >/dev/null 2>&1 || missing+=("git")
    command -v docker >/dev/null 2>&1 || missing+=("docker")
    command -v curl >/dev/null 2>&1 || missing+=("curl")
    command -v jq >/dev/null 2>&1 || missing+=("jq")

    # Check if claude CLI is available for release notes
    if ! command -v claude >/dev/null 2>&1; then
        log_warn "Claude CLI not found - release notes will be basic"
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing[*]}"
        exit 1
    fi

    # Check Gitea token
    if [[ -z "$(get_gitea_token)" ]]; then
        log_error "GITEA_AUTH_TOKEN environment variable is required"
        exit 1
    fi

    # Check we're in a git repo
    if ! git rev-parse --git-dir >/dev/null 2>&1; then
        log_error "Not in a git repository"
        exit 1
    fi

    # Check for uncommitted changes (except this script if it's new)
    if [[ -n "$(git status --porcelain --ignore-submodules 2>/dev/null)" ]]; then
        log_warn "Working directory has uncommitted changes"
        git status --short
        echo ""
        read -p "Continue anyway? [y/N] " -n 1 -r
        echo ""
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi

    log_success "Prerequisites check passed"
}

# Get current version from git tags
get_current_version() {
    local version
    version=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
    # Remove 'v' prefix if present
    echo "${version#v}"
}

# Parse version into components
parse_version() {
    local version="$1"
    local major minor patch

    if [[ "$version" =~ ^([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        major="${BASH_REMATCH[1]}"
        minor="${BASH_REMATCH[2]}"
        patch="${BASH_REMATCH[3]}"
        echo "$major $minor $patch"
    else
        log_error "Invalid version format: $version (expected X.Y.Z)"
        exit 1
    fi
}

# Increment patch version
increment_patch() {
    local version="$1"
    read -r major minor patch <<< "$(parse_version "$version")"
    echo "$major.$minor.$((patch + 1))"
}

# Validate version is greater than current
validate_version() {
    local new_version="$1"
    local current_version="$2"

    read -r new_major new_minor new_patch <<< "$(parse_version "$new_version")"
    read -r cur_major cur_minor cur_patch <<< "$(parse_version "$current_version")"

    # Compare versions
    if [[ $new_major -gt $cur_major ]]; then
        return 0
    elif [[ $new_major -eq $cur_major ]]; then
        if [[ $new_minor -gt $cur_minor ]]; then
            return 0
        elif [[ $new_minor -eq $cur_minor ]]; then
            if [[ $new_patch -gt $cur_patch ]]; then
                return 0
            fi
        fi
    fi

    log_error "New version ($new_version) must be greater than current version ($current_version)"
    exit 1
}

# Generate release notes using Claude CLI
generate_release_notes() {
    local version="$1"
    local prev_tag="$2"

    # Log to stderr so it doesn't pollute the release notes output
    log_step "Generating release notes..." >&2

    # Get commit log since last tag
    local commits
    if [[ -n "$prev_tag" ]] && git rev-parse "$prev_tag" >/dev/null 2>&1; then
        commits=$(git log "$prev_tag"..HEAD --oneline --no-decorate 2>/dev/null || echo "Initial release")
    else
        commits=$(git log --oneline --no-decorate -20 2>/dev/null || echo "Initial release")
    fi

    # If claude CLI is available, use it for release notes
    if command -v claude >/dev/null 2>&1; then
        log_info "Using Claude to generate release notes..." >&2

        local prompt="Generate release notes for Muti Metroo version $version.

Muti Metroo is a userspace mesh networking agent written in Go that creates virtual TCP tunnels across heterogeneous transport layers (QUIC, HTTP/2, WebSocket). It enables multi-hop routing with SOCKS5 ingress and CIDR-based exit routing.

Here are the commits since the last release:
$commits

Write concise, professional release notes in markdown format with these sections:
- A brief summary (1-2 sentences)
- What's New (bullet points of new features)
- Improvements (bullet points of enhancements)
- Bug Fixes (if any, bullet points)
- Breaking Changes (if any)

Be concise. Don't include sections that have no items. Don't include commit hashes."

        local notes
        notes=$(echo "$prompt" | claude --print 2>/dev/null || echo "")

        if [[ -n "$notes" ]]; then
            echo "$notes"
            return 0
        fi
    fi

    # Fallback: basic release notes
    log_warn "Generating basic release notes..." >&2
    cat <<EOF
## Muti Metroo v$version

### Changes since last release

$(echo "$commits" | sed 's/^[a-f0-9]* /- /')

### Installation

Download the appropriate binary for your platform from the assets below.
EOF
}

# Create git tag and push
create_tag() {
    local version="$1"
    local tag="v$version"

    log_step "Creating git tag $tag..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would create tag: $tag"
        return 0
    fi

    # Create annotated tag
    git tag -a "$tag" -m "Release $version"
    log_success "Created tag $tag"

    # Push tag
    if [[ "${SKIP_PUSH:-0}" != "1" ]]; then
        log_info "Pushing tag to origin..."
        git push origin "$tag"
        log_success "Pushed tag $tag"
    fi
}

# Build binary for a specific platform using Docker
build_binary() {
    local os="$1"
    local arch="$2"
    local version="$3"
    local output_name="$BINARY_NAME-$os-$arch"

    # Add .exe extension for Windows
    if [[ "$os" == "windows" ]]; then
        output_name="${output_name}.exe"
    fi

    local output_path="$BUILD_DIR/$output_name"

    log_info "Building for $os/$arch..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would build: $output_name"
        return 0
    fi

    # Build using Docker with UPX compression for Linux/Windows
    # Note: macOS binaries are NOT compressed because UPX breaks code signing
    if [[ "$os" == "darwin" ]]; then
        # macOS: build without UPX (breaks code signing)
        docker run --rm \
            -v "$PROJECT_DIR":/app \
            -w /app \
            -e CGO_ENABLED=0 \
            -e GOOS="$os" \
            -e GOARCH="$arch" \
            golang:1.24-alpine \
            go build -trimpath -ldflags="-s -w -X main.Version=$version" \
                -o "/app/build/release/$output_name" \
                ./cmd/muti-metroo
    else
        # Linux/Windows: build and try to compress with UPX
        # Note: UPX doesn't support all targets (e.g., windows/arm64)
        docker run --rm \
            -v "$PROJECT_DIR":/app \
            -w /app \
            -e CGO_ENABLED=0 \
            -e GOOS="$os" \
            -e GOARCH="$arch" \
            golang:1.24-alpine \
            sh -c "apk add --no-cache upx >/dev/null 2>&1 && \
                   go build -trimpath -ldflags='-s -w -X main.Version=$version' \
                       -o '/app/build/release/$output_name.tmp' \
                       ./cmd/muti-metroo && \
                   if upx --best --lzma -o '/app/build/release/$output_name' \
                       '/app/build/release/$output_name.tmp' >/dev/null 2>&1; then \
                       rm '/app/build/release/$output_name.tmp'; \
                   else \
                       mv '/app/build/release/$output_name.tmp' '/app/build/release/$output_name'; \
                   fi"
    fi

    # Verify binary was created
    if [[ -f "$output_path" ]]; then
        local size
        size=$(du -h "$output_path" | cut -f1)
        log_success "Built $output_name ($size)"
    else
        log_error "Failed to build $output_name"
        return 1
    fi
}

# Sign and notarize macOS binary
sign_macos_binary() {
    local binary_path="$1"
    local binary_name
    binary_name=$(basename "$binary_path")

    # Only sign on macOS
    if [[ "$(uname)" != "Darwin" ]]; then
        log_warn "Skipping macOS signing (not on macOS)"
        return 0
    fi

    if [[ ! -f "$binary_path" ]]; then
        log_warn "Binary not found for signing: $binary_path"
        return 0
    fi

    log_step "Signing and notarizing macOS binary: $binary_name"

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would sign and notarize: $binary_path"
        return 0
    fi

    # Use the notarize script for proper signing
    if "$SCRIPT_DIR/notarize.sh" "$binary_path" "$binary_name"; then
        log_success "Signed and notarized $binary_name"
    else
        log_error "Notarization failed for $binary_name"
        log_warn "Falling back to ad-hoc signing..."
        # Fallback to ad-hoc signing if notarization fails
        if codesign --sign - --force --preserve-metadata=entitlements,requirements,flags,runtime "$binary_path" 2>/dev/null; then
            log_warn "Signed $binary_path (ad-hoc) - will trigger Gatekeeper warnings"
        else
            log_error "Ad-hoc signing also failed"
            return 1
        fi
    fi
}

# Build all platforms
build_all() {
    local version="$1"

    log_step "Building binaries for all platforms..."

    # Create build directory
    mkdir -p "$BUILD_DIR"
    rm -f "$BUILD_DIR"/*

    # Build each target
    for target in "${BUILD_TARGETS[@]}"; do
        local os="${target%/*}"
        local arch="${target#*/}"
        build_binary "$os" "$arch" "$version"
    done

    # Sign macOS binaries (both arm64 and amd64)
    sign_macos_binary "$BUILD_DIR/$BINARY_NAME-darwin-arm64"
    sign_macos_binary "$BUILD_DIR/$BINARY_NAME-darwin-amd64"

    # Create checksums
    log_step "Creating checksums..."
    if [[ "${DRY_RUN:-0}" != "1" ]]; then
        (cd "$BUILD_DIR" && shasum -a 256 * > checksums.txt)
        log_success "Created checksums.txt"
    fi

    # List built files
    log_info "Built files:"
    ls -lh "$BUILD_DIR"
}

# Create Gitea release
create_gitea_release() {
    local version="$1"
    local release_notes="$2"
    local tag="v$version"

    log_step "Creating Gitea release..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would create release: $tag"
        return 0
    fi

    # Create release via API
    local response
    local token
    token=$(get_gitea_token)
    response=$(curl -s -X POST \
        -H "Authorization: token $token" \
        -H "Content-Type: application/json" \
        "$GITEA_API/repos/$GITEA_OWNER/$GITEA_REPO/releases" \
        -d "$(jq -n \
            --arg tag "$tag" \
            --arg name "Muti Metroo $tag" \
            --arg body "$release_notes" \
            '{
                tag_name: $tag,
                name: $name,
                body: $body,
                draft: false,
                prerelease: false
            }')"
    )

    # Check for errors
    local release_id
    release_id=$(echo "$response" | jq -r '.id // empty')

    if [[ -z "$release_id" ]]; then
        log_error "Failed to create release"
        log_error "Response: $response"
        return 1
    fi

    log_success "Created release (ID: $release_id)"
    echo "$release_id"
}

# Upload asset to Gitea release
upload_asset() {
    local release_id="$1"
    local file_path="$2"
    local file_name
    file_name=$(basename "$file_path")

    log_info "Uploading $file_name..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would upload: $file_name"
        return 0
    fi

    local response
    local token
    token=$(get_gitea_token)
    response=$(curl -s -X POST \
        -H "Authorization: token $token" \
        -H "Content-Type: application/octet-stream" \
        "$GITEA_API/repos/$GITEA_OWNER/$GITEA_REPO/releases/$release_id/assets?name=$file_name" \
        --data-binary "@$file_path"
    )

    local asset_id
    asset_id=$(echo "$response" | jq -r '.id // empty')

    if [[ -z "$asset_id" ]]; then
        log_error "Failed to upload $file_name"
        log_error "Response: $response"
        return 1
    fi

    log_success "Uploaded $file_name"
}

# Upload all assets
upload_all_assets() {
    local release_id="$1"

    log_step "Uploading release assets..."

    for file in "$BUILD_DIR"/*; do
        if [[ -f "$file" ]]; then
            upload_asset "$release_id" "$file"
        fi
    done

    log_success "All assets uploaded"
}

# Update version in download page before building docs
update_download_version() {
    local version="$1"
    local download_page="$DOCS_DIR/docs/download.mdx"

    log_step "Updating version in download page..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would update version to $version in $download_page"
        return 0
    fi

    if [[ ! -f "$download_page" ]]; then
        log_error "Download page not found: $download_page"
        return 1
    fi

    # Update the version line in download.mdx
    sed -i.bak "s/Current Version: v[0-9]*\.[0-9]*\.[0-9]*/Current Version: v$version/" "$download_page"
    rm -f "$download_page.bak"

    log_success "Updated download page to v$version"
}

# Build Docusaurus documentation
build_docs() {
    log_step "Building Docusaurus documentation..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would build documentation"
        return 0
    fi

    # Check if npm is available
    if ! command -v npm >/dev/null 2>&1; then
        log_error "npm not found - cannot build documentation"
        return 1
    fi

    # Build docs
    (cd "$DOCS_DIR" && npm install --silent && npm run build)

    if [[ -d "$DOCS_DIR/build" ]]; then
        log_success "Documentation built successfully"
    else
        log_error "Documentation build failed - build directory not found"
        return 1
    fi
}

# Deploy documentation to web server
deploy_docs() {
    log_step "Deploying documentation to $WEB_SERVER..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would deploy docs to $WEB_SERVER:$WEB_ROOT"
        return 0
    fi

    # Check if rsync is available
    if ! command -v rsync >/dev/null 2>&1; then
        log_error "rsync not found - cannot deploy documentation"
        return 1
    fi

    # Check if build directory exists
    if [[ ! -d "$DOCS_DIR/build" ]]; then
        log_error "Documentation build directory not found"
        return 1
    fi

    # Deploy using rsync (exclude downloads directory to preserve it)
    rsync -avz --delete \
        --exclude 'downloads/' \
        "$DOCS_DIR/build/" \
        "$WEB_SERVER_USER@$WEB_SERVER:$WEB_ROOT/"

    log_success "Documentation deployed to $WEB_SERVER"
}

# Upload binaries to web server
upload_binaries_to_web() {
    local version="$1"

    log_step "Uploading binaries to web server..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would upload binaries to $WEB_SERVER:$WEB_ROOT/downloads"
        return 0
    fi

    # Check if rsync is available
    if ! command -v rsync >/dev/null 2>&1; then
        log_error "rsync not found - cannot upload binaries"
        return 1
    fi

    local downloads_dir="$WEB_ROOT/downloads"
    local latest_dir="$downloads_dir/latest"
    local version_dir="$downloads_dir/v$version"

    # Create directories on remote server
    ssh "$WEB_SERVER_USER@$WEB_SERVER" "mkdir -p $latest_dir $version_dir"

    # Upload to latest directory
    rsync -avz "$BUILD_DIR/" "$WEB_SERVER_USER@$WEB_SERVER:$latest_dir/"

    # Copy to versioned directory
    ssh "$WEB_SERVER_USER@$WEB_SERVER" "cp -r $latest_dir/* $version_dir/"

    log_success "Binaries uploaded to $WEB_SERVER"
    log_info "  Latest: https://muti-metroo.postalsys.ee/downloads/latest/"
    log_info "  v$version: https://muti-metroo.postalsys.ee/downloads/v$version/"
}

# Run tests
run_tests() {
    if [[ "${SKIP_TESTS:-0}" == "1" ]]; then
        log_warn "Skipping tests (SKIP_TESTS=1)"
        return 0
    fi

    log_step "Running tests..."

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_info "[DRY RUN] Would run tests"
        return 0
    fi

    # Run tests using Docker
    docker run --rm \
        -v "$PROJECT_DIR":/app \
        -w /app \
        golang:1.24-alpine \
        sh -c "apk add --no-cache git && go test -short ./..."

    log_success "Tests passed"
}

# Main release function
main() {
    local new_version="${1:-}"

    echo ""
    echo "=========================================="
    echo "   Muti Metroo Release Script"
    echo "=========================================="
    echo ""

    # Check prerequisites
    check_prerequisites

    # Get current version
    local current_version
    current_version=$(get_current_version)
    log_info "Current version: $current_version"

    # Determine new version
    if [[ -z "$new_version" ]]; then
        new_version=$(increment_patch "$current_version")
        log_info "Auto-incrementing to: $new_version"
    else
        # Remove 'v' prefix if provided
        new_version="${new_version#v}"
        validate_version "$new_version" "$current_version"
        log_info "Using specified version: $new_version"
    fi

    # Confirm
    echo ""
    log_warn "This will release version $new_version"
    log_warn "Gitea: $GITEA_URL/$GITEA_OWNER/$GITEA_REPO"
    echo ""

    if [[ "${DRY_RUN:-0}" == "1" ]]; then
        log_warn "DRY RUN MODE - No changes will be made"
    fi

    read -p "Continue? [y/N] " -n 1 -r
    echo ""
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        log_info "Aborted"
        exit 0
    fi

    echo ""

    # Run tests
    run_tests

    # Create tag
    local prev_tag
    prev_tag=$(git describe --tags --abbrev=0 2>/dev/null || echo "")
    create_tag "$new_version"

    # Build all platforms
    build_all "$new_version"

    # Generate release notes
    local release_notes
    release_notes=$(generate_release_notes "$new_version" "$prev_tag")

    # Create Gitea release
    local release_id
    release_id=$(create_gitea_release "$new_version" "$release_notes")

    # Upload assets to Gitea
    if [[ -n "$release_id" ]]; then
        upload_all_assets "$release_id"
    fi

    # Deploy documentation and binaries to web server
    update_download_version "$new_version"
    build_docs
    deploy_docs
    upload_binaries_to_web "$new_version"

    echo ""
    echo "=========================================="
    log_success "Release v$new_version complete!"
    echo "=========================================="
    echo ""
    log_info "Release URL: $GITEA_URL/$GITEA_OWNER/$GITEA_REPO/releases/tag/v$new_version"
    log_info "Documentation: https://muti-metroo.postalsys.ee/"
    log_info "Downloads: https://muti-metroo.postalsys.ee/downloads/latest/"
    echo ""
}

# Parse command-line arguments
VERSION=""
while [[ $# -gt 0 ]]; do
    case $1 in
        -t|--token)
            CLI_TOKEN="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [options] [version]"
            echo ""
            echo "Options:"
            echo "  -t, --token TOKEN  Gitea API token"
            echo "  -h, --help         Show this help"
            echo ""
            echo "Examples:"
            echo "  $0              # Auto-increment patch version"
            echo "  $0 2.0.0        # Set explicit version"
            echo "  $0 -t TOKEN 1.0.0"
            exit 0
            ;;
        -*)
            echo "Unknown option: $1" >&2
            exit 1
            ;;
        *)
            VERSION="$1"
            shift
            ;;
    esac
done

# Run main
main "$VERSION"
