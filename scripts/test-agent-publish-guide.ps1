param(
    [Parameter(Mandatory)][string]$BaseUrl,
    [Parameter(Mandatory)][string]$Username,
    [Parameter(Mandatory)][string]$PasswordFile,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$Session = 'skillhub-agent-cli'
)
$ErrorActionPreference = 'Stop'
$env:NO_PROXY = '127.0.0.1,localhost'
$env:AGENT_BROWSER_HEADED = 'false'
$password = [Net.NetworkCredential]::new('', (Import-Clixml -LiteralPath $PasswordFile)).Password
New-Item -ItemType Directory -Path $OutputDirectory -Force | Out-Null
function Browser {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]]$Arguments)
    if ($Arguments[0] -eq 'click') { & agent-browser --session $Session scrollintoview $Arguments[1] | Out-Null }
    $output = & agent-browser --session $Session @Arguments 2>&1
    if ($LASTEXITCODE -ne 0) { throw ($output -join "`n") }
    $output
}
function Assert-Page([string]$Expression) {
    Browser eval "if (!($Expression)) throw new Error('guide assertion failed'); true" | Out-Null
}
Browser skills get agent-browser | Out-Null
Browser cookies clear | Out-Null
Browser open "$BaseUrl/publish?type=agent" | Out-Null
Browser wait 'input[autocomplete="username"]' | Out-Null
Browser fill 'input[autocomplete="username"]' $Username | Out-Null
Browser fill 'input[type="password"]' $password | Out-Null
Browser click 'button[type="submit"]' | Out-Null
Browser wait '#agent-slug' | Out-Null
$languageButton = 'button[title="Switch language"],button[title="切换语言"]'
if ((Browser get text $languageButton) -match '中') { Browser click $languageButton | Out-Null }
Browser fill '#agent-slug' 'retained-suite' | Out-Null
Browser fill '#agent-displayName' 'Retained Suite' | Out-Null
$fixture = Join-Path $OutputDirectory 'selection.zip'
[IO.File]::WriteAllBytes($fixture, [byte[]](1, 2, 3))
Browser upload '#agent-upload' $fixture | Out-Null
Browser click '.agent-publish .tab-btn:nth-child(2)' | Out-Null
Assert-Page 'document.querySelector(".agent-publish-guide").innerText.includes("API Token") && document.querySelector(".agent-publish-guide").innerText.includes("--type agent") && document.querySelector(".agent-publish-guide").innerText.includes("paths=<./paths.json")'
Assert-Page 'document.querySelector("#agent-slug").closest("div[hidden]") !== null && document.querySelector("#agent-upload").files.length === 1'
Browser screenshot (Join-Path $OutputDirectory 'guide-zh.png') --full | Out-Null
Browser click '.agent-publish-guide section:first-child .code-block:first-of-type .copy-btn' | Out-Null
Assert-Page 'document.querySelector(".agent-publish-guide .copy-btn").innerText.includes("已复制")'
Browser click '.agent-publish .tab-btn:nth-child(1)' | Out-Null
Assert-Page 'document.querySelector("#agent-slug").value === "retained-suite" && document.querySelector("#agent-displayName").value === "Retained Suite" && document.querySelector("#agent-upload").files.length === 1'
Browser click '.agent-publish .tab-btn:nth-child(2)' | Out-Null
Browser click 'button[title="切换语言"]' | Out-Null
Assert-Page 'document.querySelector(".agent-publish-guide").innerText.includes("Publish a directory") && !document.querySelector(".agent-publish-guide").innerText.includes("agentPublishGuide.")'
Browser set viewport 390 844 | Out-Null
Assert-Page 'document.documentElement.scrollWidth <= innerWidth + 1'
Browser screenshot (Join-Path $OutputDirectory 'guide-mobile-en.png') --full | Out-Null
Browser set viewport 1280 900 | Out-Null
Browser open "$BaseUrl/publish?type=skill" | Out-Null
Assert-Page '!document.querySelector("#agent-slug") && document.querySelectorAll(".tab-btn").length === 2'
Browser open "$BaseUrl/publish?type=agent&slug=cli-suite" | Out-Null
Browser wait '#agent-slug' | Out-Null
Assert-Page 'document.querySelector("#agent-slug").readOnly && document.querySelector("#agent-slug").value === "cli-suite"'
Browser click '.agent-publish .tab-btn:nth-child(2)' | Out-Null
Assert-Page 'document.querySelector(".agent-publish-guide") !== null'
$errors = Browser errors
if ($errors -and ($errors -join '').Trim()) { throw ($errors -join "`n") }
Write-Output 'PASS: guide CLI/API content, copy, form/file retention, English, mobile overflow, Skill entry, existing-version guide, no browser errors'
