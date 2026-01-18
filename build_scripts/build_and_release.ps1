# QMediaSync Build and Release PowerShell Script

# Check if version parameter is provided
param(
    [Parameter(Mandatory=$false)]
    [string]$Version
)

Write-Host "========================================" -ForegroundColor Green
Write-Host "QMediaSync Build and Release Script" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green

# Check if in Git repository
if (-not (Test-Path ".git")) {
    Write-Host "Error: Not a Git repository" -ForegroundColor Red
    exit 1
}

# Determine tag
if ($Version) {
    # Use provided version parameter
    $TAG = $Version
    # if (-not $TAG.StartsWith("v")) {
    #     $TAG = "v$TAG"
    # }
    # Write-Host "Using provided version: $TAG" -ForegroundColor Cyan
    
    # # Create and push tag
    # Write-Host "Creating and pushing tag: $TAG" -ForegroundColor Yellow
    # git tag $TAG
    # if ($LASTEXITCODE -ne 0) {
    #     Write-Host "Error: Failed to create tag" -ForegroundColor Red
    #     exit 1
    # }
    # git push origin $TAG
    # if ($LASTEXITCODE -ne 0) {
    #     Write-Host "Error: Failed to push tag" -ForegroundColor Red
    #     exit 1
    # }
    # Write-Host "✓ Tag created and pushed successfully" -ForegroundColor Green
} else {
    # Auto-detect existing tag
    $tagOutput = git describe --tags --exact-match 2>$null
    if (-not $tagOutput) {
        Write-Host "Error: No Git tag associated with current HEAD" -ForegroundColor Red
        Write-Host "Please create and push a tag: git tag vX.X.X && git push origin vX.X.X" -ForegroundColor Yellow
        Write-Host "Or use: .\build_and_release.ps1 -Version vX.X.X" -ForegroundColor Yellow
        exit 1
    }
    $TAG = $tagOutput.Trim()
    Write-Host "Detected tag: $TAG" -ForegroundColor Cyan
}

# Check if release notes file exists
$releaseNotesPath = ".changes/$TAG.md"
if (-not (Test-Path $releaseNotesPath)) {
    Write-Host "Warning: Release notes file $releaseNotesPath not found" -ForegroundColor Yellow
    Write-Host "Using default release notes" -ForegroundColor Yellow
    $RELEASE_BODY = "Release $TAG"
} else {
    Write-Host "Found release notes file" -ForegroundColor Green
    # Read file with UTF-8 encoding to avoid encoding issues
    $RELEASE_BODY = Get-Content $releaseNotesPath -Encoding UTF8 -Raw
}

Write-Host ""
Write-Host "Starting build..." -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green

# Create temp directory
if (Test-Path "temp_build") {
    Remove-Item "temp_build" -Recurse -Force
}
New-Item -ItemType Directory -Path "temp_build" | Out-Null

# Supported platforms and architectures
$PLATFORMS = @("windows", "linux")
$ARCHS = @("amd64", "arm64")

