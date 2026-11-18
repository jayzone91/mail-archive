param(
    [Parameter(Mandatory)]
    [string]$Executable
)

$ErrorActionPreference = 'Stop'
$binary = (Get-Item -LiteralPath $Executable).FullName
$logs = Join-Path $env:LOCALAPPDATA 'MailArchive\logs'
[void][IO.Directory]::CreateDirectory($logs)
$log = Join-Path $logs ("mailarchive-{0}-{1}.log" -f (Get-Date -Format 'yyyyMMdd-HHmmss'), $PID)
$process = Start-Process -FilePath $binary -WindowStyle Hidden -Wait -PassThru -RedirectStandardError $log
exit $process.ExitCode
