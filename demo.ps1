param(
  [Parameter(Position = 0)] [string] $Command = "seed",
  [string[]] $Args
)
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $root
if ($Command -eq "seed") {
  & go run ./backend/cmd/seed @args
  exit $LASTEXITCODE
}
Write-Host "Unknown command: $Command (only 'seed' is supported)" -ForegroundColor Red
exit 1
