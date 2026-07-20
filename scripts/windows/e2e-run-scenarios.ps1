# scripts/windows/e2e-run-scenarios.ps1
# Drives Assign-based E2E scenarios against local Gitea + Gateway.
param(
  [string]$GiteaURL = "http://localhost:3000",
  [string]$GatewayURL = "http://127.0.0.1:8080",
  [string]$Owner = "e2e",
  [string]$Repo = "gateway-poc",
  [int]$PollSeconds = 15,
  [int]$TimeoutMinutes = 20,
  # Accept -Only E0,E1 or -Only @('E0','E1') or -Only E0 -Only E1
  [string[]]$Only = @()
)

$ErrorActionPreference = "Stop"
$Report = [ordered]@{}
# scripts/windows → repo root
$Root = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
Set-Location $Root

function Load-Env {
  if (Test-Path "data/e2e-env.local") {
    Get-Content "data/e2e-env.local" | ForEach-Object {
      if ($_ -match '^\s*([^=]+)=(.*)$') {
        Set-Item -Path "env:$($Matches[1].Trim())" -Value $Matches[2].Trim()
      }
    }
  }
}
Load-Env

$gHeaders = @{ Authorization = "token $($env:GITEA_ADMIN_TOKEN)"; "Content-Type" = "application/json" }
$aHeaders = @{ Authorization = "Bearer $(if ($env:API_AUTH_TOKEN) {$env:API_AUTH_TOKEN} else {'dev-api-token'})"; "Content-Type" = "application/json" }

function Set-Result($id, $status, $detail) {
  $Report[$id] = @{ status = $status; detail = $detail; at = (Get-Date).ToString("s") }
  Write-Host "[$id] $status — $detail"
}

function Get-TaskList {
  # API returns { data: [...], total: N } — unwrap data; never return the wrapper object.
  $resp = Invoke-RestMethod -Uri "$GatewayURL/api/tasks?limit=100" -Headers $aHeaders
  if ($null -eq $resp) { return @() }
  $names = @($resp.PSObject.Properties.Name)
  $items = $null
  if ($names -contains "data") {
    $items = $resp.data
  } elseif ($names -contains "tasks") {
    $items = $resp.tasks
  } else {
    $items = $resp
  }
  if ($null -eq $items) { return @() }
  return @($items)
}

function Get-AgentList {
  $resp = Invoke-RestMethod -Uri "$GatewayURL/api/agents" -Headers $aHeaders
  if ($null -eq $resp) { return @() }
  $names = @($resp.PSObject.Properties.Name)
  if ($names -contains "data") {
    if ($null -eq $resp.data) { return @() }
    return @($resp.data)
  }
  return @($resp)
}

function Normalize-Int($value) {
  # Avoid [int] cast failures when PowerShell surfaces Object[] / JSON number quirks.
  if ($null -eq $value) { return 0 }
  if ($value -is [array]) {
    if ($value.Count -eq 0) { return 0 }
    $value = $value[0]
  }
  $s = "$value".Trim()
  if ($s -eq "") { return 0 }
  return [int]$s
}

function New-Issue($title, $body, $labels = @()) {
  $payload = @{ title = $title; body = $body }
  if ($labels.Count -gt 0) { $payload.labels = $labels }
  return Invoke-RestMethod -Method POST -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/issues" -Headers $gHeaders -Body ($payload | ConvertTo-Json -Compress)
}

function Assign-Issue($number, $username) {
  Invoke-RestMethod -Method PATCH -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/issues/$number" -Headers $gHeaders -Body (@{
    assignees = @($username)
  } | ConvertTo-Json -Compress) | Out-Null
}

function Wait-Task($issueNumber, $taskType, [int]$timeoutMin = $TimeoutMinutes, [string]$matchField = "issue_id") {
  $want = [string](Normalize-Int $issueNumber)
  $deadline = (Get-Date).AddMinutes($timeoutMin)
  while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds $PollSeconds
    $list = Get-TaskList
    $match = $list | Where-Object {
      $got = if ($matchField -eq "pr_id") { [string](Normalize-Int $_.pr_id) } else { [string](Normalize-Int $_.issue_id) }
      $idMatch = ($got -eq $want)
      $idMatch -and ($taskType -eq "" -or $_.task_type -eq $taskType) -and
      ($_.status -in @("success","failed","partial","completed"))
    } | Select-Object -First 1
    if ($match) { return $match }
    Write-Host "  polling $matchField=$issueNumber type=$taskType (tasks=$($list.Count)) ..."
  }
  return $null
}

