# End-to-end smoke test for the Estimeet API.
# Usage:  pwsh ./scripts/smoke.ps1  (the backend must already be running)

$ErrorActionPreference = 'Stop'
$base = $env:ESTIMEET_API_BASE
if (-not $base) { $base = 'http://127.0.0.1:8090/api' }

$pass = 0
$fail = 0

function Check($name, $condition) {
    if ($condition) {
        Write-Host "  PASS  $name" -ForegroundColor Green
        $script:pass++
    } else {
        Write-Host "  FAIL  $name" -ForegroundColor Red
        $script:fail++
    }
}

function Call($method, $path, $token, $body) {
    $headers = @{ Accept = 'application/json' }
    if ($token) { $headers['Authorization'] = "Bearer $token" }
    $params = @{ Method = $method; Uri = "$base$path"; Headers = $headers }
    if ($null -ne $body) {
        $params['Body'] = ($body | ConvertTo-Json -Depth 6)
        $params['ContentType'] = 'application/json'
    }
    Invoke-RestMethod @params
}

# Returns the HTTP status code of a call that is expected to fail.
function StatusOf($method, $path, $token, $body) {
    try {
        Call $method $path $token $body | Out-Null
        return 200
    } catch {
        return [int]$_.Exception.Response.StatusCode
    }
}

Write-Host "`nEstimeet smoke test against $base`n" -ForegroundColor Cyan

# --- health ------------------------------------------------------------------
$health = Call GET '/health' $null $null
Check 'health endpoint responds ok' ($health.status -eq 'ok')

# --- synchronous room --------------------------------------------------------
Write-Host "`nSynchronous mode" -ForegroundColor Yellow
$sync = Call POST '/rooms' $null @{ name = 'Smoke sync'; mode = 'sync'; hostName = 'Ada'; autoReveal = $true }
$syncCode = $sync.roomCode
$hostToken = $sync.token
Check 'room created with a 6 character code' ($syncCode.Length -eq 6)
Check 'host is flagged as host' ($sync.participant.isHost -eq $true)

$player = Call POST "/rooms/$syncCode/join" $null @{ name = 'Linus'; asObserver = $false }
$playerToken = $player.token
Check 'second player joined' ($player.participant.isHost -eq $false)

Check 'duplicate display name is rejected' `
    ((StatusOf POST "/rooms/$syncCode/join" $null @{ name = 'linus'; asObserver = $false }) -eq 409)

$state = Call POST "/rooms/$syncCode/topics" $hostToken @{
    topics = @(
        @{ title = 'Checkout redesign'; description = 'Cart plus payment step' },
        @{ title = 'Rate limiting'; description = '' }
    )
}
Check 'two topics added' ($state.topics.Count -eq 2)
Check 'sync room auto-focuses the first topic' ($state.room.currentTopicId -eq $state.topics[0].id)

$first = $state.topics[0].id
$second = $state.topics[1].id

Check 'non-host cannot add topics' `
    ((StatusOf POST "/rooms/$syncCode/topics" $playerToken @{ topics = @(@{ title = 'Sneaky'; description = '' }) }) -eq 403)

Check 'a card outside the deck is rejected' `
    ((StatusOf POST "/rooms/$syncCode/topics/$first/vote" $playerToken @{ value = '7' }) -eq 400)

Check 'voting on a non-current topic is refused in sync mode' `
    ((StatusOf POST "/rooms/$syncCode/topics/$second/vote" $playerToken @{ value = '5' }) -eq 403)

$state = Call POST "/rooms/$syncCode/topics/$first/vote" $playerToken @{ value = '5' }
$topic = $state.topics | Where-Object { $_.id -eq $first }
Check 'vote recorded on the current topic' ($topic.votedBy.Count -eq 1)
Check 'card values stay hidden before the reveal' ($topic.votes.Count -eq 0)

$state = Call POST "/rooms/$syncCode/topics/$first/vote" $hostToken @{ value = '13' }
$state = Call POST "/rooms/$syncCode/topics/$first/reveal" $hostToken $null
$topic = $state.topics | Where-Object { $_.id -eq $first }
Check 'reveal exposes both cards' ($topic.votes.Count -eq 2)
Check 'statistics are computed' ($topic.stats.average -eq 9)
Check 'suggestion falls on a Fibonacci card' ($topic.stats.suggested -eq '8')

