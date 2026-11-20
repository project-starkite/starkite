# starkite Windows PowerShell installer script
# Safe, non-admin, and dry-run capable.
# Usage:
#   irm https://starkite.run/install.ps1 | iex
#   With custom directory:
#   $env:PREFIX = "C:\Tools\starkite"; irm https://starkite.run/install.ps1 | iex
#   Dry-run mode:
#   $env:INSTALL_DRY_RUN = "1"; irm https://starkite.run/install.ps1 | iex

$ErrorActionPreference = 'Stop'

function Install-Starkite {
    $Owner = "project-starkite"
    $Repo  = "starkite"
    $HomeDir = if ($env:USERPROFILE) { $env:USERPROFILE } elseif ($env:HOME) { $env:HOME } else { "." }
    $InstallDir = if ($env:PREFIX) { $env:PREFIX } else { Join-Path $HomeDir ".starkite\bin" }
    $DryRun = if ($env:INSTALL_DRY_RUN -eq "1" -or $env:INSTALL_DRY_RUN -eq "true") { $true } else { $false }

    Write-Host "--- Starkite Windows Installer ---" -ForegroundColor Cyan

    # 1. Detect Architecture
    $Arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") {
        "arm64"
    } elseif ($env:PROCESSOR_ARCHITECTURE -eq "AMD64") {
        "amd64"
    } elseif ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture -eq [System.Runtime.InteropServices.Architecture]::Arm64) {
        "arm64"
    } else {
        "amd64"
    }

    $BinaryName = "kite-windows-$Arch.exe"

    # 2. Fetch Latest Version Tag via GitHub redirect (avoids unauthenticated API rate limits),
    # falling back to the GitHub API when needed.
    $LatestTag = if ($env:STARKITE_VERSION) { $env:STARKITE_VERSION } elseif ($env:VERSION) { $env:VERSION } else { "" }
    if ([string]::IsNullOrWhiteSpace($LatestTag)) {
        Write-Host "Checking latest release version..."
        try {
            $HeadResp = Invoke-WebRequest -Uri "https://github.com/$Owner/$Repo/releases/latest" -Method Head -UseBasicParsing -Headers @{ "User-Agent" = "starkite-installer" }
            $FinalUri = if ($HeadResp.BaseResponse.RequestMessage -and $HeadResp.BaseResponse.RequestMessage.RequestUri) {
                $HeadResp.BaseResponse.RequestMessage.RequestUri.AbsoluteUri
            } elseif ($HeadResp.BaseResponse.ResponseUri) {
                $HeadResp.BaseResponse.ResponseUri.AbsoluteUri
            } else { "" }
            if ($FinalUri -match '/releases/tag/([^/?#]+)$') {
                $LatestTag = $Matches[1]
            }
        } catch {
            $LatestTag = ""
        }
        if ([string]::IsNullOrWhiteSpace($LatestTag)) {
            try {
                $Release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Owner/$Repo/releases/latest" -Headers @{ "User-Agent" = "starkite-installer" }
                $LatestTag = $Release.tag_name
            } catch {
                if ($DryRun) {
                    $LatestTag = "dry-run"
                } else {
                    throw "Could not retrieve latest release tag from GitHub: $_"
                }
            }
        }
    }
    if ([string]::IsNullOrWhiteSpace($LatestTag)) {
        if ($DryRun) {
            $LatestTag = "dry-run"
        } else {
            throw "Could not retrieve latest release tag from GitHub API."
        }
    }

    $BaseUrl = if ($env:STARKITE_DOWNLOAD_BASE_URL) { $env:STARKITE_DOWNLOAD_BASE_URL.TrimEnd('/') } else { "https://github.com/$Owner/$Repo/releases/download/$LatestTag" }
    $DownloadUrl = "$BaseUrl/$BinaryName"
    $ChecksumUrl = "$BaseUrl/checksums.txt"

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
        Invoke-WebRequest -Uri $ChecksumUrl -OutFile $TempChecksums -UseBasicParsing
        $ExpectedHash = $null
        foreach ($line in (Get-Content $TempChecksums)) {
            $parts = ($line.Trim() -split '\s+')
            if ($parts.Count -ge 2 -and $parts[1].TrimStart('*') -eq $BinaryName) {
                $ExpectedHash = $parts[0].Trim()
                break
            }
        }
        if ([string]::IsNullOrWhiteSpace($ExpectedHash)) {
            throw "Checksum for $BinaryName not found in checksums.txt"
        }
        $ActualHash = (Get-FileHash -Path $TempExe -Algorithm SHA256).Hash.ToLower()
        if ($ActualHash -ne $ExpectedHash.ToLower()) {
            throw "Checksum verification failed! Expected: $ExpectedHash, Actual: $ActualHash"
        }
        Write-Host "Checksum verified successfully." -ForegroundColor Green

        # 5. Install Binary
        if (-not (Test-Path $InstallDir)) {
            New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        }
        $DestPath = Join-Path $InstallDir "kite.exe"
        Copy-Item -Path $TempExe -Destination $DestPath -Force
        Write-Host "Installed kite binary to: $DestPath" -ForegroundColor Green

        # 6. Add to User PATH if running on Windows and missing
        $IsWin = $IsWindows -or ($null -eq $IsWindows -and [System.Environment]::OSVersion.Platform -eq [System.PlatformID]::Win32NT)
        if ($IsWin) {
            try {
                $UserPath = [Environment]::GetEnvironmentVariable("Path", [EnvironmentVariableTarget]::User)
                $PathEntries = if ($UserPath) { $UserPath -split ';' } else { @() }
                if ($PathEntries -notcontains $InstallDir) {
                    Write-Host "Adding $InstallDir to User PATH environment variable..."
                    $NewPath = if ($UserPath) { "$UserPath;$InstallDir" } else { $InstallDir }
                    [Environment]::SetEnvironmentVariable("Path", $NewPath, [EnvironmentVariableTarget]::User)
                    $env:Path += ";$InstallDir"
                }
            } catch {
                Write-Warning "Could not update User PATH environment variable: $_"
            }
        }

        Write-Host "`nSuccessfully installed starkite! Open a new terminal and run: kite version" -ForegroundColor Cyan
    } finally {
        Remove-Item -Path $TempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Install-Starkite
