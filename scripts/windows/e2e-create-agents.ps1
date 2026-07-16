# scripts/windows/e2e-create-agents.ps1
# Creates or updates E2E agents via Gateway API after Gateway is up.
# Run from repo root: powershell -NoProfile -File scripts/windows/e2e-create-agents.ps1
param(
  [string]$GatewayURL = "http://127.0.0.1:8080",
  [string]$Repo = "e2e/gateway-poc",
  [string]$ApiToken = ""
)

$ErrorActionPreference = "Stop"
if (-not $ApiToken) {
  if (Test-Path "data/e2e-env.local") {
    Get-Content "data/e2e-env.local" | ForEach-Object {
      if ($_ -match '^\s*([^=]+)=(.*)$') {
        Set-Item -Path "env:$($Matches[1].Trim())" -Value $Matches[2].Trim()
      }
    }
  }
  $ApiToken = if ($env:API_AUTH_TOKEN) { $env:API_AUTH_TOKEN } else { "dev-api-token" }
}

$headers = @{
  Authorization = "Bearer $ApiToken"
  "Content-Type" = "application/json"
}

function Ensure-Agent($body) {
  $list = @(Invoke-RestMethod -Uri "$GatewayURL/api/agents" -Headers $headers)
  $found = $null
  foreach ($a in $list) {
    if ("$($a.gitea_username)" -eq "$($body.gitea_username)") { $found = $a; break }
  }
  if ($found) {
    $id = [int]$found.id
    $existing = Invoke-RestMethod -Uri "$GatewayURL/api/agents/$id" -Headers $headers
    $patch = @{
      name = $body.name
      provider = $body.provider
      model = $body.model
      max_output_tokens = $body.max_output_tokens
      max_input_tokens = $body.max_input_tokens
      temperature = $body.temperature
      timeout = $body.timeout
      status = "active"
      role = $body.role
      backend = $body.backend
      repos = $body.repos
      system_prompt = "$($existing.system_prompt)"
      user_template = "$($existing.user_template)"
    }
    if ($body.tool_pack) { $patch.tool_pack = $body.tool_pack }
    if ($body.mcp_servers) { $patch.mcp_servers = $body.mcp_servers }
    if ($body.backend_options) { $patch.backend_options = $body.backend_options }
    $json = $patch | ConvertTo-Json -Depth 8 -Compress
    $updated = Invoke-RestMethod -Method PUT -Uri "$GatewayURL/api/agents/$id" -Headers $headers -Body $json
    Write-Host "agent updated id=$id name=$($body.name) provider=$($body.provider) backend=$($body.backend)"
    return $updated
  }
  $json = $body | ConvertTo-Json -Depth 8 -Compress
  $created = Invoke-RestMethod -Method POST -Uri "$GatewayURL/api/agents" -Headers $headers -Body $json
  $agent = if ($created.agent) { $created.agent } else { $created }
  Write-Host "agent created id=$($agent.id) name=$($agent.name)"
  return $agent
}

$agents = @(
  @{
    name = "e2e-analyze"
    gitea_username = "e2e-analyze"
    role = "analyze"
    provider = "sensenova"
    model = "deepseek-v4-flash"
    backend = "internal"
    tool_pack = "analyze-readonly"
    repos = @($Repo)
    max_output_tokens = 4096
    max_input_tokens = 128000
    temperature = 0.3
    timeout = "5m"
  },
  @{
    name = "e2e-coder-internal"
    gitea_username = "e2e-coder-internal"
    role = "coder"
    provider = "sensenova"
    model = "deepseek-v4-flash"
    backend = "internal"
    tool_pack = "coder-default"
    mcp_servers = @("e2e-mock")
    repos = @($Repo)
    max_output_tokens = 8192
    max_input_tokens = 128000
    temperature = 0.2
    timeout = "10m"
  },
  @{
    name = "e2e-coder-opencode"
    gitea_username = "e2e-coder-opencode"
    role = "coder"
    provider = "opencode"
    model = "big-pickle"
    backend = "opencode-local"
    backend_options = @{
      opencode_provider = "opencode"
      opencode_model = "big-pickle"  # Zen paid deepseek-v4-flash blocked without billing; free path
    }
    repos = @($Repo)
    max_output_tokens = 8192
    max_input_tokens = 128000
    temperature = 0.2
    timeout = "30m"
  },
  @{
    name = "e2e-review"
    gitea_username = "e2e-review"
    role = "review"
    provider = "sensenova"
    model = "deepseek-v4-flash"
    backend = "internal"
    repos = @($Repo)
    max_output_tokens = 4096
    max_input_tokens = 128000
    temperature = 0.3
    timeout = "5m"
  },
  @{
    name = "e2e-bugfix"
    gitea_username = "e2e-bugfix"
    role = "coder"
    provider = "opencode"
    model = "big-pickle"
    backend = "opencode-local"
    backend_options = @{
      opencode_provider = "opencode"
      opencode_model = "big-pickle"
    }
    repos = @($Repo)
    max_output_tokens = 8192
    max_input_tokens = 128000
    temperature = 0.2
    timeout = "30m"
  }
)

$created = @()
foreach ($a in $agents) {
  $created += Ensure-Agent $a
}

# Add collaborators via Gitea admin token
if (-not $env:GITEA_ADMIN_TOKEN) { throw "GITEA_ADMIN_TOKEN missing for collaborators" }
$gHeaders = @{ Authorization = "token $($env:GITEA_ADMIN_TOKEN)"; "Content-Type" = "application/json" }
foreach ($u in @("e2e-analyze","e2e-coder-internal","e2e-coder-opencode","e2e-review","e2e-bugfix")) {
  try {
    Invoke-RestMethod -Method PUT -Uri "http://localhost:3000/api/v1/repos/e2e/gateway-poc/collaborators/$u" `
      -Headers $gHeaders -Body '{"permission":"admin"}' | Out-Null
    Write-Host "collaborator ok $u"
  } catch {
    Write-Host "collaborator warn $u : $($_.Exception.Message)"
  }
}

Write-Host "AGENTS_OK count=$($created.Count)"
$created | ForEach-Object { "$($_.id) $($_.name) $($_.provider) $($_.role) $($_.backend)" }
