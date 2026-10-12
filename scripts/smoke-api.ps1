#Requires -Version 5.1
<#
.SYNOPSIS
  Smoke-тест API NetLynx (login, topology, discovered).

.EXAMPLE
  $env:INVETOR_URL='http://10.0.0.1:8080'
  $env:INVETOR_USER='admin'
  $env:INVETOR_PASS='...'
  .\scripts\smoke-api.ps1
#>
param(
    [string]$BaseUrl = $env:INVETOR_URL,
    [string]$User = $env:INVETOR_USER,
    [string]$Password = $env:INVETOR_PASS
)

$ErrorActionPreference = 'Stop'
if (-not $BaseUrl) { throw 'Set INVETOR_URL (e.g. http://10.0.0.1:8080)' }
if (-not $User -or -not $Password) { throw 'Set INVETOR_USER and INVETOR_PASS' }

$BaseUrl = $BaseUrl.TrimEnd('/')

function Invoke-Json {
    param([string]$Method, [string]$Path, [hashtable]$Headers = @{}, [object]$Body = $null)
    $uri = "$BaseUrl$Path"
    $params = @{
        Method      = $Method
        Uri         = $uri
        Headers     = $Headers
        ContentType = 'application/json'
        TimeoutSec  = 30
    }
    if ($null -ne $Body) {
        $params.Body = ($Body | ConvertTo-Json -Compress -Depth 6)
    }
    return Invoke-RestMethod @params
}

Write-Host "[*] health $BaseUrl/health"
$health = Invoke-Json -Method GET -Path '/health'
Write-Host "[+] version=$($health.version)"

Write-Host "[*] login"
$login = Invoke-Json -Method POST -Path '/api/v1/auth/login' -Body @{ username = $User; password = $Password }
if (-not $login.access_token) { throw 'no access_token in login response' }
$auth = @{ Authorization = "Bearer $($login.access_token)" }
Write-Host "[+] auth ok"

Write-Host "[*] topology"
$topo = Invoke-Json -Method GET -Path '/api/v1/topology' -Headers $auth
Write-Host "[+] nodes=$($topo.nodes.Count) edges=$($topo.edges.Count)"

Write-Host "[*] discovered?status=all"
$disc = Invoke-Json -Method GET -Path '/api/v1/discovered?status=all' -Headers $auth
Write-Host "[+] discovered=$($disc.Count)"

Write-Host "[*] devices"
$devs = Invoke-Json -Method GET -Path '/api/v1/devices' -Headers $auth
Write-Host "[+] devices=$($devs.Count)"

Write-Host 'OK smoke-api passed'
