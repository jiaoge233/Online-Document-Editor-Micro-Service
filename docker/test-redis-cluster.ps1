# Redis cluster simulation script (PowerShell)
param(
    [int]$Scenario = 0,
    [int]$BenchmarkRequests = 100000,
    [int]$BenchmarkClients = 100,
    [int]$BenchmarkPipeline = 16,
    [int]$BenchmarkSeconds = 0
)

$REDIS_PASS = "123456"
$NODES = @(
    "redis-node-1",
    "redis-node-2",
    "redis-node-3",
    "redis-node-4",
    "redis-node-5",
    "redis-node-6"
)

function Invoke-RedisCommand {
    param([string]$Node, [string]$Command)
    try {
        $result = & docker exec -it $Node redis-cli -a $REDIS_PASS $Command.Split(" ")
        return $result
    }
    catch {
        return "Error: $_"
    }
}

function Simulate-MasterFailure {
    Write-Host "=== Scenario 1: Simulate master failure ===" -ForegroundColor Yellow

    Write-Host "Current masters:" -ForegroundColor Green
    docker exec -it redis-node-1 redis-cli -a $REDIS_PASS cluster nodes | Select-String "master"

    Write-Host "`nStopping master redis-node-1..." -ForegroundColor Red
    docker stop redis-node-1

    Write-Host "Waiting for failover..." -ForegroundColor Yellow
    Start-Sleep -Seconds 10

    Write-Host "Cluster masters after failover:" -ForegroundColor Green
    docker exec -it redis-node-2 redis-cli -a $REDIS_PASS cluster nodes | Select-String "master"

    Write-Host "`nStarting redis-node-1..." -ForegroundColor Green
    docker start redis-node-1

    Write-Host "Waiting for rejoin..." -ForegroundColor Yellow
    Start-Sleep -Seconds 15
    docker exec -it redis-node-1 redis-cli -a $REDIS_PASS cluster nodes
}

function Simulate-HighLoad {
    Write-Host "=== Scenario 2: Simulate high QPS (monitor only) ===" -ForegroundColor Yellow
    Write-Host "Monitoring instantaneous_ops_per_sec. Press Ctrl+C to stop." -ForegroundColor Green

    try {
        while ($true) {
            Clear-Host
            Write-Host "=== Cluster QPS Monitor ===" -ForegroundColor Cyan
            Write-Host "Time: $(Get-Date)"

            foreach ($node in $NODES) {
                Write-Host "Node $node " -ForegroundColor White
                try {
                    $info = & docker exec -it $node redis-cli -a $REDIS_PASS info stats 2>$null
                    if ($LASTEXITCODE -eq 0) {
                        $ops = $info | Select-String "instantaneous_ops_per_sec"
                        Write-Host "  $ops" -ForegroundColor Green
                    } else {
                        Write-Host "  connection failed" -ForegroundColor Red
                    }
                }
                catch {
                    Write-Host "  connection failed" -ForegroundColor Red
                }
            }
            Start-Sleep -Seconds 5
        }
    }
    catch {
        Write-Host "Monitor stopped." -ForegroundColor Yellow
    }
}

function Simulate-SlowQueries {
    Write-Host "=== Scenario 3: Simulate slow queries ===" -ForegroundColor Yellow

    $SLOW_SCRIPT = @'
local start = redis.call("TIME")[1]
while redis.call("TIME")[1] - start < 5 do
    -- busy loop 5 seconds
end
return "slow operation completed"
'@

    Write-Host "Running 5s Lua script..." -ForegroundColor Green
    docker exec -it redis-node-1 redis-cli -a $REDIS_PASS --eval $SLOW_SCRIPT

    Write-Host "`nSlowlog last 5:" -ForegroundColor Green
    docker exec -it redis-node-1 redis-cli -a $REDIS_PASS slowlog get 5
}

