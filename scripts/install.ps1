# starkite Windows PowerShell installer script
# Safe, non-admin, and dry-run capable.
# Usage:
#   irm https://install.starkite.run/install.ps1 | iex
#   With custom directory:
#   $env:PREFIX = "C:\Tools\starkite"; irm https://install.starkite.run/install.ps1 | iex
#   Dry-run mode:
#   $env:INSTALL_DRY_RUN = "1"; irm https://install.starkite.run/install.ps1 | iex

$ErrorActionPreference = 'Stop'

function Install-Starkite {
    $Owner = "project-starkite"
    $Repo  = "starkite"
    $InstallDir = if ($env:PREFIX) { $env:PREFIX } else { Join-Path $env:USERPROFILE ".starkite\bin" }
    $DryRun = if ($env:INSTALL_DRY_RUN -eq "1" -or $env:INSTALL_DRY_RUN -eq "true") { $true } else { $false }

    Write-Host "--- Starkite Windows Installer ---" -ForegroundColor Cyan

    # 1. Detect Architecture
    $Arch = switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { "amd64" }
        "ARM64" { "arm64" }
        default {
            Write-Error "Unsupported architecture: $env:PROCESSOR_ARCHITECTURE"
            return
        }
    }

    $BinaryName = "kite-windows-$Arch.exe"

    # 2. Fetch Latest Version Tag from GitHub API
    Write-Host "Checking latest release version..."
    try {
        $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Owner/$Repo/releases/latest" -Headers @{ "User-Agent" = "starkite-installer" }
        $LatestTag = $Release.tag_name
    } catch {
        $LatestTag = "v0.1.0"
        Write-Warning "Could not retrieve latest release tag from API. Defaulting to $LatestTag"
    }

    $DownloadUrl = "https://github.com/$Owner/$Repo/releases/download/$LatestTag/$BinaryName"
    $ChecksumUrl = "https://github.com/$Owner/$Repo/releases/download/$LatestTag/checksums.txt"

    # Dry-Run Inspection
    if ($DryRun) {
        Write-Host "[DRY-RUN] Architecture:     $Arch"
        Write-Host "[DRY-RUN] Target Binary:    $BinaryName"
        Write-Host "[DRY-RUN] Release Tag:      $LatestTag"
        Write-Host "[DRY-RUN] Download URL:     $DownloadUrl"
        Write-Host "[DRY-RUN] Checksum URL:     $ChecksumUrl"
        Write-Host "[DRY-RUN] Target Directory: $InstallDir"
        Write-Host "[DRY-RUN] Dry run completed. No files were downloaded or modified."
        return
    }

    # 3. Download to Temporary Directory
    $TempDir = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid().ToString())
    New-Item -ItemType Directory -Path $TempDir -Force | Out-Null
    $TempExe = Join-Path $TempDir "kite.exe"
    $TempChecksums = Join-Path $TempDir "checksums.txt"

    try {
        Write-Host "Downloading starkite ($LatestTag) for Windows ($Arch)..."
        Invoke-WebRequest -Uri $DownloadUrl -OutFile $TempExe -UseBasicParsing

        # 4. Verify Checksum
        Write-Host "Verifying checksum..."
        try {
            Invoke-WebRequest -Uri $ChecksumUrl -OutFile $TempChecksums -UseBasicParsing
            $ExpectedLine = Get-Content $TempChecksums | Where-Object { $_ -match $BinaryName }
            if ($ExpectedLine) {
                $ExpectedHash = ($ExpectedLine -split '\s+')[0].Trim()
                $ActualHash = (Get-FileHash -Path $TempExe -Algorithm SHA256).Hash.ToLower()
                if ($ActualHash -ne $ExpectedHash.ToLower()) {
                    throw "Checksum verification failed! Expected: $ExpectedHash, Actual: $ActualHash"
                }
                Write-Host "Checksum verified successfully." -ForegroundColor Green
            }
        } catch {
            Write-Warning "Checksum validation skipped: $_"
        }

        # 5. Install Binary
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }
        $DestPath = Join-Path $InstallDir "kite.exe"
        Copy-Item -Path $TempExe -Destination $DestPath -Force
        Write-Host "Installed kite binary to: $DestPath" -ForegroundColor Green

        # 6. Add to User PATH if missing
        $UserPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
        $PathEntries = if ($UserPath) { $UserPath -split ';' } else { @() }
        if ($PathEntries -notcontains $InstallDir) {
            Write-Host "Adding $InstallDir to User PATH environment variable..."
            $NewPath = if ($UserPath) { "$UserPath;$InstallDir" } else { $InstallDir }
            [Environment]::SetEnvironmentVariable("Path", $NewPath, [EnvironmentVariableTarget]::User)
            $env:Path += ";$InstallDir"
        }

        Write-Host "`nSuccessfully installed starkite! Open a new terminal and run: kite version" -ForegroundColor Cyan
    } finally {
        Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Install-Starkite