Check 'a non-numeric final estimate is rejected' `
    ((StatusOf POST "/rooms/$syncCode/topics/$first/estimate" $hostToken @{ value = '?' }) -eq 400)

$state = Call POST "/rooms/$syncCode/topics/$first/estimate" $hostToken @{ value = '8' }
$topic = $state.topics | Where-Object { $_.id -eq $first }
Check 'topic is estimated' ($topic.status -eq 'estimated' -and $topic.finalEstimate -eq '8')
Check 'total points are summed' ($state.summary.totalPoints -eq 8)

$state = Call POST "/rooms/$syncCode/current" $hostToken @{ direction = 'next' }
Check 'host can advance to the next topic' ($state.room.currentTopicId -eq $second)

# --- asynchronous room -------------------------------------------------------
Write-Host "`nAsynchronous mode" -ForegroundColor Yellow
$async = Call POST '/rooms' $null @{ name = 'Smoke async'; mode = 'async'; hostName = 'Grace'; autoReveal = $true }
$asyncCode = $async.roomCode
$asyncHost = $async.token
$asyncPlayer = (Call POST "/rooms/$asyncCode/join" $null @{ name = 'Margaret'; asObserver = $false }).token
$observer = (Call POST "/rooms/$asyncCode/join" $null @{ name = 'Watcher'; asObserver = $true }).token

$state = Call POST "/rooms/$asyncCode/topics" $asyncHost @{
    topics = @(@{ title = 'Audit log'; description = '' }, @{ title = 'SSO'; description = '' })
}
$a1 = $state.topics[0].id
$a2 = $state.topics[1].id
Check 'async room has no current topic' ($null -eq $state.room.currentTopicId)

Check 'setting a current topic is refused in async mode' `
    ((StatusOf POST "/rooms/$asyncCode/current" $asyncHost @{ topicId = $a1 }) -eq 409)

Check 'observers cannot vote' `
    ((StatusOf POST "/rooms/$asyncCode/topics/$a1/vote" $observer @{ value = '3' }) -eq 403)

$state = Call POST "/rooms/$asyncCode/topics/$a2/vote" $asyncPlayer @{ value = '21' }
Check 'any topic is votable in async mode' (($state.topics | Where-Object { $_.id -eq $a2 }).myVote -eq '21')

$state = Call POST "/rooms/$asyncCode/topics/$a1/vote" $asyncPlayer @{ value = '3' }
$topic = $state.topics | Where-Object { $_.id -eq $a1 }
Check 'no reveal while a teammate still owes a card' ($topic.revealed -eq $false)

$state = Call POST "/rooms/$asyncCode/topics/$a1/vote" $asyncHost @{ value = '5' }
$topic = $state.topics | Where-Object { $_.id -eq $a1 }
Check 'auto-reveal once every member voted (observer excluded)' ($topic.revealed -eq $true)
Check 'auto-revealed topic exposes both cards' ($topic.votes.Count -eq 2)

$state = Call POST "/rooms/$asyncCode/topics/$a1/reset" $asyncHost $null
$topic = $state.topics | Where-Object { $_.id -eq $a1 }
Check 'reset clears the round' ($topic.status -eq 'pending' -and $topic.votedBy.Count -eq 0)

# --- isolation ---------------------------------------------------------------
Write-Host "`nIsolation" -ForegroundColor Yellow
Check 'a token from another room is rejected' `
    ((StatusOf GET "/rooms/$asyncCode/state" $hostToken $null) -eq 403)
Check 'an unknown token is rejected' `
    ((StatusOf GET "/rooms/$syncCode/state" 'bogus-token' $null) -eq 403)
Check 'an unknown room returns 404' `
    ((StatusOf GET '/rooms/ZZZZZZ' $null $null) -eq 404)

Write-Host "`n$pass passed, $fail failed`n" -ForegroundColor $(if ($fail -eq 0) { 'Green' } else { 'Red' })
if ($fail -gt 0) { exit 1 }