# Build loop
foreach ($platform in $PLATFORMS) {
    foreach ($arch in $ARCHS) {
        Write-Host ""
        Write-Host "Building $platform/$arch version..." -ForegroundColor Cyan
        
        # Set environment variables
        $env:GOOS = $platform
        $env:GOARCH = $arch
        $env:CGO_ENABLED = "0"
        
        # Determine executable name and link flags
        # Get current date in format: yyyy-mm-dd HH:MM:ss
        $PUBLISH_DATE = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
        
        if ($platform -eq "windows") {
            $EXE_NAME = "QMediaSync.exe"
            # $LDFLAGS = "-s -w -H windowsgui -X main.Version=$TAG"
            $LDFLAGS = "-s -w -X main.Version=$TAG -X 'main.PublishDate=$PUBLISH_DATE'"
        } else {
            $EXE_NAME = "QMediaSync"
            $LDFLAGS = "-s -w -X main.Version=$TAG -X 'main.PublishDate=$PUBLISH_DATE'"
        }
        
        # Build
        $buildResult = go build -ldflags $LDFLAGS -o "temp_build/$EXE_NAME"
        if ($LASTEXITCODE -ne 0) {
            Write-Host "Error: Build failed for $platform/$arch" -ForegroundColor Red
            exit 1
        }
        
        # Create archive name
        $ARCHIVE_NAME = "QMediaSync_${platform}_${arch}"
        if ($arch -eq "amd64") {
            $ARCHIVE_NAME = "QMediaSync_${platform}_x86_64"
        }
        
        # Create release directory
        New-Item -ItemType Directory -Path "temp_build/$ARCHIVE_NAME" | Out-Null
        
        # Copy files
        Copy-Item "temp_build/$EXE_NAME" "temp_build/$ARCHIVE_NAME/"
        if (Test-Path "web_statics") {
            Copy-Item "web_statics" "temp_build/$ARCHIVE_NAME/web_statics" -Recurse
        }
        if (Test-Path "scripts") {
            Copy-Item "scripts" "temp_build/$ARCHIVE_NAME/scripts" -Recurse
        }
        
        # Windows specific files
        if ($platform -eq "windows" -and (Test-Path "icon.ico")) {
            Copy-Item "icon.ico" "temp_build/$ARCHIVE_NAME/"
        }
        
        # PostgreSQL binaries
        $postgresPath = "postgres/$platform/$arch"
        if (Test-Path $postgresPath) {
            Copy-Item $postgresPath "temp_build/$ARCHIVE_NAME/postgres/$platform/$arch" -Recurse
        }
        
        # Create archive
        if ($platform -eq "windows") {
            Write-Host "Creating ${ARCHIVE_NAME}.zip" -ForegroundColor Yellow
            Compress-Archive -Path "temp_build/$ARCHIVE_NAME/*" -DestinationPath "${ARCHIVE_NAME}.zip" -Force
        } else {
            Write-Host "Creating ${ARCHIVE_NAME}.tar.gz" -ForegroundColor Yellow
            tar -czf "${ARCHIVE_NAME}.tar.gz" -C "temp_build" "$ARCHIVE_NAME"
        }
        
        # Cleanup temp files
        Remove-Item "temp_build/$ARCHIVE_NAME" -Recurse -Force
        Remove-Item "temp_build/$EXE_NAME" -Force -ErrorAction SilentlyContinue
        
        Write-Host "✓ Completed $platform/$arch version" -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "Build completed!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green

Write-Host ""
Write-Host "Creating GitHub Release..." -ForegroundColor Green

# Check if GitHub CLI is installed
$ghCheck = Get-Command "gh" -ErrorAction SilentlyContinue
if (-not $ghCheck) {
    Write-Host "Warning: GitHub CLI (gh) not installed" -ForegroundColor Yellow
    Write-Host "Please manually upload these files to GitHub Release:" -ForegroundColor Yellow
    Get-ChildItem "QMediaSync_*.zip", "QMediaSync_*.tar.gz" -ErrorAction SilentlyContinue | ForEach-Object { Write-Host $_.Name }
    Write-Host ""
    Write-Host "Or install GitHub CLI: https://cli.github.com/" -ForegroundColor Yellow
    exit 0
}

Write-Host "Using GitHub CLI to create release..." -ForegroundColor Cyan

# Create release notes temp file with UTF-8 encoding
$RELEASE_BODY | Out-File -FilePath "release_body.txt" -Encoding UTF8 -NoNewline

# Create GitHub Release
try {
    gh release create $TAG `
        --repo "qicfan/qmediasync" `
        --title "Release $TAG" `
        --notes-file "release_body.txt" `
        QMediaSync_*.zip `
        QMediaSync_*.tar.gz
    
    if ($LASTEXITCODE -ne 0) {
        throw "GitHub CLI command failed"
    }
    
    Write-Host ""
    Write-Host "✓ GitHub Release created successfully in qicfan/qmediasync!" -ForegroundColor Green
} catch {
    Write-Host "Error: Failed to create GitHub Release" -ForegroundColor Red
    Write-Host $_.Exception.Message -ForegroundColor Red
}

# Cleanup temp files
if (Test-Path "temp_build") {
    Remove-Item "temp_build" -Recurse -Force
}
if (Test-Path "release_body.txt") {
    Remove-Item "release_body.txt" -Force
}

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "All operations completed!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host ""
Write-Host "Release files:" -ForegroundColor Cyan
Get-ChildItem "QMediaSync_*.zip", "QMediaSync_*.tar.gz" -ErrorAction SilentlyContinue | ForEach-Object { Write-Host $_.Name }

# Ask user if they want to clean up build files
Write-Host ""
$cleanup = Read-Host "Do you want to clean up build files? (y/n)"
if ($cleanup -eq "y" -or $cleanup -eq "Y") {
    Write-Host "Cleaning up build files..." -ForegroundColor Yellow
    Remove-Item "QMediaSync_*.zip", "QMediaSync_*.tar.gz" -Force -ErrorAction SilentlyContinue
    Write-Host "✓ Build files cleaned up" -ForegroundColor Green
} else {
    Write-Host "Build files preserved" -ForegroundColor Yellow
}
Remove-Item Env:GOOS -ErrorAction SilentlyContinue
Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
go env -w GOOS=windows
go env -w GOARCH=amd64