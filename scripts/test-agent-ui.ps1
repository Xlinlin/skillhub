param(
    [string]$BaseUrl = 'http://localhost:10071',
    [string]$Username = $env:SKILLHUB_TEST_USER,
    [string]$Password = $env:SKILLHUB_TEST_PASSWORD,
    [string]$PasswordFile = '',
    [string]$OutputDirectory = '',
    [string]$Session = 'skillhub-agents'
)
$ErrorActionPreference = 'Stop'
if ($PasswordFile) {
    $secure = Import-Clixml -LiteralPath $PasswordFile
    $Password = [System.Net.NetworkCredential]::new('', $secure).Password
}
if (!$Username -or !$Password) { throw 'Set test credentials through environment variables or an encrypted PasswordFile.' }
if (!$OutputDirectory) { $OutputDirectory = Join-Path ([IO.Path]::GetTempPath()) ('skillhub-agent-ui-' + [Guid]::NewGuid().ToString('N')) }
$OutputDirectory = [IO.Path]::GetFullPath($OutputDirectory)
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
$env:NO_PROXY = '127.0.0.1,localhost'
$env:AGENT_BROWSER_HEADED = 'false'
function Browser {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$BrowserArgs)
    Write-Host "Browser: $($BrowserArgs[0])"
    if ($IsWindows -and $BrowserArgs[0] -eq 'download') {
        $cdp = (& agent-browser --session $Session get cdp-url).Trim()
        if ($LASTEXITCODE -ne 0) { throw 'Unable to discover test browser' }
        $nodeExecutable = (Get-Command node.exe -CommandType Application | Select-Object -First 1).Source
        & $nodeExecutable (Join-Path $PSScriptRoot 'agent-ui-download.mjs') $cdp $BaseUrl $BrowserArgs[1] $BrowserArgs[2]
        if ($LASTEXITCODE -ne 0) { throw 'Browser download failed' }
        return
    }
    if ($BrowserArgs[0] -in @('click', 'download', 'check', 'select')) {
        & agent-browser --session $Session scrollintoview $BrowserArgs[1] | Out-Null
        if ($LASTEXITCODE -ne 0) { throw 'Unable to bring the control into view' }
    }
    $result = & agent-browser --session $Session @BrowserArgs 2>&1
    if ($LASTEXITCODE -ne 0) { throw ($result -join "`n") }
    $result | Write-Output
}
function Assert-Page([string]$Expression) {
    Browser eval "if (!($Expression)) throw new Error('UI assertion failed'); true" | Out-Null
}
function Write-Fixture([string]$Root, [string]$Relative, [byte[]]$Bytes) {
    $target = Join-Path $Root $Relative
    New-Item -ItemType Directory -Path ([IO.Path]::GetDirectoryName($target)) -Force | Out-Null
    [IO.File]::WriteAllBytes($target, $Bytes)
}
function Make-Zip([string]$Root, [string]$Target) {
    $archive = [IO.Compression.ZipFile]::Open($Target, [IO.Compression.ZipArchiveMode]::Create)
    try {
        foreach ($file in [IO.Directory]::EnumerateFiles($Root, '*', [IO.SearchOption]::AllDirectories)) {
            $relative = [IO.Path]::GetRelativePath($Root, $file).Replace('\', '/')
            [IO.Compression.ZipFileExtensions]::CreateEntryFromFile($archive, $file, $relative) | Out-Null
        }
    } finally { $archive.Dispose() }
}
$source = Join-Path $OutputDirectory 'suite-source'
$utf8 = [Text.UTF8Encoding]::new($false)
Write-Fixture $source '.agents/pm.md' ($utf8.GetBytes("# Product agent`nVersion one`n"))
Write-Fixture $source '.claude/agents/dev.md' ($utf8.GetBytes("# Development agent`n"))
Write-Fixture $source 'design/需求.md' ($utf8.GetBytes("需求 → 设计 → 开发 → 测试`n"))
Write-Fixture $source 'test/binary.bin' ([byte[]](0, 1, 255))
Write-Fixture $source 'test/empty.txt' ([byte[]]@())
$zip1 = Join-Path $OutputDirectory 'suite-v1.zip'
Make-Zip $source $zip1
Write-Fixture $source '.agents/pm.md' ($utf8.GetBytes("# Product agent`nVersion two`n"))
Write-Fixture $source 'test/new.md' ($utf8.GetBytes("# Test agent`n"))
$zip2 = Join-Path $OutputDirectory 'suite-v2.zip'
Make-Zip $source $zip2
$slug = 'ui-suite-' + [Guid]::NewGuid().ToString('N').Substring(0, 8)
$checks = [Collections.Generic.List[string]]::new()
Browser skills get agent-browser | Out-Null
Browser cookies clear | Out-Null
Browser open "$BaseUrl/agents" | Out-Null
Browser wait 500 | Out-Null
Assert-Page '[...document.querySelectorAll("nav a")].some(a => a.pathname === "/agents")'
Browser click 'a[href="/publish?type=agent"]' | Out-Null
Browser wait 'input[autocomplete="username"]' | Out-Null
Browser fill 'input[autocomplete="username"]' $Username | Out-Null
Browser fill 'input[autocomplete="current-password"]' $Password | Out-Null
Browser click 'button[type="submit"]' | Out-Null
Browser wait '#agent-slug' | Out-Null
$checks.Add('E01: visible menu, publish entry, actual form login and redirect')
Browser fill '#agent-slug' $slug | Out-Null
Browser fill '#agent-displayName' '产品研发协作套件' | Out-Null
Browser fill '#agent-version' '1.0.0' | Out-Null
Browser fill '#agent-summary' '产品、设计、开发、测试 Agent 协作。' | Out-Null
Browser fill '#agent-instructions' '先运行产品 Agent，再依次运行设计、开发与测试 Agent。' | Out-Null
Browser upload '#agent-upload' $zip1 | Out-Null
Browser click 'button[type="submit"]' | Out-Null
Browser wait 'a[data-testid="agent-download"]' | Out-Null
Browser download 'a[data-testid="agent-download"]' (Join-Path $OutputDirectory 'download-v1.zip') | Out-Null
if ((Get-FileHash $zip1).Hash -ne (Get-FileHash (Join-Path $OutputDirectory 'download-v1.zip')).Hash) { throw 'ZIP v1 bytes changed' }
Browser click '.agent-file-list li:first-child button' | Out-Null
Browser wait '[data-testid="agent-preview"]' | Out-Null
Assert-Page 'document.querySelector("[data-testid=agent-preview]").textContent.includes("Version one")'
Browser screenshot (Join-Path $OutputDirectory 'detail-v1.png') | Out-Null
$checks.Add('E02: ZIP form publish, hidden file preview, clicked download SHA256 equality')
Browser click 'a[href*="/publish?type=agent&slug="]' | Out-Null
Browser wait '#agent-version' | Out-Null
Assert-Page 'document.querySelector("#agent-slug").readOnly'
Browser fill '#agent-version' '1.1.0' | Out-Null
Browser upload '#agent-upload' $zip2 | Out-Null
Browser click 'button[type="submit"]' | Out-Null
Browser wait 'a[data-testid="agent-download"]' | Out-Null
Assert-Page 'document.querySelector("[data-testid=agent-download]").href.includes("1.1.0")'
Browser download 'a[data-testid="agent-download"]' (Join-Path $OutputDirectory 'download-v2.zip') | Out-Null
if ((Get-FileHash $zip2).Hash -ne (Get-FileHash (Join-Path $OutputDirectory 'download-v2.zip')).Hash) { throw 'ZIP v2 bytes changed' }
Browser wait 'aside select' | Out-Null
Browser wait 300 | Out-Null
Browser select 'aside select' '1.0.0' | Out-Null
Browser wait 500 | Out-Null
Assert-Page 'document.querySelector("[data-testid=agent-download]").href.includes("1.0.0") && !document.querySelector(".agent-file-list").textContent.includes("new.md")'
Browser download 'a[data-testid="agent-download"]' (Join-Path $OutputDirectory 'download-old.zip') | Out-Null
if ((Get-FileHash $zip1).Hash -ne (Get-FileHash (Join-Path $OutputDirectory 'download-old.zip')).Hash) { throw 'Old version changed' }
$checks.Add('E04: new version, immutable old snapshot, selected-version download')
Browser open "$BaseUrl/agents" | Out-Null
Browser wait 500 | Out-Null
Assert-Page "document.querySelector('main').textContent.includes('$slug')"
Browser screenshot (Join-Path $OutputDirectory 'list.png') | Out-Null
Browser click 'a[href="/publish?type=agent"]' | Out-Null
Browser wait '#agent-slug' | Out-Null
Browser fill '#agent-slug' ($slug + '-files') | Out-Null
Browser fill '#agent-displayName' 'Single agent file upload' | Out-Null
Browser select '#agent-kind' 'single' | Out-Null
Browser fill '#agent-version' '1.0.0' | Out-Null
Browser check 'input[name="uploadMode"][value="files"]' | Out-Null
Browser upload '#agent-upload' (Join-Path $source '.agents/pm.md') | Out-Null
Browser click 'button[type="submit"]' | Out-Null
Browser wait 'a[data-testid="agent-download"]' | Out-Null
Browser download 'a[data-testid="agent-download"]' (Join-Path $OutputDirectory 'download-files.zip') | Out-Null
$downloaded = [IO.Compression.ZipFile]::OpenRead((Join-Path $OutputDirectory 'download-files.zip'))
try { if ($downloaded.Entries.Count -ne 1 -or $downloaded.Entries[0].FullName -ne 'pm.md') { throw 'Files upload manifest changed' } } finally { $downloaded.Dispose() }
$checks.Add('E03 files: single-agent upload through real file picker')
Browser open "$BaseUrl/agents/publish" | Out-Null
Browser wait '#agent-slug' | Out-Null
Browser fill '#agent-slug' ($slug + '-folder') | Out-Null
Browser fill '#agent-displayName' 'Folder upload suite' | Out-Null
Browser fill '#agent-version' '1.0.0' | Out-Null
Browser check 'input[name="uploadMode"][value="folder"]' | Out-Null
Browser upload '#agent-upload' $source | Out-Null
Browser click 'button[type="submit"]' | Out-Null
Browser wait 'a[data-testid="agent-download"]' | Out-Null
Browser download 'a[data-testid="agent-download"]' (Join-Path $OutputDirectory 'download-folder.zip') | Out-Null
$downloaded = [IO.Compression.ZipFile]::OpenRead((Join-Path $OutputDirectory 'download-folder.zip'))
try {
    $actual = @($downloaded.Entries | ForEach-Object { $_.FullName })
    foreach ($file in [IO.Directory]::EnumerateFiles($source, '*', [IO.SearchOption]::AllDirectories)) {
        $relative = [IO.Path]::GetRelativePath($source, $file).Replace('\', '/')
        if ($actual -notcontains $relative) { throw "Folder lost path $relative" }
        $entry = $downloaded.GetEntry($relative)
        $stream = $entry.Open()
        try { $hash = [Convert]::ToHexString([Security.Cryptography.SHA256]::HashData($stream)) } finally { $stream.Dispose() }
        if ($hash -ne (Get-FileHash $file).Hash) { throw "Folder changed bytes $relative" }
    }
} finally { $downloaded.Dispose() }
$checks.Add('E03 folder: native directory picker, hidden paths and every file SHA256 preserved')
Browser open "$BaseUrl/agents/publish" | Out-Null
Browser wait '#agent-slug' | Out-Null
Browser fill '#agent-slug' ($slug + '-invalid') | Out-Null
Browser fill '#agent-displayName' 'Rejected ZIP test' | Out-Null
Browser fill '#agent-version' '1.0.0' | Out-Null
$badZip = Join-Path $OutputDirectory 'invalid.zip'
[IO.File]::WriteAllText($badZip, 'not a ZIP')
Browser upload '#agent-upload' $badZip | Out-Null
Browser click 'button[type="submit"]' | Out-Null
Browser wait '[role="alert"]' | Out-Null
Assert-Page 'location.pathname === "/publish" && new URLSearchParams(location.search).get("type") === "agent" && document.querySelector("#agent-version").value === "1.0.0" && !document.querySelector("fieldset").disabled'
Browser check 'input[name="uploadMode"][value="files"]' | Out-Null
Assert-Page '!document.querySelector(".agent-selection") && document.querySelector("#agent-upload").files.length === 0'
$checks.Add('E07: rejected ZIP retains inputs, switching modes clears stale files')
Browser open "$BaseUrl/agents" | Out-Null
Browser wait '.agent-filters' | Out-Null
Browser fill '.agent-filters input' 'zzzxqnomatch-unique' | Out-Null
Browser click '.agent-filters button' | Out-Null
Browser wait 500 | Out-Null
Assert-Page 'document.querySelectorAll(".agent-card").length === 0 && !!document.querySelector(".agent-empty")'
Browser fill '.agent-filters input' $slug | Out-Null
Browser click '.agent-filters button' | Out-Null
Browser wait 500 | Out-Null
Browser select '.agent-filters select' 'single' | Out-Null
Browser wait 500 | Out-Null
Assert-Page 'document.querySelectorAll(".agent-card").length === 1 && document.querySelector(".agent-card").textContent.includes("Single agent")'
$checks.Add('E05: no-result transition, keyword recovery, type filtering')
Browser click '.nav button.icon-btn:nth-last-of-type(2)' | Out-Null
Assert-Page 'document.querySelector("main").textContent.includes("Publish Agent")'
Browser click '.nav button.icon-btn:nth-last-of-type(2)' | Out-Null
Browser eval 'window.__themeBefore = document.documentElement.dataset.theme' | Out-Null
Browser click '.nav button.icon-btn:last-of-type' | Out-Null
Assert-Page 'document.documentElement.dataset.theme !== window.__themeBefore'
Browser set viewport 390 844 | Out-Null
Assert-Page 'document.documentElement.scrollWidth <= window.innerWidth'
Browser screenshot (Join-Path $OutputDirectory 'mobile.png') | Out-Null
Browser set viewport 1280 900 | Out-Null
$checks.Add('E05 visual: language, theme, 390px viewport without horizontal overflow')
Browser click '.agent-card' | Out-Null
Browser wait 'a[data-testid="agent-download"]' | Out-Null
Browser click '.agent-actions button' | Out-Null
Browser wait '.error-box .agent-actions' | Out-Null
Browser click '.error-box .agent-actions button:last-child' | Out-Null
Browser wait '.agent-filters' | Out-Null
Browser open "$BaseUrl/agents/$slug-files" | Out-Null
Browser wait '[role="alert"]' | Out-Null
Assert-Page '!document.querySelector("[data-testid=agent-download]")'
$checks.Add('E06: delete confirmation, post-delete deep link has no download')
Browser click '.nav button.btn' | Out-Null
Browser open "$BaseUrl/agents/publish?slug=$slug" | Out-Null
Browser wait 'input[autocomplete="username"]' | Out-Null
Assert-Page 'location.pathname === "/login" && location.search.includes("redirect=")'
$checks.Add('E08: logout and protected publish deep-link redirect')
$browserErrors = @(Browser errors)
[IO.File]::WriteAllText((Join-Path $OutputDirectory 'browser-errors.txt'), ($browserErrors -join "`n"))
[pscustomobject]@{ baseUrl=$BaseUrl; slug=$slug; checks=@($checks); zip1SHA256=(Get-FileHash $zip1).Hash; zip2SHA256=(Get-FileHash $zip2).Hash; output=$OutputDirectory; completedAt=(Get-Date).ToString('o') } | ConvertTo-Json -Depth 5 | Set-Content -LiteralPath (Join-Path $OutputDirectory 'result.json')
$checks | Write-Output
Write-Output "Artifacts: $OutputDirectory"