function Monitor-Cluster {
    Write-Host "=== Scenario 4: Cluster health monitor ===" -ForegroundColor Yellow
    Write-Host "Press Ctrl+C to stop." -ForegroundColor Yellow

    try {
        while ($true) {
            Clear-Host
            Write-Host "=== Redis Cluster Health ===" -ForegroundColor Cyan
            Write-Host "Time: $(Get-Date)"

            Write-Host "`nCluster info:" -ForegroundColor Green
            try {
                docker exec -it redis-node-1 redis-cli -a $REDIS_PASS cluster info 2>$null
            }
            catch {
                Write-Host "cluster connection failed" -ForegroundColor Red
            }

            Write-Host "`nNode status:" -ForegroundColor Green
            foreach ($node in $NODES) {
                $status = try {
                    $ping = & docker exec -it $node redis-cli -a $REDIS_PASS ping 2>$null
                    if ($LASTEXITCODE -eq 0 -and $ping -eq "PONG") { "OK" } else { "FAIL" }
                }
                catch {
                    "FAIL"
                }
                Write-Host "Node $node : $status"
            }

            Write-Host "`nSlowlog size:" -ForegroundColor Green
            foreach ($node in $NODES) {
                try {
                    $slowCount = & docker exec -it $node redis-cli -a $REDIS_PASS slowlog len 2>$null
                    if ($LASTEXITCODE -eq 0) {
                        Write-Host "Node $node slowlog: $slowCount"
                    } else {
                        Write-Host "Node $node : connection failed"
                    }
                }
                catch {
                    Write-Host "Node $node : connection failed"
                }
            }

            Start-Sleep -Seconds 5
        }
    }
    catch {
        Write-Host "Monitor stopped." -ForegroundColor Yellow
    }
}

function Run-BenchmarkPing {
    Write-Host "=== Scenario 5: QPS spike with redis-benchmark (PING only) ===" -ForegroundColor Yellow
    Write-Host "This does NOT write data. It runs PING against each node locally." -ForegroundColor Green
    if ($BenchmarkSeconds -gt 0) {
        Write-Host "Clients: $BenchmarkClients, Duration: ${BenchmarkSeconds}s, Pipeline: $BenchmarkPipeline" -ForegroundColor Green
    } else {
        Write-Host "Clients: $BenchmarkClients, Requests: $BenchmarkRequests, Pipeline: $BenchmarkPipeline" -ForegroundColor Green
    }

    foreach ($node in $NODES) {
        Write-Host "`nBenchmarking $node ..." -ForegroundColor Cyan
        try {
            if ($BenchmarkSeconds -gt 0) {
                & docker exec -it $node redis-benchmark -a $REDIS_PASS -p 6379 -t ping -c $BenchmarkClients -P $BenchmarkPipeline -l -d 16 -t ping --test-time $BenchmarkSeconds
            } else {
                & docker exec -it $node redis-benchmark -a $REDIS_PASS -p 6379 -t ping -n $BenchmarkRequests -c $BenchmarkClients -P $BenchmarkPipeline
            }
        }
        catch {
            Write-Host "Benchmark failed on $node $_" -ForegroundColor Red
        }
    }
}

switch ($Scenario) {
    1 { Simulate-MasterFailure }
    2 { Simulate-HighLoad }
    3 { Simulate-SlowQueries }
    4 { Monitor-Cluster }
    5 { Run-BenchmarkPing }
    default {
        Write-Host "Redis cluster simulation script (PowerShell)" -ForegroundColor Cyan
        Write-Host "Usage: .\\test-redis-cluster.ps1 -Scenario [number] [-BenchmarkRequests N] [-BenchmarkClients N] [-BenchmarkPipeline N] [-BenchmarkSeconds N]"
        Write-Host ""
        Write-Host "Scenarios:" -ForegroundColor Yellow
        Write-Host "  1: Simulate master failure"
        Write-Host "  2: Monitor high QPS (read-only)"
        Write-Host "  3: Simulate slow query"
        Write-Host "  4: Cluster health monitor"
        Write-Host "  5: QPS spike (redis-benchmark PING)"
        Write-Host ""
        Write-Host "Examples:" -ForegroundColor Green
        Write-Host "  .\\test-redis-cluster.ps1 -Scenario 1"
        Write-Host "  .\\test-redis-cluster.ps1 -Scenario 4"
        Write-Host "  .\\test-redis-cluster.ps1 -Scenario 5 -BenchmarkRequests 500000 -BenchmarkClients 200"
        Write-Host "  .\\test-redis-cluster.ps1 -Scenario 5 -BenchmarkSeconds 60 -BenchmarkClients 200"
        Write-Host ""
        Write-Host "Or positional:"
        Write-Host "  .\\test-redis-cluster.ps1 1"
    }
}
