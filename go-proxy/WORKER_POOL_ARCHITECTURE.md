# Worker Pool Architecture - Brainstorming Session

**Date**: December 21, 2025  
**Status**: Planning Phase  
**Goal**: Convert go-proxy to heavily use worker pools for better concurrency and resource management

---

## Context

- **Server**: 8-core dedicated server in Germany (Hetzner)
- **Setup**: 3-node Docker Swarm (limited, no multi-region)
- **Workload**: Reverse proxy for ~20 services (Pterodactyl, Vaultwarden, Orbat, etc.)
- **Current Issue**: Unbounded goroutine spawning, especially in database writes
- **Shared Resources**: Proxy competes with other services (MariaDB, PostgreSQL, Redis, etc.)

---

## Current State Analysis

### ✅ What Already Has Worker Pools
- **Registry V2**: 5 pre-allocated workers for maintenance verification
  - Buffered channel (100 tasks)
  - No dynamic scaling
  - Simple, static allocation

### ❌ Major Bottlenecks (No Worker Pools)
1. **Access Logger** - Spawns unbounded goroutines per request
   ```go
   go func() {
       db.LogAccessRequest(entry)  // 1000s of goroutines under load
   }()
   ```

2. **Health Checker** - One goroutine per service (acceptable, but not pooled)
   ```go
   for _, svc := range services {
       go c.monitorService(ctx, svc)  // One per service forever
   }
   ```

3. **Proxy ServeHTTP** - No request pooling, synchronous work in request path

4. **Metrics Collector** - Atomic operations (good) but could batch better