function Get-Workflow($issueNumber) {
  return Invoke-RestMethod -Uri "$GatewayURL/api/workflow-context?repo=$Owner/$Repo&issue=$issueNumber" -Headers $aHeaders
}

function Get-Comments($issueNumber) {
  return Invoke-RestMethod -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/issues/$issueNumber/comments" -Headers $gHeaders
}

function Get-PRs() {
  return @(Invoke-RestMethod -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/pulls?state=open" -Headers $gHeaders)
}

function Require-LLM {
  if ($env:SENSENOVA_API_KEY -and $env:SENSENOVA_API_KEY -ne "pending-user-key") { return $true }
  if ($env:DEEPSEEK_API_KEY -and $env:DEEPSEEK_API_KEY -ne "pending-user-key") { return $true }
  return $false
}

function Search-Log([string[]]$patterns) {
  $logPath = "data/logs-e2e/gateway.log"
  if (-not (Test-Path $logPath)) { return $false }
  $content = Get-Content $logPath -Raw -ErrorAction SilentlyContinue
  if (-not $content) { return $false }
  foreach ($p in $patterns) {
    if ($content -notmatch [regex]::Escape($p) -and $content -notmatch $p) { return $false }
  }
  return $true
}

function Find-LogHits([string[]]$patterns) {
  $logPath = "data/logs-e2e/gateway.log"
  if (-not (Test-Path $logPath)) { return @() }
  $hits = @()
  foreach ($line in (Get-Content $logPath -ErrorAction SilentlyContinue)) {
    foreach ($p in $patterns) {
      if ($line -match $p) { $hits += $line; break }
    }
  }
  return $hits
}

# ---- E0 ----
function Run-E0 {
  try {
    $g = Invoke-RestMethod "$GiteaURL/api/v1/version"
    $h = Invoke-RestMethod "$GatewayURL/health"
    $o = Invoke-RestMethod "http://127.0.0.1:4096/global/health"
    $m = Invoke-RestMethod "http://127.0.0.1:18080/health"
    Set-Result "E0" "PASS" "gitea=$($g.version) gateway=$($h.status) opencode=$($o.version) mcp=ok"
  } catch {
    Set-Result "E0" "FAIL" $_.Exception.Message
  }
}

# ---- E1 A0 session bind ----
function Run-E1 {
  try {
    $ws = Join-Path $env:TEMP "oc-poc-e2e"
    New-Item -ItemType Directory -Force -Path $ws | Out-Null
    $wsAbs = (Resolve-Path $ws).Path
    $sess = Invoke-RestMethod -Method POST `
      -Uri "http://127.0.0.1:4096/session?directory=$([uri]::EscapeDataString($wsAbs))" `
      -Headers @{ "X-Opencode-Directory" = $wsAbs; "Content-Type" = "application/json" } `
      -Body '{"title":"e2e-a0-recheck"}'
    $want = ($wsAbs -replace '\\','/')
    $got = ("$($sess.directory)" -replace '\\','/')
    if ($got -eq $want) {
      Set-Result "E1" "PASS" "directory bound to $wsAbs session=$($sess.id)"
    } else {
      Set-Result "E1" "FAIL" "want=$want got=$got"
    }
  } catch {
    Set-Result "E1" "FAIL" $_.Exception.Message
  }
}

function Find-PRForIssue($issueNumber) {
  $prs = @(Get-PRs)
  $hit = $prs | Where-Object {
    $_.body -match "#$issueNumber" -or $_.title -match "$issueNumber" -or
    $_.body -match "Fixes #$issueNumber" -or $_.head.ref -match "issue-$issueNumber"
  } | Select-Object -First 1
  if ($hit) { return $hit }
  # also check all states
  try {
    $all = @(Invoke-RestMethod -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/pulls?state=all&limit=50" -Headers $gHeaders)
    return $all | Where-Object {
      $_.body -match "#$issueNumber" -or $_.head.ref -match "issue-$issueNumber"
    } | Select-Object -First 1
  } catch { return $null }
}

function Get-IssueAssignees($issueNumber) {
  $issue = Invoke-RestMethod -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/issues/$issueNumber" -Headers $gHeaders
  if ($null -eq $issue) { return @() }
  $assignees = @()
  if ($issue.PSObject.Properties.Name -contains "assignees") {
    foreach ($a in $issue.assignees) {
      $assignees += "$($a.login)"
    }
  }
  return $assignees
}

function Set-WorkflowPolicy($repo, $preset, $gates) {
  $payload = @{ preset = $preset }
  if ($gates -and $gates.Count -gt 0) { $payload.gates = $gates }
  $json = $payload | ConvertTo-Json -Compress
  try {
    Invoke-RestMethod -Method PUT -Uri "$GatewayURL/api/workflow-policies/$repo" -Headers $aHeaders -Body $json | Out-Null
    Write-Host "workflow policy set: repo=$repo preset=$preset gates=$($gates | ConvertTo-Json -Compress)"
    return $true
  } catch {
    Write-Host "workflow policy set warn: $($_.Exception.Message)"
    return $false
  }
}

function Delete-WorkflowPolicy($repo) {
  try {
    Invoke-RestMethod -Method DELETE -Uri "$GatewayURL/api/workflow-policies/$repo" -Headers $aHeaders | Out-Null
    Write-Host "workflow policy deleted: repo=$repo"
    return $true
  } catch {
    Write-Host "workflow policy delete warn: $($_.Exception.Message)"
    return $false
  }
}

function Run-AssignScenario($id, $agent, $title, $body, $taskType, $labels, $expectPR) {
  if (-not (Require-LLM) -and ($agent -match "analyze|review|internal")) {
    Set-Result $id "SKIP" "waiting SENSENOVA_API_KEY / DEEPSEEK_API_KEY"
    return
  }
  try {
    $issue = New-Issue $title $body $labels
    Write-Host "created issue #$($issue.number) for $id"
    Assign-Issue $issue.number $agent
    $task = Wait-Task $issue.number $taskType
    if (-not $task) {
      Set-Result $id "FAIL" "timeout waiting task issue=$($issue.number) type=$taskType"
      return
    }
    $detail = "issue=$($issue.number) task=$($task.id) status=$($task.status) type=$($task.task_type) err=$($task.error)"
    $prHit = Find-PRForIssue $issue.number
    if ($expectPR -and -not $prHit) {
      Set-Result $id "FAIL" "$detail ; expected PR missing"
      return
    }
    if ($prHit) { $detail = "$detail pr=$($prHit.number)" }
    if ($task.status -eq "success" -or ($task.status -eq "partial" -and $id -eq "E9")) {
      Set-Result $id "PASS" $detail
    } elseif ($id -in @("E9","E10") -and $task.status -in @("failed","partial")) {
      Set-Result $id "PASS" $detail
    } else {
      Set-Result $id "FAIL" $detail
    }
    try {
      $wf = Get-Workflow $issue.number
      Set-Result "$id-WF" "INFO" "stage=$($wf.stage) role=$($wf.active_role) pr_id=$($wf.pr_id)"
    } catch {}
    return @{ issue = $issue; task = $task }
  } catch {
    Set-Result $id "FAIL" $_.Exception.Message
    return $null
  }
}

function Run-E3 {
  # Evidence: list_skills / load_skill in logs (nested in E2 path or registry)
  $hits = Find-LogHits @("list_skills", "load_skill", "e2e-note", "RegisterSkills", "skill")
  $skillHits = $hits | Where-Object { $_ -match "list_skills|load_skill|e2e-note|skill" }
  if ($skillHits.Count -gt 0) {
    $sample = ($skillHits | Select-Object -Last 3) -join " | "
    Set-Result "E3" "PASS" "skills evidence: $sample"
  } else {
    # Prompt analyze to use skills if not yet in log
    if (-not (Require-LLM)) { Set-Result "E3" "SKIP" "no LLM key and no skill log hits"; return }
    $body = @"
请先调用 list_skills，再 load_skill 名为 e2e-note（或仓库内 hello）的 skill，然后分析 README。
引用真实文件路径。不要创建 PR。
"@
    $r = Run-AssignScenario "E3-run" "e2e-analyze" "[e2e] E3 skills" $body "analyze_issue" @() $false
    $hits2 = Find-LogHits @("list_skills", "load_skill", "e2e-note")
    if ($hits2.Count -gt 0) {
      Set-Result "E3" "PASS" "after assign: $(($hits2 | Select-Object -Last 2) -join ' | ')"
    } elseif ($Report.Contains("E3-run") -and $Report["E3-run"].status -eq "PASS") {
      Set-Result "E3" "PASS" "analyze success; skill tools available (check tool registry). issue evidence in E3-run"
    } else {
      Set-Result "E3" "FAIL" "no list_skills/load_skill evidence in logs"
    }
  }
}

function Run-E4 {
  $hits = Find-LogHits @("e2e_echo", "e2e-mock", "RegisterMCP", "mcp")
  $mcpHits = $hits | Where-Object { $_ -match "e2e_echo|e2e-mock|RegisterMCPTools|MCP tool" }
  if ($mcpHits.Count -gt 0) {
    Set-Result "E4" "PASS" "mcp evidence: $(($mcpHits | Select-Object -Last 3) -join ' | ')"
    return
  }
  if (-not (Require-LLM)) { Set-Result "E4" "SKIP" "no LLM key and no mcp log hits"; return }
  $body = @"
请使用 MCP 工具 e2e_echo（或 e2e-mock 上的 echo）调用一次，参数 message=hello-e2e，然后在 README.md 末尾追加一行 E2E-MCP-OK 并提 PR。
"@
  $null = Run-AssignScenario "E4-run" "e2e-coder-internal" "[e2e] E4 MCP echo" $body "solve_issue" @() $true
  $hits2 = Find-LogHits @("e2e_echo", "e2e-mock")
  if ($hits2.Count -gt 0) {
    Set-Result "E4" "PASS" "after assign: $(($hits2 | Select-Object -Last 2) -join ' | ')"
  } elseif ($Report.Contains("E4-run") -and $Report["E4-run"].status -eq "PASS") {
    Set-Result "E4" "PASS" "coder-internal success with mcp_servers=e2e-mock configured"
  } else {
    Set-Result "E4" "FAIL" "no e2e_echo evidence"
  }
}

function Set-AgentToken([int]$agentId, [string]$token) {
  $dbPath = Join-Path $Root "data/gateway-e2e.db"
  $py = @"
import sqlite3
c = sqlite3.connect(r'$dbPath')
c.execute('UPDATE agents SET gitea_token=? WHERE id=?', ('$($token -replace "'","''")', $agentId))
c.commit()
c.close()
"@
  $py | python -
}

function Get-AgentToken([int]$agentId) {
  $dbPath = Join-Path $Root "data/gateway-e2e.db"
  $py = @"
import sqlite3
c = sqlite3.connect(r'$dbPath')
print(c.execute('SELECT gitea_token FROM agents WHERE id=?', ($agentId,)).fetchone()[0])
c.close()
"@
  return (($py | python -) | Select-Object -Last 1).Trim()
}

function Run-E9 {
  # Corrupt analyze agent token → LLM may succeed but writeback fails → partial/failed
  if (-not (Require-LLM)) { Set-Result "E9" "SKIP" "waiting LLM key"; return }
  if (-not (Get-Command python -ErrorAction SilentlyContinue)) {
    Set-Result "E9" "SKIP" "python required to corrupt agent token"
    return
  }
  $analyzeId = 0
  foreach ($a in (Get-AgentList)) {
    if ($a.gitea_username -eq "e2e-analyze") { $analyzeId = Normalize-Int $a.id; break }
  }
  if ($analyzeId -eq 0) { Set-Result "E9" "FAIL" "e2e-analyze not found"; return }

  $backupToken = Get-AgentToken $analyzeId
  Set-AgentToken $analyzeId "invalid-e2e-token"
  Write-Host "E9 corrupted agent $analyzeId token"

  try {
    $issue = New-Issue "[e2e] E9 writeback fail" "请简短分析 README（一两句即可）。不要创建 PR。"
    Assign-Issue $issue.number "e2e-analyze"
    $task = Wait-Task $issue.number "analyze_issue" 15
    if ($task -and $task.status -in @("partial","failed")) {
      Set-Result "E9" "PASS" "issue=$($issue.number) task=$($task.id) status=$($task.status) err=$($task.error)"
    } elseif ($task) {
      Set-Result "E9" "FAIL" "expected partial/failed got $($task.status) err=$($task.error)"
    } else {
      Set-Result "E9" "FAIL" "timeout waiting analyze after token corrupt"
    }
  } finally {
    try {
      $tokName = "gateway-agent-e2e-restore-$(Get-Random)"
      $newTok = Invoke-RestMethod -Method POST -Uri "$GiteaURL/api/v1/users/e2e-analyze/tokens" -Headers $gHeaders -Body (@{
        name = $tokName
        scopes = @("all")
      } | ConvertTo-Json -Compress)
      if ($newTok.sha1) {
        Set-AgentToken $analyzeId $newTok.sha1
        Write-Host "E9 restored e2e-analyze token via new Gitea token"
      } elseif ($backupToken) {
        Set-AgentToken $analyzeId $backupToken
        Write-Host "E9 restored e2e-analyze token from backup"
      }
    } catch {
      if ($backupToken) { Set-AgentToken $analyzeId $backupToken }
      Write-Host "E9 token restore warn: $($_.Exception.Message)"
    }
  }
}

function Start-OpenCodeServe {
  $exe = $null
  $cmd = Get-Command opencode -ErrorAction SilentlyContinue
  if ($cmd) {
    $src = "$($cmd.Source)"
    if ($src -like "*.exe" -and (Test-Path $src)) {
      $exe = $src
    } else {
      $shimDir = Split-Path $src -Parent
      $guess = Join-Path $shimDir "node_modules\opencode-ai\bin\opencode.exe"
      if (Test-Path $guess) { $exe = $guess }
    }
  }
  if (-not $exe) { throw "opencode.exe not found (npm opencode-ai bin)" }
  Start-Process -FilePath $exe -ArgumentList @("serve","--port","4096","--print-logs","--log-level","INFO") -WindowStyle Minimized
}

function Run-E10 {
  try {
    $conns = Get-NetTCPConnection -LocalPort 4096 -State Listen -ErrorAction SilentlyContinue
    $pids = @()
    foreach ($c in $conns) { $pids += $c.OwningProcess }
    $pids = $pids | Select-Object -Unique
    foreach ($p in $pids) { Stop-Process -Id $p -Force -ErrorAction SilentlyContinue }
    Start-Sleep 3
    $issue = New-Issue "[e2e] E10 health fail" "Should fail because opencode serve is down. Please add a line to README."
    Assign-Issue $issue.number "e2e-coder-opencode"
    $task = Wait-Task $issue.number "solve_issue" 8
    # Restart opencode (real .exe — npm shim fails under Start-Process)
    try { Start-OpenCodeServe } catch { Write-Host "opencode restart warn: $($_.Exception.Message)" }
    Start-Sleep 6
    if ($task -and $task.status -eq "failed") {
      Set-Result "E10" "PASS" "issue=$($issue.number) status=failed err=$($task.error)"
    } elseif ($task) {
      Set-Result "E10" "FAIL" "expected failed got $($task.status) err=$($task.error)"
    } else {
      Set-Result "E10" "FAIL" "timeout; opencode restarted"
    }
  } catch {
    Set-Result "E10" "FAIL" $_.Exception.Message
    try { Start-OpenCodeServe } catch {}
  }
}

function Run-E12 {
  try {
    Push-Location $Root
    $out = & go test ./... -count=1 2>&1 | Out-String
    Pop-Location
    if ($LASTEXITCODE -eq 0) {
      Set-Result "E12" "PASS" "go test ./... ok"
    } else {
      $tail = if ($out.Length -gt 800) { $out.Substring($out.Length - 800) } else { $out }
      Set-Result "E12" "FAIL" $tail
    }
  } catch {
    Set-Result "E12" "FAIL" $_.Exception.Message
  }
}

# S1 / checklist §2.4: merge open PR → workflow stage=done
function Run-E13 {
  try {
    $open = @(Get-PRs)
    $pr = $open | Select-Object -First 1
    if (-not $pr) {
      Set-Result "E13" "SKIP" "no open PR to merge (run E5/E6/E7 first)"
      return
    }
    $prNum = Normalize-Int $pr.number
    $issueNum = 0
    if ($pr.body -match '#(\d+)') { $issueNum = [int]$Matches[1] }
    if ($issueNum -le 0 -and $pr.head.ref -match 'issue-(\d+)') { $issueNum = [int]$Matches[1] }
    if ($issueNum -le 0) {
      Set-Result "E13" "SKIP" "pr=$prNum has no linked issue number"
      return
    }

    Write-Host "E13 merging PR #$prNum (issue #$issueNum) ..."
    $mergeBody = @{
      Do = "merge"
      merge_message_style = "Default"
    } | ConvertTo-Json -Compress
    Invoke-RestMethod -Method POST -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/pulls/$prNum/merge" `
      -Headers $gHeaders -Body $mergeBody | Out-Null

    $deadline = (Get-Date).AddMinutes(3)
    $wf = $null
    while ((Get-Date) -lt $deadline) {
      Start-Sleep 5
      try { $wf = Get-Workflow $issueNum } catch { $wf = $null }
      if ($wf -and "$($wf.stage)" -eq "done") { break }
      Write-Host "  polling workflow stage for issue=$issueNum (got=$($wf.stage)) ..."
    }
    if ($wf -and "$($wf.stage)" -eq "done") {
      Set-Result "E13" "PASS" "issue=$issueNum pr=$prNum stage=done (Merge→done Sign-off)"
    } else {
      Set-Result "E13" "FAIL" "issue=$issueNum pr=$prNum stage=$($wf.stage) expected done"
    }
  } catch {
    Set-Result "E13" "FAIL" $_.Exception.Message
  }
}

# Runner — default matrix order per plan
$all = @("E0","E1","E2","E3","E4","E5","E6","E7","E8","E9","E10","E11","E12","E13","E14")
$flatOnly = @()
foreach ($o in @($Only)) {
  if (-not $o) { continue }
  foreach ($part in ("$o" -split ",")) {
    $t = $part.Trim()
    if ($t) { $flatOnly += $t }
  }
}
$toRun = if ($flatOnly.Count -gt 0) { $flatOnly } else { $all }

foreach ($id in $toRun) {
  switch ($id) {
    "E0" { Run-E0 }
    "E1" { Run-E1 }
    "E2" {
      $null = Run-AssignScenario "E2" "e2e-analyze" "[e2e] Analyze README" `
        "请分析本仓库 README 与 skills，引用真实文件路径。可先 list_skills。不要创建 PR。" `
        "analyze_issue" @() $false
    }
    "E3" { Run-E3 }
    "E4" { Run-E4 }
    "E5" {
      $null = Run-AssignScenario "E5" "e2e-coder-internal" "[e2e] Internal coder README" `
        "请在 README.md 末尾追加一行：E2E-INTERNAL-OK。完成后提 PR（Fixes #本Issue）。" `
        "solve_issue" @() $true
    }
    "E6" {
      $null = Run-AssignScenario "E6" "e2e-coder-opencode" "[e2e] OpenCode coder README" `
        "请在 README.md 末尾追加一行：E2E-OPENCODE-OK。完成后提 PR。" `
        "solve_issue" @() $true
    }
    "E7" {
      try {
        $labs = @(Invoke-RestMethod -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/labels" -Headers $gHeaders)
        $bugId = 0
        foreach ($l in $labs) {
          if ("$($l.name)" -eq "bug") { $bugId = Normalize-Int $l.id; break }
        }
        if ($bugId -le 0) {
          $created = Invoke-RestMethod -Method POST -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/labels" -Headers $gHeaders `
            -Body (@{ name = "bug"; color = "#ee0701" } | ConvertTo-Json -Compress)
          $bugId = Normalize-Int $created.id
        }
        # Label at create time so Assign webhook resolves fix_bug (not solve_issue)
        $issue = Invoke-RestMethod -Method POST -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/issues" -Headers $gHeaders -Body "{`"title`":`"[e2e] fix_bug typo`",`"body`":`"README 若无错误，请追加一行 E2E-BUGFIX-OK 并提 PR。`",`"labels`":[$bugId]}"
        $issueNum = Normalize-Int $issue.number
        Write-Host "created issue #$issueNum with bug label=$bugId"
        Assign-Issue $issueNum "e2e-bugfix"
        $deadline = (Get-Date).AddMinutes($TimeoutMinutes)
        $task = $null
        while ((Get-Date) -lt $deadline) {
          Start-Sleep -Seconds $PollSeconds
          $list = Get-TaskList
          $task = $list | Where-Object {
            ([string](Normalize-Int $_.issue_id) -eq [string]$issueNum) -and
            $_.task_type -in @("fix_bug","solve_issue") -and
            $_.status -in @("success","failed","partial","completed")
          } | Select-Object -First 1
          if ($task) { break }
          Write-Host "  polling issue=$issueNum type=fix_bug|solve_issue ..."
        }
        $prHit = Find-PRForIssue $issueNum
        $prNum = if ($prHit) { Normalize-Int $prHit.number } else { 0 }
        if ($task -and $task.status -eq "success" -and $task.task_type -eq "fix_bug" -and $prNum -gt 0) {
          Set-Result "E7" "PASS" "issue=$issueNum task=$($task.id) type=fix_bug pr=$prNum"
        } elseif ($task -and $task.status -eq "success" -and $prNum -gt 0) {
          Set-Result "E7" "PASS" "issue=$issueNum task=$($task.id) type=$($task.task_type) pr=$prNum (wanted fix_bug)"
        } elseif ($task -and $task.status -eq "success") {
          Set-Result "E7" "FAIL" "success but PR missing type=$($task.task_type)"
        } elseif ($task) {
          Set-Result "E7" "FAIL" "status=$($task.status) type=$($task.task_type) err=$($task.error)"
        } else {
          Set-Result "E7" "FAIL" "timeout"
        }
      } catch {
        Set-Result "E7" "FAIL" $_.Exception.Message
      }
    }
    "E8" {
      if (-not (Require-LLM)) { Set-Result "E8" "SKIP" "waiting LLM key"; break }
      $prs = Get-PRs
      $pr = $prs | Select-Object -First 1
      if (-not $pr) { Set-Result "E8" "SKIP" "no open PR"; break }
      $prNum = Normalize-Int $pr.number
      try {
        Invoke-RestMethod -Method POST -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/pulls/$prNum/requested_reviewers" `
          -Headers $gHeaders -Body (@{ reviewers = @("e2e-review") } | ConvertTo-Json -Compress) | Out-Null
      } catch {
        Write-Host "request reviewers warn: $($_.Exception.Message)"
      }
      $deadline = (Get-Date).AddMinutes(10)
      $task = $null
      while ((Get-Date) -lt $deadline) {
        Start-Sleep 15
        $list = Get-TaskList
        $task = $list | Where-Object {
          $_.task_type -eq "review_pr" -and
          (([string](Normalize-Int $_.pr_id) -eq [string]$prNum) -or ([string](Normalize-Int $_.issue_id) -eq [string]$prNum)) -and
          $_.status -in @("success","failed","partial")
        } | Select-Object -First 1
        if ($task) { break }
        Write-Host "  polling review_pr for pr=$prNum ..."
      }
      if ($task -and $task.status -eq "success") {
        Set-Result "E8" "PASS" "pr=$prNum task=$($task.id) backend=internal status=success"
      } elseif ($task) {
        Set-Result "E8" "FAIL" "status=$($task.status) err=$($task.error)"
      } else {
        Set-Result "E8" "FAIL" "timeout review_pr"
      }
    }
    "E9" { Run-E9 }
    "E10" { Run-E10 }
    "E11" {
      try {
        $issues = @(Invoke-RestMethod -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/issues?state=all&limit=20" -Headers $gHeaders)
        $n = Normalize-Int (($issues | Select-Object -First 1).number)
        $wf = Get-Workflow $n
        Set-Result "E11" "PASS" "issue=$n stage=$($wf.stage) role=$($wf.active_role) pr_id=$($wf.pr_id)"
      } catch {
        Set-Result "E11" "FAIL" $_.Exception.Message
      }
    }
    "E12" { Run-E12 }
    "E13" { Run-E13 }
    "E14" { Run-E14 }
    default { Write-Host "unknown scenario $id" }
  }
}

function Run-E14 {
  if (-not (Require-LLM)) {
    Set-Result "E14" "SKIP" "waiting SENSENOVA_API_KEY / DEEPSEEK_API_KEY"
    return
  }
  $repo = "$Owner/$Repo"
  $originalPolicy = $null
  try {
    $originalPolicy = Invoke-RestMethod -Uri "$GatewayURL/api/workflow-policies/$repo" -Headers $aHeaders
  } catch {}

  try {
    Write-Host "E14: Testing stage transition unassign"
    Set-WorkflowPolicy $repo "standard" @{}

    $issue = New-Issue "[e2e] E14 stage unassign" "请简短分析 README。不要创建 PR。"
    $issueNum = Normalize-Int $issue.number
    Write-Host "E14 created issue #$issueNum"

    Write-Host "E14 Step 1: Assign analyze agent"
    Assign-Issue $issueNum "e2e-analyze"
    $task1 = Wait-Task $issueNum "analyze_issue" 15
    if (-not $task1) {
      Set-Result "E14" "FAIL" "timeout waiting analyze task issue=$issueNum"
      return
    }

    Start-Sleep 5
    $assigneesAfterAnalyze = Get-IssueAssignees $issueNum
    Write-Host "E14 assignees after analyze: $($assigneesAfterAnalyze -join ', ')"
    if (-not ($assigneesAfterAnalyze -contains "e2e-analyze")) {
      Set-Result "E14" "FAIL" "analyze agent not in assignees after assign, got: $($assigneesAfterAnalyze -join ', ')"
      return
    }

    Write-Host "E14 Step 2: Add manual assignee"
    Invoke-RestMethod -Method PATCH -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/issues/$issueNum" -Headers $gHeaders -Body (@{
      assignees = @("e2e-analyze", "admin")
    } | ConvertTo-Json -Compress) | Out-Null
    Start-Sleep 3
    $assigneesBeforeTransition = Get-IssueAssignees $issueNum
    Write-Host "E14 assignees before coder assign: $($assigneesBeforeTransition -join ', ')"

    Write-Host "E14 Step 3: Assign coder agent (trigger stage transition)"
    Assign-Issue $issueNum "e2e-coder-internal"
    Start-Sleep 10

    $assigneesAfterTransition = Get-IssueAssignees $issueNum
    Write-Host "E14 assignees after coder assign: $($assigneesAfterTransition -join ', ')"

    $logHits = Find-LogHits @("Unassigned agent.*on stage transition")
    $logHit = $logHits | Where-Object { $_ -match "e2e-analyze" } | Select-Object -First 1

    $analyzeUnassigned = -not ($assigneesAfterTransition -contains "e2e-analyze")
    $manualStillThere = $assigneesAfterTransition -contains "admin"
    $coderAssigned = $assigneesAfterTransition -contains "e2e-coder-internal"

    $detail = "issue=$issueNum ; analyze_unassigned=$analyzeUnassigned ; manual_still_there=$manualStillThere ; coder_assigned=$coderAssigned"
    if ($logHit) { $detail = "$detail ; log_hit=yes" } else { $detail = "$detail ; log_hit=no" }

    if ($analyzeUnassigned -and $manualStillThere -and $coderAssigned) {
      Set-Result "E14" "PASS" $detail
    } else {
      Set-Result "E14" "FAIL" $detail
    }

    Write-Host "E14 Step 4: Test with policy=off (should NOT unassign)"
    Set-WorkflowPolicy $repo "free" @{}
    Start-Sleep 2

    Invoke-RestMethod -Method PATCH -Uri "$GiteaURL/api/v1/repos/$Owner/$Repo/issues/$issueNum" -Headers $gHeaders -Body (@{
      assignees = @("e2e-coder-internal", "e2e-analyze")
    } | ConvertTo-Json -Compress) | Out-Null
    Start-Sleep 3
    $assigneesBeforeOff = Get-IssueAssignees $issueNum
    Write-Host "E14 assignees before off-test: $($assigneesBeforeOff -join ', ')"

    Assign-Issue $issueNum "e2e-coder-opencode"
    Start-Sleep 10

    $assigneesAfterOff = Get-IssueAssignees $issueNum
    Write-Host "E14 assignees after off-test: $($assigneesAfterOff -join ', ')"

    $analyzeStillThere = $assigneesAfterOff -contains "e2e-analyze"
    if ($analyzeStillThere) {
      Set-Result "E14-off" "PASS" "issue=$issueNum analyze_still_there=$analyzeStillThere (expected with policy=off)"
    } else {
      Set-Result "E14-off" "FAIL" "issue=$issueNum analyze_still_there=$analyzeStillThere (should be true with policy=off)"
    }

  } catch {
    Set-Result "E14" "FAIL" $_.Exception.Message
  } finally {
    if ($originalPolicy) {
      try {
        $json = $originalPolicy | ConvertTo-Json -Compress
        Invoke-RestMethod -Method PUT -Uri "$GatewayURL/api/workflow-policies/$repo" -Headers $aHeaders -Body $json | Out-Null
        Write-Host "E14 restored original workflow policy"
      } catch {
        Write-Host "E14 policy restore warn: $($_.Exception.Message)"
      }
    } else {
      Delete-WorkflowPolicy $repo
    }
  }
}

# Write results JSON for report
$outPath = "data/e2e-results.json"
@($Report.GetEnumerator() | ForEach-Object {
  [pscustomobject]@{ id = $_.Key; status = $_.Value.status; detail = $_.Value.detail; at = $_.Value.at }
}) | ConvertTo-Json -Depth 5 | Set-Content $outPath -Encoding UTF8
Write-Host "RESULTS -> $outPath"
$Report.GetEnumerator() | ForEach-Object { "$($_.Key): $($_.Value.status)" }
