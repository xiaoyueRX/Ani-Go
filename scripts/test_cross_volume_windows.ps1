param(
    [Parameter(Mandatory)][string]$AniGo,
    [Parameter(Mandatory)][string]$QB
)
$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_ENVIRONMENT -ne 'github-hosted' -or
    $env:RUNNER_OS -ne 'Windows' -or -not (Test-Path -LiteralPath $env:RUNNER_TEMP -PathType Container)) {
    throw 'This fixture requires a GitHub-hosted Windows runner'
}

$python = (Get-Command python -CommandType Application).Source
$probe = Join-Path $PSScriptRoot 'test_cross_volume_runtime.py'
$sourcePath = & $python -c 'import tempfile; print(tempfile.gettempdir())'
if ($LASTEXITCODE -ne 0) { throw 'Cannot identify the Python temporary directory' }

function Get-PathVolume([string]$Path) {
    $root = [IO.Path]::GetPathRoot($Path)
    if ($root -notmatch '^[A-Za-z]:\\$') { throw "Expected a local drive for '$Path'" }
    $volume = Get-Volume -DriveLetter $root[0]
    if (-not $volume.UniqueId) { throw "No volume identity for '$Path'" }
    return $volume
}

$sourceVolume = Get-PathVolume $sourcePath
function Invoke-Probe([string]$Parent, [string]$Type) {
    $volume = Get-PathVolume $Parent
    if ($volume.UniqueId -eq $sourceVolume.UniqueId) { throw 'The probe requires a distinct destination volume' }
    $folder = Join-Path $Parent ('anigo-cross-volume-' + [guid]::NewGuid().ToString('N'))
    [void](New-Item -ItemType Directory -Path $folder)
    try {
        Write-Host "Cross-volume fixture: source=$($sourceVolume.UniqueId) destination=$($volume.UniqueId) type=$Type"
        & $python $probe $AniGo $QB $folder
        if ($LASTEXITCODE -ne 0) { throw "Cross-volume test exited $LASTEXITCODE" }
    } finally {
        [IO.Directory]::Delete($folder, $true)
    }
}

# Use only the workflow's directories when the runner already has two volumes.
foreach ($parent in @($env:RUNNER_TEMP, $env:GITHUB_WORKSPACE)) {
    if ($parent -and (Test-Path -LiteralPath $parent -PathType Container) -and
        (Get-PathVolume $parent).UniqueId -ne $sourceVolume.UniqueId) {
        Invoke-Probe $parent 'existing'
        Write-Host 'Cross-volume fixture cleanup complete'
        return
    }
}

$scratch = Join-Path $env:RUNNER_TEMP ('anigo-cross-volume-' + [guid]::NewGuid().ToString('N'))
[void](New-Item -ItemType Directory -Path $scratch)
$image = Join-Path $scratch 'library.vhdx'
try {
    # DiskPart only creates a new image. All partition/format operations below are
    # bound to the disk obtained from that exact image, never to a guessed disk number.
    $script = Join-Path $scratch 'create.txt'
    $command = 'create vdisk file="' + $image + '" maximum=256 type=expandable'
    $encoding = [Text.Encoding]::ASCII
    if ($encoding.GetString($encoding.GetBytes($command)) -cne $command) {
        throw 'DiskPart needs an ASCII runner temp path for the image creation command'
    }
    [IO.File]::WriteAllLines($script, [string[]]@($command, 'exit'), $encoding)
    & diskpart.exe /s $script
    if ($LASTEXITCODE -ne 0 -or -not (Test-Path -LiteralPath $image -PathType Leaf)) { throw 'VHDX creation failed' }
    Mount-DiskImage -ImagePath $image -Access ReadWrite -NoDriveLetter | Out-Null
    $disk = @(Get-DiskImage -ImagePath $image | Get-Disk)
    if ($disk.Count -ne 1 -or $disk[0].IsBoot -or $disk[0].IsSystem -or $disk[0].PartitionStyle -ne 'RAW') {
        throw 'Expected exactly one new, empty, non-system test disk'
    }
    $partition = $disk[0] | Initialize-Disk -PartitionStyle GPT -PassThru | New-Partition -UseMaximumSize -AssignDriveLetter
    $volume = $partition | Format-Volume -FileSystem NTFS -NewFileSystemLabel 'AniGoCI' -Force -Confirm:$false
    if (-not $volume.DriveLetter -or $volume.FileSystem -ne 'NTFS') { throw 'The test NTFS volume has no usable drive letter' }
    Invoke-Probe "$($volume.DriveLetter):\" 'NTFS-VHDX'
} finally {
    if ((Test-Path -LiteralPath $image -PathType Leaf) -and (Get-DiskImage -ImagePath $image).Attached) {
        Dismount-DiskImage -ImagePath $image -ErrorAction Stop | Out-Null
    }
    [IO.Directory]::Delete($scratch, $true)
    Write-Host 'Cross-volume fixture cleanup complete'
}
