param(
    [string]$Gateway = "http://127.0.0.1:18888",
    [int]$Users = 50,
    [int]$MessagesPerUser = 100,
    [double]$Rps = 0,
    [int]$FixturesCount = 3000,
    [string]$Output = "./result.json",
    [switch]$SkipSmoke,
    [switch]$SkipBenchmark,
    [switch]$NoFixtures,
    [switch]$NoQuiet
)

$ErrorActionPreference = "Stop"
$env:PYTHONIOENCODING = "utf-8"

function Convert-GatewayHttpToWs([string]$HttpUrl) {
    $trimmed = $HttpUrl.TrimEnd("/")
    if ($trimmed.StartsWith("https://")) {
        return "wss://" + $trimmed.Substring("https://".Length) + "/ws"
    }
    if ($trimmed.StartsWith("http://")) {
        return "ws://" + $trimmed.Substring("http://".Length) + "/ws"
    }
    return $trimmed + "/ws"
}

$gatewayHttp = $Gateway.TrimEnd("/")
$gatewayWs = Convert-GatewayHttpToWs $gatewayHttp
$env:AIM_GATEWAY_HTTP = $gatewayHttp
$env:AIM_GATEWAY_WS = $gatewayWs

Write-Host "AIM gateway HTTP: $gatewayHttp"
Write-Host "AIM gateway WS:   $gatewayWs"
Write-Host "PYTHONIOENCODING: $env:PYTHONIOENCODING"

if (-not $NoFixtures) {
    Write-Host "`n==> Generating fixtures ($FixturesCount)"
    python .\generate_fixtures.py --count $FixturesCount
}

if (-not $SkipSmoke) {
    Write-Host "`n==> Smoke: aim_test.py run-all"
    python .\aim_test.py run-all
}

if (-not $SkipBenchmark) {
    Write-Host "`n==> Benchmark: ws-message users=$Users messages/user=$MessagesPerUser rps=$Rps"
    $benchArgs = @(
        ".\benchmark.py",
        "ws-message",
        "--gateway", $gatewayHttp,
        "--users", "$Users",
        "--messages-per-user", "$MessagesPerUser",
        "--output", $Output
    )
    if ($Rps -gt 0) {
        $benchArgs += @("--rps", "$Rps")
    }
    if ($NoFixtures) {
        $benchArgs += "--no-fixtures"
    }
    if (-not $NoQuiet) {
        $benchArgs += "--quiet"
    }
    python @benchArgs
}

Write-Host "`nDone. Report: $Output"
