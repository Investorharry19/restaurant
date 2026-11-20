param(
    [Parameter(Mandatory = $true)]
    [ValidateSet("api", "migrate")]
    [string]$Command
)

$ErrorActionPreference = "Stop"
$repositoryRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
$environmentFile = Join-Path $repositoryRoot ".env"

if (-not (Test-Path -LiteralPath $environmentFile)) {
    throw "Copy .env.example to .env and configure it before running the Go server."
}

Get-Content -LiteralPath $environmentFile | ForEach-Object {
    $line = $_.Trim()
    if ($line.Length -eq 0 -or $line.StartsWith("#")) {
        return
    }

    if ($line -notmatch "^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)\s*$") {
        throw "The .env file contains an invalid assignment."
    }

    $name = $Matches[1]
    $value = $Matches[2]
    if (
        ($value.StartsWith('"') -and $value.EndsWith('"')) -or
        ($value.StartsWith("'") -and $value.EndsWith("'"))
    ) {
        $value = $value.Substring(1, $value.Length - 2)
    }

    Set-Item -Path "Env:$name" -Value $value
}

Push-Location (Join-Path $repositoryRoot "server")
$commandExitCode = 1
try {
    & go run "./cmd/$Command"
    $commandExitCode = $LASTEXITCODE
}
finally {
    Pop-Location
}

exit $commandExitCode