### 🔥 Biggest Problem
At 1000 req/sec:
- 1000 connection goroutines (Go's http.Server)
- 1000 DB write goroutines (accesslog spawning)
- **Total: 2000+ goroutines**
- **Result**: SQLite contention, goroutine scheduling overhead, memory churn

---

## Proposed Architecture: Hybrid Worker Pools

### Design Principles

1. **Tier-based isolation**: Critical/High get dedicated pools, Normal/Low share
2. **Hybrid allocation**: Minimum workers always running + dynamic scaling
3. **Threshold-based spawning**: Don't spawn for tiny queues
4. **Work-stealing shared pool**: Helps overwhelmed pools on-demand
5. **Crash recovery**: Track in-flight tasks, recover on worker panic
6. **Conservative resource usage**: Leave CPU for other services

---

## Pool Configuration

### Pool Sizes (8-core server, shared with other services)

```
┌─────────────────────────────────────────────────────────┐
│ DEDICATED POOLS                                         │
├─────────────────────────────────────────────────────────┤
│ Critical Pool:  1 min → 4 max workers                   │
│   - Circuit breakers, error logging                     │
│   - Queue: 100 tasks                                    │
│   - Scale up ratio: 5.0 (spawn when queue/workers > 5) │
│   - Scale down ratio: 2.0 (remove when queue/workers <2)│
│   - Overflow ratio: 5.0 (alert when queue/maxWorkers >5)│
│   - Task timeout: 5s                                    │
│   - Max retries: 1                                      │
├─────────────────────────────────────────────────────────┤
│ High Pool:      1 min → 6 max workers                   │
│   - Health checks, access logging                       │
│   - Queue: 500 tasks                                    │
│   - Scale up ratio: 5.0 (spawn when queue/workers > 5) │
│   - Scale down ratio: 2.0 (remove when queue/workers <2)│
│   - Overflow ratio: 5.0 (alert when queue/maxWorkers >5)│
│   - Task timeout: 30s                                   │
│   - Max retries: 3                                      │
├─────────────────────────────────────────────────────────┤
│ Normal Pool:    1 min → 1 max workers                   │
│   - Metrics recording, analytics aggregation            │
│   - Queue: 200 tasks                                    │
│   - No scaling (serial execution)                       │
│   - Task timeout: 60s                                   │
│   - Max retries: 5                                      │
├─────────────────────────────────────────────────────────┤
│ Low Pool:       1 min → 1 max workers                   │
│   - Cleanup, retention policies                         │
│   - Queue: 100 tasks                                    │
│   - No scaling (serial execution)                       │
│   - Task timeout: 120s                                  │
│   - Max retries: 5                                      │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│ SHARED POOL (Work-Stealing)                            │
├─────────────────────────────────────────────────────────┤
│ Workers:        0 min → 12 max                          │
│   - NO queue (steals from dedicated pools)              │
│   - Pre-spawn strategy:                                 │
│     * When Critical OR High reaches maxWorkers          │
│     * Spawn 1 idle shared worker (ready to help)        │
│     * Faster response time vs spawning on-demand        │
│   - Activation rules:                                   │
│     * Critical/High: Steal when pool is FULL + queue    │
│     * Normal/Low: Steal when queue > threshold          │
│   - Checks pools in priority order:                     │
│     1. Critical (if maxed out + any queue)              │
│     2. High (if maxed out + any queue)                  │
│     3. Normal (if queue > 10)                           │
│     4. Low (if queue > 10)                              │
│   - Idle worker exits after 30s with no work            │
│   - Additional workers spawn if first is busy           │
└─────────────────────────────────────────────────────────┘

Total Workers: 4 min → 24 max (~25-30% of CPU capacity)
Memory Footprint: ~8KB idle → ~48KB peak
```

---

## Key Features

### 1. Hybrid Worker Allocation

**Minimum Workers (Always Running)**
- 1 worker per dedicated pool (4 pools)
- 0 workers in shared pool (spawned only when needed)
- **Total: 4 workers** always operational
- These workers NEVER exit (permanent=true)
- Guarantees minimum progress even under no load

**Dynamic Scaling (Spawned on Demand)**
- Temporary workers spawn when thresholds exceeded
- **Pre-spawning optimization**:
  - When Critical/High reaches maxWorkers → spawn 1 idle shared worker
  - This worker is ready to steal immediately (no spawn delay)
  - Provides ~1-2ms faster response time
- Additional shared workers spawn if needed (up to 12)
- All temporary workers exit after 30s idle
- Max caps prevent resource exhaustion

**Example Lifecycle**:
```
Idle:       Critical(1), High(1), Normal(1), Low(1), Shared(0) = 4 workers

Busy:       Critical(4 MAX), High(2), Normal(1), Low(1), Shared(1 idle) = 9 workers
            ↑ Critical maxed → pre-spawned 1 shared worker (waiting)

Overflow:   Critical(4 MAX + queue:20), High(2), Shared(1 active) = 7 workers
            ↑ Shared worker immediately starts stealing (no spawn delay)

Heavy:      Critical(4 MAX + queue:50), High(6 MAX), Shared(8 busy) = 18 workers
            ↑ More shared workers spawned as needed

Peak:       Critical(4), High(6), Normal(1), Low(1), Shared(12) = 24 workers
```

### 2. Dynamic Scaling Based on Queue-to-Worker Ratio

**Problem**: Fixed thresholds don't scale well with worker count

**Solution**: Scale based on tasks-per-worker ratio

```go
type DedicatedPool struct {
    tasksPerWorker    int  // Spawn new worker when queue/workers > this
    overflowRatio     int  // Alert shared pool when queue/maxWorkers > this
}

// Example: Critical Pool (max 4 workers)
tasksPerWorker:  5  // Spawn when queue > (activeWorkers × 5)
overflowRatio:   5  // Overflow when queue > (maxWorkers × 5) = 20
```

**Dynamic Scaling Behavior**:
```
1 worker active, queue: 1-5 tasks
  → queue/workers = 5 → No action (worker can handle it)

1 worker active, queue: 6 tasks
  → queue/workers = 6 > 5 → Spawn worker #2
  
2 workers active, queue: 11 tasks
  → queue/workers = 5.5 > 5 → Spawn worker #3
  
3 workers active, queue: 16 tasks
  → queue/workers = 5.3 > 5 → Spawn worker #4 (max reached)
  
4 workers active (MAX), queue: 21 tasks
  → queue/maxWorkers = 5.25 > 5 → Notify shared pool
```

**Advantages**:
- **Proportional scaling**: More workers → higher threshold
- **Self-balancing**: Prevents over-spawning
- **Consistent load**: Each worker handles ~5 tasks
- **Predictable**: Easy to reason about capacity

### 2b. Asymmetric Scale-Down (Hysteresis)

**Problem**: Scale up at 5 tasks/worker, scale down at 5 tasks/worker = thrashing

**Solution**: Use different thresholds for scale-up vs scale-down

```go
type DedicatedPool struct {
    scaleUpRatio    float64  // Spawn worker when queue/workers > this (e.g., 5.0)
    scaleDownRatio  float64  // Remove worker when queue/workers < this (e.g., 2.0)
}

// Example: Critical Pool (4 workers max)
scaleUpRatio:   5.0  // Spawn when queue/workers > 5
scaleDownRatio: 2.0  // Remove when queue/workers < 2
```

**Scale-Down Behavior**:
```
4 workers active, queue: 20 tasks
  → 20/4 = 5.0 → Keep all workers

4 workers active, queue: 12 tasks
  → 12/4 = 3.0 → Still above 2.0, keep all workers

4 workers active, queue: 7 tasks
  → 7/4 = 1.75 < 2.0 → Signal worker #4 to exit
  
3 workers active, queue: 5 tasks
  → 5/3 = 1.67 < 2.0 → Signal worker #3 to exit
  
2 workers active, queue: 3 tasks
  → 3/2 = 1.5 < 2.0 → Signal worker #2 to exit
  
1 worker remains (permanent, never exits)
```

**Benefits**:
- **Hysteresis gap**: Scale up at 5, scale down at 2 (2.5× difference)
- **Prevents thrashing**: Won't rapidly spawn/kill workers
- **Keeps buffer**: Extra workers handle small bursts without re-spawning
- **Smooth transitions**: Load fluctuations don't cause chaos

**Alternative Scale-Down Strategies**:

**Option A: Ratio-Based (Above)**
- Scale down when `queue/workers < 2.0`
- Pro: Symmetric with scale-up logic
- Con: Still somewhat reactive

**Option B: Idle Time Based**
- Worker exits after 30s with no tasks stolen
- Pro: Simple, current implementation
- Con: Doesn't respond to queue depth

**Option C: Combined (Recommended)**
```go
// Worker checks both conditions:
shouldExit := false

// Condition 1: Queue depth suggests we're over-capacity
if !permanent && len(queue) > 0 {
    queuePerWorker := float64(len(queue)) / float64(activeWorkers)
    if queuePerWorker < scaleDownRatio {
        shouldExit = true  // Scale down, we have too many workers
    }
}

// Condition 2: Idle timeout (no work for 30s)
if idleTime > 30*time.Second {
    shouldExit = true
}

// Never exit if we're at minimum workers
if activeWorkers <= minWorkers {
    shouldExit = false
}
```

**Recommended Configuration**:
```yaml
Critical Pool:
  scaleUpRatio: 5.0      # Spawn at 5 tasks/worker
  scaleDownRatio: 2.0    # Remove at 2 tasks/worker
  idleTimeout: 30s       # Also remove if idle 30s

High Pool:
  scaleUpRatio: 5.0
  scaleDownRatio: 2.0
  idleTimeout: 30s

Normal/Low Pools:
  scaleUpRatio: 10.0     # Less aggressive
  scaleDownRatio: 3.0
  idleTimeout: 60s       # Longer timeout
```

**Example Lifecycle with Hysteresis**:
```
Time  Queue  Workers  Action                      Ratio
----  -----  -------  --------------------------  -----
0s    6      1        Spawn #2 (6/1 = 6.0 > 5)    6.0↑
1s    11     2        Spawn #3 (11/2 = 5.5 > 5)   5.5↑
2s    16     3        Spawn #4 (16/3 = 5.3 > 5)   5.3↑
3s    15     4        Keep (15/4 = 3.75 > 2)      3.75
4s    12     4        Keep (12/4 = 3.0 > 2)       3.0
5s    8      4        Keep (8/4 = 2.0 = 2)        2.0
6s    7      4        Remove #4 (7/4 = 1.75 < 2)  1.75↓
7s    5      3        Remove #3 (5/3 = 1.67 < 2)  1.67↓
8s    4      2        Keep (4/2 = 2.0 = 2)        2.0
9s    3      2        Remove #2 (3/2 = 1.5 < 2)   1.5↓
10s   3      1        Keep (permanent worker)     3.0
```

**Submit Implementation with Dynamic Scaling**:
```go
func (dp *DedicatedPool) Submit(task Task) {
    dp.taskQueue <- task
    
    active := atomic.LoadInt32(&dp.activeWorkers)
    queueLen := len(dp.taskQueue)
    
    // Dynamic scaling UP: spawn when queue-to-worker ratio exceeds threshold
    tasksPerWorker := float64(queueLen) / float64(active)
    
    if int(active) < dp.maxWorkers && tasksPerWorker > dp.scaleUpRatio {
        dp.spawnWorker(false) // Spawn temporary worker
        active++ // Update for overflow check
    }
    
    // Check if we need shared pool help
    if int(active) >= dp.maxWorkers {
        tasksPerMaxWorker := float64(queueLen) / float64(dp.maxWorkers)
        
        if tasksPerMaxWorker > dp.overflowRatio {
            // Maxed out AND queue exceeds overflow ratio
            dp.sharedPool.NotifyOverflow(dp)
        } else if queueLen > 0 {
            // Maxed out with any queue - pre-spawn standby for Critical/High
            if dp.name == "critical" || dp.name == "high" {
                dp.sharedPool.PreSpawnIdleWorker()
            }
        }
    }
}

func (dp *DedicatedPool) worker(id int, permanent bool) {
    defer atomic.AddInt32(&dp.activeWorkers, -1)
    
    idleTimer := time.NewTimer(dp.idleTimeout)
    defer idleTimer.Stop()
    
    for {
        select {
        case task := <-dp.taskQueue:
            task.Execute()
            idleTimer.Reset(dp.idleTimeout)
            
            // Check if we should scale down (too many workers)
            if !permanent {
                active := atomic.LoadInt32(&dp.activeWorkers)
                queueLen := len(dp.taskQueue)
                
                if queueLen > 0 {
                    tasksPerWorker := float64(queueLen) / float64(active)
                    
                    // Scale DOWN when queue/workers < scaleDownRatio
                    if tasksPerWorker < dp.scaleDownRatio && int(active) > dp.minWorkers {
                        return // Exit this worker (graceful scale-down)
                    }
                }
            }
            
        case <-idleTimer.C:
            if !permanent {
                return // Exit after idle timeout (only temporary workers)
            }
            idleTimer.Reset(dp.idleTimeout)
        }
    }
}
```

### 3. Work-Stealing Shared Pool

**Key Innovation**: Shared pool has NO queue - it pulls from others

**Activation Logic** (Different per priority):
```
Critical/High pools:
  Shared workers activate when:
    1. Pool reaches maxWorkers (all workers busy)
    AND
    2. Queue has ANY tasks waiting (> 0)
  
  Rationale: Time-critical, can't wait for threshold
  
Normal/Low pools:
**Shared Worker with Intelligent Cleanup**:
```go
func (sp *SharedPool) worker() {
    defer atomic.AddInt32(&sp.activeWorkers, -1)
    
    idleTimer := time.NewTimer(30 * time.Second)
    defer idleTimer.Stop()
    
    for {
        stolen := false
        shouldStayAlive := false
        
        // Check if any high-priority pool still needs standby worker
        criticalMaxed := critical.isMaxedOut()
        highMaxed := high.isMaxedOut()
        
        // Stay alive if Critical or High are still at max capacity
        if criticalMaxed || highMaxed {
            shouldStayAlive = true
        }
        
        // Priority 1: Critical pool (if maxed out + any queue)
        if criticalMaxed && len(critical.queue) > 0 {
            select {
            case task := <-critical.queue:
                task.Execute()
                stolen = true
                idleTimer.Reset(30 * time.Second)
            default:
            }
        }
        
        // Priority 2: High pool (if maxed out + any queue)
        if !stolen && highMaxed && len(high.queue) > 0 {
            select {
            case task := <-high.queue:
                task.Execute()
                stolen = true
                idleTimer.Reset(30 * time.Second)
            default:
            }
        }
        
        // Priority 3: Normal pool (if queue > threshold)
        if !stolen && len(normal.queue) >= stealThreshold {
            select {
            case task := <-normal.queue:
                task.Execute()
                stolen = true
                idleTimer.Reset(30 * time.Second)
            default:
            }
        }
        
        // Priority 4: Low pool (if queue > threshold)
        if !stolen && len(low.queue) >= stealThreshold {
            select {
            case task := <-low.queue:
                task.Execute()
                stolen = true
                idleTimer.Reset(30 * time.Second)
            default:
            }
        }
        
        if !stolen {
            if shouldStayAlive {
                // Critical/High still maxed - stay in standby mode
                idleTimer.Reset(30 * time.Second)
                time.Sleep(1 * time.Millisecond)
            } else {
                // Check if idle timeout reached
                select {
                case <-idleTimer.C:
                    return // Exit after 30s idle with no pools maxed
                default:
                    time.Sleep(1 * time.Millisecond)
                }
            }
        }
    }
}
```

**Benefits**:
- **Low latency**: Pre-spawned worker eliminates spawn delay (~1-2ms faster)
- **Zero cold start**: Worker ready when Critical/High maxes out
- **Intelligent standby**: Stays alive while Critical/High are maxed (ready for burst)
- **Minimal overhead**: Only 1 idle worker pre-spawned (~2KB RAM)
- **Different rules per priority**: Urgent tasks get immediate help
- **Automatically helps busiest pool**: Dynamic load balancing
- **No queue to manage**: Work-stealing is simpler
- **Priority-aware**: Always checks critical first
- **Smart cleanup**: Only exits when pools scale down AND been idle 30s

### 4. Crash Recovery & Task Tracking

**Problem**: Worker crashes → lose in-flight tasks

**Solution**: Three-stage task lifecycle

```
┌─────────────┐
│   PENDING   │ (in taskQueue channel)
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ PROCESSING  │ (in processingMap sync.Map)
└──────┬──────┘
       │
   ┌───┴────┐
   ▼        ▼
SUCCESS   FAILED → RETRY (back to queue)
                      or
                   DEAD LETTER (max retries)
```

**TrackedTask**:
```go
type TrackedTask struct {
    ID          string
    Task        Task
    StartedAt   time.Time
    WorkerID    int
    Attempts    int
    LastError   error
}
```

**Recovery on Crash**:
```go
defer func() {
    if r := recover(); r != nil {
        // Worker crashed!
        recoverTasksFromWorker(workerID)  // Move processing → pending
        
        if permanent {
            go worker(workerID, true)  // Restart permanent workers only
        }
        // Temporary and shared workers don't restart (spawn on-demand)
    }
}()
```

**Timeout Handling**:
```go
ctx, cancel := context.WithTimeout(ctx, taskTimeout)

select {
case <-taskDone:
    // Success
case <-ctx.Done():
    // Timeout → retry with backoff
}
```

---

## Example Scaling Scenarios

### Scenario 1: Light Load
```
Critical: 1 worker active, queue: 3 tasks
  → queue/workers = 3/1 = 3 (≤ 5)
  → No action, single worker handles it

Shared: No workers spawned
Total: 4 workers (Critical, High, Normal, Low - all at minimum)
```

### Scenario 2: Moderate Load
```
Critical: 1 worker active, queue: 8 tasks
  → queue/workers = 8/1 = 8 (> 5)
  → Spawn worker #2
  
Critical: 2 workers active, queue: 11 tasks
  → queue/workers = 11/2 = 5.5 (> 5)
  → Spawn worker #3
  
Critical: 3 workers active, queue: 13 tasks
  → queue/workers = 13/3 = 4.3 (≤ 5)
  → No more spawning (ratio acceptable)

Shared: Pre-spawns NOT triggered (Critical not maxed)
Total: 6 workers (Critical=3, High=1, Normal=1, Low=1)
```

### Scenario 3: Critical Reaches Capacity
```
Critical: 3 workers active, queue: 16 tasks
  → queue/workers = 16/3 = 5.3 (> 5)
  → Spawns worker #4 (max reached)
  
Critical: 4 workers active (MAX!), queue: 15 tasks
  → 4 >= maxWorkers AND queue > 0
  → Triggers: sharedPool.PreSpawnIdleWorker()
  
Shared: 1 worker pre-spawned (idle, standby mode)
  → Checks critical: maxed (4/4) + queue (15) = conditions met
  → Worker ALREADY RUNNING, starts stealing immediately
  → No spawn delay!
  
Total: 5 workers (Critical=4, High=1, Normal=1, Low=1, Shared=1 standby)
Effect: Next burst of critical tasks served instantly
```

### Scenario 4: Critical Overwhelmed (Heavy Load)
```
Critical: 4 workers active (MAX!), queue: 25 tasks
  → queue/maxWorkers = 25/4 = 6.25 (> 5 overflow ratio)
  → Triggers: sharedPool.NotifyOverflow()
  
Shared: 1 worker pre-spawned + overflow notification
  → Worker #1 already stealing
  → queue/maxWorkers still > 5, spawn worker #2
  → queue/maxWorkers still > 5, spawn worker #3
  → Continues until queue/maxWorkers ≤ 5 OR max 12 workers
  
Critical: 4 workers, queue: 100 tasks
  → 100/4 = 25 tasks per worker!
  → Shared spawns many workers to help
  
Total: 16 workers (Critical=4, High=1, Normal=1, Low=1, Shared=9 active)
Effect: Shared pool handles overflow, critical queue drains fast
```

### Scenario 5: Combined Load (Multiple Pools)
```
Critical: 4 workers (MAX), queue: 18 tasks
  → 18/4 = 4.5 (< 5, no overflow)
  → But maxed with queue > 0 → Pre-spawn triggered
  
High: 3 workers, queue: 20 tasks
  → 20/3 = 6.7 (> 5)
  → Spawns worker #4, then #5, then #6 (max reached)
  → 6 >= maxWorkers, queue: 15 tasks → Pre-spawn triggered
  
Shared: 1 worker spawned (standby)
  → criticalMaxed=true OR highMaxed=true
  → shouldStayAlive = true
  → Worker stays alive, steals from both Critical and High
  → As queues grow, more shared workers spawn
  
Total: 15 workers (Critical=4, High=6, Normal=1, Low=1, Shared=3)
Effect: Shared workers balance load between Critical and High
```

### Scenario 6: Scale Down with Hysteresis (Load Decreases)
```
Previous state: Critical=4 workers, queue=20 tasks
  → 20/4 = 5.0 (all workers busy)

Load drops: Critical=4 workers, queue=12 tasks
  → 12/4 = 3.0 (above scale-down threshold of 2.0)
  → Keep all 4 workers (buffer for bursts)

Load drops more: Critical=4 workers, queue=7 tasks
  → 7/4 = 1.75 < 2.0 (below scale-down threshold!)
  → Worker #4 finishes task, checks ratio, exits gracefully
  
Now: Critical=3 workers, queue=5 tasks
  → 5/3 = 1.67 < 2.0
  → Worker #3 finishes task, exits
  
Now: Critical=2 workers, queue=3 tasks
  → 3/2 = 1.5 < 2.0
  → Worker #2 finishes task, exits
  
Final: Critical=1 worker (permanent), queue=3 tasks
  → 3/1 = 3.0 (single worker handles it fine)
  → If queue grows to 6: 6/1 = 6.0 > 5.0 → Spawn #2 again

Total: Back to 4 minimum workers
Effect: Smooth scale-down, no thrashing at boundary
```

**Anti-Thrashing Example**:
```
WITHOUT hysteresis (scale up/down both at 5.0):
Queue=10, Workers=2 → 10/2=5.0 → Spawn #3
Queue=9,  Workers=3 → 9/3=3.0 → Remove #3  ← Thrashing!
Queue=10, Workers=2 → 10/2=5.0 → Spawn #3  ← Waste!
Queue=9,  Workers=3 → 9/3=3.0 → Remove #3  ← Chaos!

WITH hysteresis (up=5.0, down=2.0):
Queue=10, Workers=2 → 10/2=5.0 → Spawn #3
Queue=9,  Workers=3 → 9/3=3.0 → Keep #3 (3.0 > 2.0) ✓
Queue=10, Workers=3 → 10/3=3.3 → Keep #3 (stable) ✓
Queue=5,  Workers=3 → 5/3=1.67 → Remove #3 (clean exit) ✓
```

### Scenario 7: Idle Timeout (No Load)
```
Previous: Critical=4, High=6, Shared=5 active
  → Heavy load subsides, queues clear
  
Critical: 4 workers, queue: 0
  → Temporary workers (3) idle for 30s → exit
  → Returns to 1 permanent worker
  
High: 6 workers, queue: 0
  → Temporary workers (5) idle for 30s → exit
  → Returns to 1 permanent worker
  
Shared: 5 workers idle
  → Critical no longer maxed (1/4)
  → High no longer maxed (1/6)
  → shouldStayAlive = false
  → All 5 workers exit after 30s idle
  
Total: 4 workers (back to minimum: Critical=1, High=1, Normal=1, Low=1)
Effect: Zero overhead when idle, ready to scale up again
```

---

### 5. Observability & Monitoring

**Pool Metrics**:
```go
type PoolStats struct {
    Name            string
    ActiveWorkers   int    // Currently running
    MinWorkers      int    // Guaranteed minimum
    MaxWorkers      int    // Hard cap
    PendingTasks    int    // Waiting in queue
    ProcessingTasks int    // Currently executing
    FailedTasks     int    // In dead letter queue
    TotalProcessed  uint64
    TotalFailed     uint64
}
```

**Dashboard Endpoints**:
- `/metrics/pools` - All pool statistics
- `/metrics/pools/{name}` - Specific pool details
- `/metrics/pools/{name}/stuck` - Find stuck tasks (> timeout)

**Alerts**:
- Queue depth > 80% of buffer size
- Dead letter queue growing
- Worker crash rate
- Task timeout rate

---

## Task Assignment by Type

### Critical Pool (Instant Response Required)
- Circuit breaker state updates
- Error logging (500 errors)
- Security violations (WAF blocks)
- Rate limit exceeded events
- **Why**: Must never be delayed, affects traffic immediately

### High Pool (Important, Time-Sensitive)
- Access logging (DB writes)
- Health check execution
- Certificate expiry checks
- Webhook notifications
- **Why**: Core observability, can't be lost

### Normal Pool (Can Wait, Not Urgent)
- Metrics aggregation
- Analytics computation
- Traffic analysis
- Response time histograms
- **Why**: Batch processing is fine

### Low Pool (Background Maintenance)
- Database cleanup
- Retention policy enforcement
- Old log rotation
- Temporary file cleanup
- **Why**: Can wait hours if needed

---

## Connection Handling Strategy

### Decision: Keep Separate (Option 1)

**Go's http.Server spawns goroutines per connection** (unchanged):
- Connection goroutines remain unlimited
- Each connection gets its own goroutine
- Leverages Go's excellent HTTP handling

**ServeHTTP offloads to worker pools**:
```go
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    // Minimal work in request path:
    backend := s.findBackend(host, path)  // Fast route lookup
    backend.Proxy.ServeHTTP(rw, r)        // Proxy request
    
    // Offload to worker pools (non-blocking):
    highPool.Submit(Task{
        Priority: PriorityHigh,
        Execute: func() {
            db.LogAccessRequest(entry)  // No longer spawns goroutine!
        },
    })
    
    sharedPool.Submit(Task{
        Priority: PriorityNormal,
        Execute: func() {
            metrics.Record(data)
        },
# Scaling ratios (asymmetric for hysteresis)
WORKER_CRITICAL_SCALE_UP_RATIO=5.0       # Spawn when queue/workers > 5
WORKER_CRITICAL_SCALE_DOWN_RATIO=2.0     # Remove when queue/workers < 2
WORKER_CRITICAL_OVERFLOW_RATIO=5.0       # Alert shared when queue/maxWorkers > 5

WORKER_HIGH_SCALE_UP_RATIO=5.0
WORKER_HIGH_SCALE_DOWN_RATIO=2.0
WORKER_HIGH_OVERFLOW_RATIO=5.0

WORKER_NORMAL_SCALE_UP_RATIO=10.0        # Less aggressive for low priority
WORKER_NORMAL_SCALE_DOWN_RATIO=3.0
WORKER_LOW_SCALE_UP_RATIO=10.0
WORKER_LOW_SCALE_DOWN_RATIO=3.0

WORKER_SHARED_STEAL_THRESHOLD=10         # Steal from Normal/Low when queue > 10
- Simpler than pooling entire request lifecycle
- Only need Option 2 (request pooling) if seeing >5000 concurrent connections

---

## Configuration

### Environment Variables

```yaml
# Pool sizes
WORKER_CRITICAL_MIN=1
WORKER_CRITICAL_MAX=4
WORKER_HIGH_MIN=1
WORKER_HIGH_MAX=6
WORKER_NORMAL_MIN=1
WORKER_NORMAL_MAX=1
WORKER_LOW_MIN=1
WORKER_LOW_MAX=1
WORKER_SHARED_MIN=0    # Shared pool spawns on-demand only
WORKER_SHARED_MAX=12

# Queue sizes
WORKER_CRITICAL_QUEUE=100
WORKER_HIGH_QUEUE=500
WORKER_NORMAL_QUEUE=200
WORKER_LOW_QUEUE=100

# Scaling ratios
WORKER_CRITICAL_TASKS_PER_WORKER=5    # Spawn when queue/workers > 5
WORKER_CRITICAL_OVERFLOW_RATIO=5      # Alert shared when queue/maxWorkers > 5
WORKER_HIGH_TASKS_PER_WORKER=5        # Spawn when queue/workers > 5
WORKER_HIGH_OVERFLOW_RATIO=5          # Alert shared when queue/maxWorkers > 5
WORKER_NORMAL_TASKS_PER_WORKER=10     # For future scaling
WORKER_LOW_TASKS_PER_WORKER=10        # For future scaling
WORKER_SHARED_STEAL_THRESHOLD=10      # Steal from Normal/Low when queue > 10

# Timeouts
WORKER_CRITICAL_TASK_TIMEOUT=5s
WORKER_HIGH_TASK_TIMEOUT=30s
WORKER_NORMAL_TASK_TIMEOUT=60s
WORKER_LOW_TASK_TIMEOUT=120s
WORKER_IDLE_TIMEOUT=30s

# Retry
WORKER_CRITICAL_MAX_RETRIES=1
WORKER_HIGH_MAX_RETRIES=3
WORKER_NORMAL_MAX_RETRIES=5
WORKER_LOW_MAX_RETRIES=5
```

### Docker Compose Integration

```yaml
services:
  proxy:
    image: ghcr.io/chilla55/go-proxy:nightly
    environment:
      # Enable worker pools
      WORKER_POOLS_ENABLED: "1"
      
      # Auto-detect cores and calculate defaults
      WORKER_AUTO_CONFIGURE: "1"
      
      # Or manual override
      WORKER_CRITICAL_MAX: "4"
      WORKER_HIGH_MAX: "6"
      WORKER_SHARED_MAX: "12"
    
    deploy:
      resources:
        limits:
          cpus: '2'      # Proxy gets 2 of 8 cores
          memory: 4G
```

---

## Implementation Plan

### Phase 1: Core Worker Pool Package
- [ ] Create `worker/` package
- [ ] Implement `DedicatedPool` with hybrid allocation
- [ ] Implement `SharedPool` with work-stealing
- [ ] Add task tracking (pending/processing/failed)
- [ ] Add crash recovery
- [ ] Add metrics/observability

### Phase 2: Integrate with Existing Systems
- [ ] Refactor `accesslog.Logger` to use High pool
- [ ] Refactor `metrics.Collector` to use Normal pool
- [ ] Refactor `health.Checker` to use High pool
- [ ] Refactor `analytics.Aggregator` to use Normal pool
- [ ] Add cleanup tasks to Low pool

### Phase 3: Database Optimization
- [ ] Implement batch writes for SQLite
- [ ] Buffer 100 access log entries
- [ ] Flush every 100ms or when buffer full
- [ ] Reduce DB transactions 50x (1000/s → 20/s)

### Phase 4: Testing & Tuning
- [ ] Load testing (1000 req/s sustained)
- [ ] Burst testing (10000 req/s spike)
- [ ] Memory profiling
- [ ] CPU profiling
- [ ] Tune thresholds based on results

### Phase 5: Monitoring & Documentation
- [ ] Add Prometheus metrics for pools
- [ ] Dashboard integration
- [ ] Performance documentation
- [ ] Deployment guide

---

## Expected Benefits

### Performance Improvements
- **50x reduction** in DB transactions (batching)
- **90% reduction** in goroutine spawning (pooling)
- **30% lower** memory usage (bounded workers)
- **1-2ms faster response**: Pre-spawned worker eliminates spawn delay for critical/high
- **Predictable** latency under load (no goroutine explosion)
- **Minimal pre-spawn overhead**: Only 1 idle worker (~2KB RAM) when maxed out

### Resource Management
- **Conservative CPU usage**: 25-30% of server capacity
- **Graceful degradation**: Queues instead of crashes
- **Coexistence**: Leaves resources for other services

### Reliability
- **No lost tasks**: Crash recovery
- **Automatic retry**: Exponential backoff
- **Dead letter queue**: Investigate failures
- **Observability**: See exactly what's happening

### Operational Benefits
- **Tunable**: All thresholds configurable
- **Adaptive**: Auto-scales with traffic
- **Efficient**: Workers sleep when idle
- **Debuggable**: Full task lifecycle visibility

---

## Open Questions / Future Considerations

1. **Should Normal/Low pools scale beyond 1 worker?**
   - Pro: Better burst handling with dynamic ratio-based scaling
   - Con: More complexity, rarely needed for background tasks
   - Decision: Keep at 1 for now, ratio system ready if needed later

2. **Should we implement priority within shared pool stealing?**
   - Currently steals in pool priority order (critical → high → normal → low)
   - Could add task-level priority within each pool

3. **Database batching: Transaction vs single writes?**
   - Transaction: Faster, but all-or-nothing
   - Individual: Slower, but partial success possible

4. **Should critical pool tasks skip retry?**
   - Circuit breakers should fail-fast
   - But error logging should retry

5. **CPU affinity: Pin critical workers to specific cores?**
   - Possible on Linux with `runtime.LockOSThread()`
   - Needs careful testing

6. **Metrics collection: Should it be async?**
   - Currently atomic operations (fast)
   - Could offload aggregation to Normal pool

---

## Alternatives Considered

### Alternative 1: Pre-allocated Only (Rejected)
- Simple, predictable
- But wastes resources when idle
- Can't handle bursts

### Alternative 2: Fully Dynamic (Rejected)
- Maximum flexibility
- But adds complexity
- Cold start issues under burst

### Alternative 3: Work-Stealing at All Levels (Rejected)
- Elegant design
- But normal/low don't need it
- Over-engineering for small queues

---

## References

- Go's `http.Server` goroutine-per-connection model
- Registry V2 existing worker pool (maintenance tasks)
- SQLite write contention issues
- Existing metrics collection (atomic operations)

---

## Next Steps

1. **Review this document** with team/stakeholders
2. **Finalize configuration values** based on production metrics
3. **Create GitHub issue** with implementation checklist
4. **Start Phase 1**: Core worker pool package
5. **Benchmark** before/after for validation

---

**End of Brainstorming Session**
