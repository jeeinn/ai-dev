# scripts/windows/e2e-setup-gitea.ps1
# Creates org/repo/webhook/initial commit for local E2E. Requires GITEA_ADMIN_TOKEN.
# Run from repo root: powershell -NoProfile -File scripts/windows/e2e-setup-gitea.ps1
param(
  [string]$GiteaURL = "http://localhost:3000",
  [string]$Owner = "e2e",
  [string]$Repo = "gateway-poc",
  [string]$WebhookURL = "http://127.0.0.1:8080/webhook/gitea",
  [string]$WebhookSecret = "local-e2e-webhook-2026"
)

$ErrorActionPreference = "Stop"
if (-not $env:GITEA_ADMIN_TOKEN) {
  if (Test-Path "data/e2e-env.local") {
    Get-Content "data/e2e-env.local" | ForEach-Object {
      if ($_ -match '^\s*([^=]+)=(.*)$') {
        Set-Item -Path "env:$($Matches[1].Trim())" -Value $Matches[2].Trim()
      }
    }
  }
}
if (-not $env:GITEA_ADMIN_TOKEN) { throw "GITEA_ADMIN_TOKEN missing" }

$headers = @{
  Authorization = "token $($env:GITEA_ADMIN_TOKEN)"
  "Content-Type" = "application/json"
}

function Invoke-Gitea($Method, $Path, $Body = $null) {
  $uri = "$GiteaURL$Path"
  if ($null -eq $Body) {
    return Invoke-RestMethod -Method $Method -Uri $uri -Headers $headers
  }
  $json = if ($Body -is [string]) { $Body } else { $Body | ConvertTo-Json -Depth 10 -Compress }
  return Invoke-RestMethod -Method $Method -Uri $uri -Headers $headers -Body $json
}

Write-Host "== Ensure org $Owner =="
try {
  Invoke-Gitea GET "/api/v1/orgs/$Owner" | Out-Null
  Write-Host "org exists"
} catch {
  Invoke-Gitea POST "/api/v1/orgs" @{
    username = $Owner
    visibility = "public"
  } | Out-Null
  Write-Host "org created"
}

Write-Host "== Ensure repo $Owner/$Repo =="
$repoExists = $false
try {
  Invoke-Gitea GET "/api/v1/repos/$Owner/$Repo" | Out-Null
  $repoExists = $true
  Write-Host "repo exists"
} catch {
  Invoke-Gitea POST "/api/v1/orgs/$Owner/repos" @{
    name = $Repo
    private = $false
    auto_init = $true
    default_branch = "main"
    description = "Gateway local E2E test repo"
  } | Out-Null
  Write-Host "repo created"
  Start-Sleep -Seconds 2
}

# Seed files via Contents API (skills + README tweak)
function Put-File($path, $content, $message) {
  $b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($content))
  $sha = $null
  try {
    $existing = Invoke-Gitea GET "/api/v1/repos/$Owner/$Repo/contents/$([uri]::EscapeDataString($path))?ref=main"
    $sha = $existing.sha
  } catch {}
  $body = @{
    content = $b64
    message = $message
    branch = "main"
  }
  if ($sha) { $body.sha = $sha }
  Invoke-Gitea PUT "/api/v1/repos/$Owner/$Repo/contents/$path" $body | Out-Null
  Write-Host "wrote $path"
}

$skill = @"
---
name: hello
description: Repo-local E2E skill for gateway-poc
---

# Hello Skill

Minimal skill used by E2E to verify list_skills / load_skill.
"@
Put-File ".agents/skills/hello/SKILL.md" $skill "chore: add e2e hello skill"

$readme = @"
# gateway-poc

Local E2E test repository for Gitea Agent Gateway.

## Files

- README.md (this file)
- .agents/skills/hello/SKILL.md
"@
Put-File "README.md" $readme "chore: seed README for e2e"

Write-Host "== Ensure webhook =="
$hooks = Invoke-Gitea GET "/api/v1/repos/$Owner/$Repo/hooks"
$existing = $hooks | Where-Object { $_.config.url -eq $WebhookURL }
if ($existing) {
  Write-Host "webhook exists id=$($existing.id)"
} else {
  $hook = Invoke-Gitea POST "/api/v1/repos/$Owner/$Repo/hooks" @{
    type = "gitea"
    active = $true
    config = @{
      url = $WebhookURL
      content_type = "json"
      secret = $WebhookSecret
      http_method = "post"
    }
    events = @("issues", "issue_comment", "pull_request", "pull_request_comment")
  }
  Write-Host "webhook created id=$($hook.id)"
}

Write-Host "SETUP_OK repo=$Owner/$Repo"
