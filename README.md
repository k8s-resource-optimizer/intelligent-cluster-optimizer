# Intelligent Cluster Optimizer

A Kubernetes-native resource optimization system that automatically right-sizes workloads based on historical usage patterns, reducing costs while maintaining performance and reliability.

## Project Overview

This project implements an intelligent resource optimizer for Kubernetes clusters. It collects metrics, analyzes usage patterns, detects anomalies, and generates (or auto-applies) resource recommendations for workloads.

### Key Goals
- **Cost Reduction**: Right-size over-provisioned workloads
- **Reliability**: Detect memory leaks, prevent OOM kills
- **Safety**: Conservative defaults, circuit breakers, rollback capability
- **Intelligence**: Time-based patterns, confidence scoring, environment profiles

---

## Architecture

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Metrics        │────▶│  Analysis       │────▶│  Recommendation │
│  Collection     │     │  Engine         │     │  Engine         │
└─────────────────┘     └─────────────────┘     └─────────────────┘
        │                       │                       │
        ▼                       ▼                       ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Anomaly        │     │  Prediction     │     │  Pareto         │
│  Detection      │     │  (Holt-Winters) │     │  Optimizer      │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                ┌───────────────────────┤
                                ▼                       ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Policy         │────▶│  SLA            │────▶│  Safety         │
│  Engine         │     │  Monitor        │     │  Checks         │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  Rollback       │◀────│  Applier        │     │  GitOps         │
│  Manager        │     │                 │     │  Exporter       │
└─────────────────┘     └─────────────────┘     └─────────────────┘
```

---

## Implementation Status

### Core Features

| Feature | Status | Description |
|---------|--------|-------------|
| Recommendation Engine | ✅ Done | P95/P99 percentile-based resource recommendations |
| Cost Estimator | ✅ Done | Calculates potential savings (hourly/monthly/yearly) |
| OOM Detection | ✅ Done | Detects OOM-killed containers, boosts memory, prioritizes |
| Confidence Scoring | ✅ Done | 0-100% score based on data quality (5 weighted factors) |
| Recommendation Expiry | ✅ Done | TTL-based expiry prevents stale recommendations |
| Memory Leak Detection | ✅ Done | Linear regression slope analysis with R² consistency |
| Time Pattern Analyzer | ✅ Done | Detects business hours, night batch, spike patterns |
| Environment Profiles | ✅ Done | Production/Staging/Development/Test presets |
| Circuit Breaker | ✅ Done | Stops scaling after repeated failures |
| Emergency Rollback | ✅ Done | Reverts changes if health checks fail |
| HPA/PDB Conflict Check | ✅ Done | Avoids conflicts with existing autoscalers |

### Advanced Analytics

| Feature | Status | Description |
|---------|--------|-------------|
| Anomaly Detection | ✅ Done | Multi-method consensus (Z-Score, IQR, Moving Average) |
| Time Series Prediction | ✅ Done | Holt-Winters forecasting for proactive scaling |
| Pareto Optimization | ✅ Done | Multi-objective optimization (cost, performance, reliability, efficiency, stability) |

### Policy & Governance

| Feature | Status | Description |
|---------|--------|-------------|
| Policy Engine | ✅ Done | Expression-based policies with YAML configuration |
| SLA Monitoring | ✅ Done | Latency, error rate, availability, throughput tracking |
| Health Checker | ✅ Done | Control chart-based health assessment |

### GitOps Integration

| Feature | Status | Description |
|---------|--------|-------------|
| Kustomize Export | ✅ Done | Strategic merge and JSON 6902 patch generation |
| Helm Export | ✅ Done | Values.yaml generation for Helm charts |

### Infrastructure

| Component | Status | Description |
|-----------|--------|-------------|
| Kubernetes Controller | ✅ Done | Reconciliation loop watching OptimizerConfig CRD |
| Custom Resource (CRD) | ✅ Done | OptimizerConfig for declarative configuration |
| Metrics Collector | ✅ Done | Collects pod CPU/memory from metrics API |
| In-Memory Storage | ✅ Done | Stores historical metrics |
| Vertical Scaler | ✅ Done | Patches deployment resource specs |
| Event Recorder | ✅ Done | Records Kubernetes events for audit |

### Testing

| Test Type | Status | Files | Cases | Coverage |
|-----------|--------|------:|------:|---------|
| Unit Tests | ✅ Done | 48 | 606 | 84.2% of `pkg/` |
| Integration Tests | ✅ Done | 37 | 341 | 77.8% of `pkg/` |
| E2E Tests | ✅ Done | 6 | 12 | N/A (kind cluster) |
| Race Detection | ✅ Done | — | — | `go test -race ./...` passes |
| **Total** | ✅ **≥80% target met** | **91** | **959** | **83.0%** |

**Coverage by package (measured):** events 100%, applier 98%, cost 98%, scheduler 97%, pareto 96%, profile 96%, apiserver 94%, sla 94%, prediction 93%, anomaly 91%, policy 91%, storage 91%, recommendation 90%, rollback 90%, safety 89%, leakdetector 83%, apis 83%, trends 83%, forecaster 77%, gitops 74%, scaler 72%, controller 48%.

### Code Quality

| Aspect | Status | Details |
|--------|--------|---------|
| Linting | ✅ Passing | golangci-lint clean |
| Formatting | ✅ Passing | gofmt compliant |
| Security Scan | ✅ Passing | gosec, govulncheck clean |
| Error Handling | 🟡 Needs Work | Some errors swallowed, missing context wrapping |
| Input Validation | 🟡 Needs Work | Missing bounds checking in parsers |
| Code Duplication | 🟡 Moderate | Workload type handlers (Deployment/StatefulSet/DaemonSet) |
| Performance | ✅ Fixed | O(n²) sort replaced with `sort.Slice` |
| Resource Management | ✅ Fixed | GC goroutine respects context cancellation; storage protected by `sync.RWMutex` |

**Known Issues:**
- 🟡 **Medium:** Circuit breaker mutates shared state without persistence
- 🟡 **Medium:** 200+ lines of duplicated workload handling code

### Production Readiness

| Component | Status | Notes |
|-----------|--------|-------|
| Health Probes | ✅ Done | `/healthz` (liveness) and `/readyz` (readiness) on `:8081` |
| Graceful Shutdown | ✅ Done | Signal handler cancels context; all goroutines exit cleanly |
| Observability | 🟡 Partial | Prometheus metrics exported (34 metrics), logs need structure |
| High Availability | ✅ Done | Leader election via `k8s.io/client-go/tools/leaderelection` |
| Backup/Restore | ✅ Done | Periodic backup manager with restore-on-startup |
| Security Hardening | ✅ Done | Non-root, read-only FS, RBAC configured |
| Deployment Automation | ✅ Done | Helm chart with production/staging/development profiles |

**Remaining gaps:**
1. **Observability:** Structured JSON logging and distributed tracing not yet added
2. **Scalability:** Storage is in-memory per-instance; no cross-replica sharing

---

## Project Structure

```
intelligent-cluster-optimizer/
├── cmd/
│   ├── controller/      # Main Kubernetes controller
│   ├── collector/       # Standalone metrics collector
│   └── optctl/          # CLI tool
├── config/
│   └── crd/             # Kubernetes CRD definitions
├── pkg/
│   ├── apis/            # Custom Resource types
│   ├── apiserver/       # REST API backend for the React UI
│   ├── controller/      # Kubernetes controller logic
│   ├── recommendation/  # Core recommendation engine (P95/P99 percentile)
│   ├── leakdetector/    # Memory leak detection (linear regression)
│   ├── timepattern/     # Time-based pattern analysis (business hours, batch)
│   ├── profile/         # Environment profiles (prod/staging/dev/test)
│   ├── safety/          # Safety checks (OOM, HPA, PDB, circuit breaker)
│   ├── rollback/        # Emergency rollback system
│   ├── cost/            # Cost calculation (AWS/GCP/Azure/generic)
│   ├── metrics/         # Metrics collection from Kubernetes API
│   ├── storage/         # In-memory time-series storage with backup/restore
│   ├── applier/         # Change application to workloads
│   ├── scaler/          # Vertical scaling (patches deployment specs)
│   ├── scheduler/       # Maintenance windows
│   ├── anomaly/         # Statistical anomaly detection (Z-Score, IQR, MA)
│   ├── prediction/      # Time series forecasting (Holt-Winters triple smoothing)
│   ├── forecaster/      # Chronos-2 ML client with Holt-Winters fallback
│   ├── pareto/          # Multi-objective Pareto optimization (5 objectives)
│   ├── policy/          # Expression-based policy engine
│   ├── sla/             # SLA monitoring and control-chart health checks
│   ├── gitops/          # GitOps export (Kustomize, Helm)
│   ├── notifications/   # Webhook, Slack, and email notifications
│   ├── reports/         # Cost optimization report generator (JSON/CSV/HTML)
│   ├── simulator/       # What-if scenario simulator (aggressive/balanced/conservative)
│   ├── trends/          # Capacity and growth rate analysis
│   └── events/          # Kubernetes event broadcasting
└── go.mod
```

---

## How It Works

### 1. Data Collection
- Scrapes CPU/memory metrics from Kubernetes metrics API
- Stores 24 hours of historical data per container
- Detects anomalies using statistical methods (Z-Score, IQR, Moving Average)

### 2. Analysis
- **Leak Detection**: Analyzes memory slope; blocks scaling if leak detected
- **Pattern Detection**: Identifies business hours, night batch, spike patterns
- **Anomaly Detection**: Multi-method consensus for outlier detection
- **Time Series Prediction**: Holt-Winters forecasting for proactive scaling
- **Profile Resolution**: Applies environment-specific settings (prod vs dev)

### 3. Recommendation Generation
- Calculates P95/P99 percentiles from historical usage
- Applies safety margin (1.2x default)
- Boosts memory for OOM-affected containers
- Scores confidence based on data quality
- Estimates cost savings
- **Pareto Optimization**: Generates multiple solutions balancing:
  - Cost (minimize resource spend)
  - Performance (headroom above average usage)
  - Reliability (buffer for peak loads)
  - Efficiency (resource utilization)
  - Stability (minimize change frequency)

### 4. Policy Evaluation
- Evaluates YAML-defined policies with expression-based conditions
- Supports actions: allow, deny, skip, modify, require-approval
- Enforces resource limits (min/max CPU/memory)
- Priority-based policy ordering

### 5. SLA Monitoring
- Tracks latency, error rate, availability, throughput SLAs
- Percentile-based latency checks (P95, P99)
- Control chart-based health assessment
- Blocks scaling during SLA violations

### 6. Safety Checks
- Verifies no HPA/PDB conflicts
- Checks circuit breaker state
- Validates recommendation confidence threshold
- Enforces MaxChangePercent limits

### 7. Application
- Patches deployment resource requests/limits
- Monitors health for rollback window
- Records events for audit trail

### 8. GitOps Export
- Exports recommendations as Kustomize patches (strategic merge or JSON 6902)
- Generates Helm values.yaml for GitOps workflows
- Supports PR-based review processes

---

## Test Scenarios

The integration tests validate these real-world scenarios:

| Scenario | Input Pattern | Expected Behavior |
|----------|---------------|-------------------|
| Memory Leak | Continuously growing memory | Block scaling, alert |
| Stable Usage | Normal GC sawtooth, low usage | Recommend scale down |
| Business Hours | High 9-5, low otherwise | Recommend schedule-based scaling |
| High Usage | Consistently near limits | Recommend scale up |

---

## Quick Start

### Prerequisites

- Go 1.21+
- Kubernetes cluster with metrics-server installed
- kubectl configured to access your cluster

### Installation

```bash
# Clone the repository
git clone https://github.com/yourusername/intelligent-cluster-optimizer.git
cd intelligent-cluster-optimizer

# Build all binaries
go build -o bin/optimizer-controller ./cmd/controller/
go build -o bin/optimizer-collector ./cmd/collector/
go build -o bin/optctl ./cmd/optctl/

# Install CRD
kubectl apply -f config/crd/
```

### Basic Usage

```bash
# Run the controller (connects to current kubeconfig context)
./bin/optimizer-controller

# Use the CLI to calculate resource costs
./bin/optctl cost default

# View optimization history
./bin/optctl history

# Rollback a workload to previous configuration
./bin/optctl rollback default/Deployment/nginx
```

---

## CLI Reference (optctl)

The `optctl` CLI provides commands for cluster monitoring, cost analysis, history tracking, and rollback operations.

### Commands

| Command | Description |
|---------|-------------|
| `dashboard` | Show cluster overview with resources, costs, and history |
| `cost [namespace]` | Calculate resource costs for workloads |
| `cost pricing` | Show available cloud pricing models |
| `history [resource]` | Show optimization history |
| `rollback <resource>` | Rollback workload to previous configuration |

### Dashboard

Get a quick overview of your cluster:

```bash
# Show cluster dashboard
optctl dashboard
```

**Sample Output:**
```
╔══════════════════════════════════════════════════════════════════════════════╗
║                    INTELLIGENT CLUSTER OPTIMIZER                              ║
║                           Dashboard v1.2.0                                    ║
╚══════════════════════════════════════════════════════════════════════════════╝

┌─ CLUSTER OVERVIEW ───────────────────────────────────────────────────────────┐
│  Nodes: 3    Namespaces: 5    Workloads: 12    Containers: 18    Replicas: 24 │
└──────────────────────────────────────────────────────────────────────────────┘

┌─ RESOURCE SUMMARY ───────────────────────────────────────────────────────────┐
│  Total CPU Requests:     4.50 cores                                          │
│  Total Memory Requests:  8.00 Gi                                             │
└──────────────────────────────────────────────────────────────────────────────┘

┌─ COST SUMMARY (aws-us-east-1) ───────────────────────────────────────────────┐
│  Hourly:   $0.25     Daily:    $6.00     Monthly:  $180.00    Yearly: $2190   │
└──────────────────────────────────────────────────────────────────────────────┘
```

### Cost Calculator

Calculate current resource costs with cloud provider pricing:

```bash
# Show available pricing models
optctl cost pricing

# Calculate costs for a namespace (default pricing)
optctl cost production

# Calculate costs with AWS pricing
optctl --pricing=aws-us-east-1 cost production

# Calculate costs across all namespaces
optctl --all-namespaces cost
```

**Sample Output:**
```
Resource Cost Report (Pricing: aws-us-east-1)
================================================================================
Namespaces: 1 | Workloads: 5 | Containers: 8 | Replicas: 12
--------------------------------------------------------------------------------
NAMESPACE  WORKLOAD              REPLICAS  CPU         MEMORY    COST/MONTH
prod       Deployment/api        3         500m        512 Mi    $45.36
prod       StatefulSet/postgres  1         1.00 cores  2.00 Gi   $34.56
--------------------------------------------------------------------------------
COST SUMMARY
--------------------------------------------------------------------------------
Total CPU:      2.50 cores
Total Memory:   4.00 Gi
Monthly Cost:   $89.86
Yearly Cost:    $1093.01
```

**Supported Pricing Models:**
- `aws-us-east-1` - AWS On-Demand (US East)
- `aws-us-east-1-spot` - AWS Spot (~70% discount)
- `gcp-us-central1` - Google Cloud (US Central)
- `azure-eastus` - Azure (US East)
- `default` - Generic conservative estimate

### History Tracking

View optimization history and previous configurations:

```bash
# Show all optimization history
optctl history

# Show history for specific workload
optctl --container=nginx history default/Deployment/nginx
```

**Sample Output:**
```
Optimization History (3 entries across 2 workloads)
--------------------------------------------------------------------------------
WORKLOAD                  CONTAINER  CPU   MEMORY  TIMESTAMP         AGE
default/Deployment/nginx  nginx      200m  256Mi   2025-12-27 12:00  2h
default/Deployment/nginx  nginx      100m  128Mi   2025-12-27 10:00  4h
prod/StatefulSet/redis    redis      500m  512Mi   2025-12-26 08:00  1d
```

### Rollback

Revert workloads to previous resource configurations:

```bash
# Rollback to previous configuration
optctl rollback default/Deployment/nginx

# Rollback specific container
optctl --container=app rollback prod/StatefulSet/redis
```

### Global Options

| Option | Description |
|--------|-------------|
| `--kubeconfig` | Path to kubeconfig (default: ~/.kube/config) |
| `--container` | Target container name |
| `--pricing` | Pricing model for cost calculation |
| `--all-namespaces` | Operate across all namespaces |
| `--history-file` | Path to history file |
| `--json` | Output in JSON format |

---

## CI/CD Pipeline

This project uses GitHub Actions for continuous integration and delivery. The pipeline runs automatically on every push to `main` and on pull requests.

### Pipeline Stages

```
┌─────────┐     ┌─────────┐     ┌──────────┐     ┌─────────┐     ┌─────────┐
│  Lint   │────▶│  Test   │────▶│ Security │────▶│  Build  │────▶│ Release │
└─────────┘     └─────────┘     └──────────┘     └─────────┘     └─────────┘
```

| Stage | Tools | Description |
|-------|-------|-------------|
| **Lint** | gofmt, golangci-lint | Code formatting and static analysis |
| **Test** | go test, Codecov | Unit tests with race detection and coverage |
| **Security** | gosec, govulncheck | Security vulnerability scanning |
| **Build** | go build | Compile all binaries |
| **Release** | GitHub Releases | Cross-platform binaries (Linux/macOS, amd64/arm64) |

### Running Locally

```bash
# Format code
go fmt ./...

# Run linter
golangci-lint run

# Run tests with coverage
go test -v -race -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Security scan
gosec ./...
govulncheck ./...

# Build
go build -v ./cmd/...
```

### Release Process

Releases are triggered automatically when a version tag is pushed:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This creates a GitHub Release with pre-built binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64)

---

## Running Tests

```bash
# Run all tests
go test ./pkg/...

# Run integration tests only
go test ./pkg/recommendation/... -run Integration -v

# Run specific package tests
go test ./pkg/leakdetector/... -v
go test ./pkg/timepattern/... -v
go test ./pkg/safety/... -v
go test ./pkg/anomaly/... -v
go test ./pkg/prediction/... -v
go test ./pkg/pareto/... -v
go test ./pkg/policy/... -v
go test ./pkg/sla/... -v
go test ./pkg/gitops/... -v
```

---

## Known Limitations

### Current Constraints
1. **In-Memory Storage per instance** - Metrics are not shared across replicas; leader election ensures single-writer consistency
2. **`timepattern` not yet integrated** - Business-hours and batch-pattern detection is implemented (`pkg/timepattern`) but not yet wired into the reconcile loop
3. **`optctl report` / `optctl simulate`** - CLI stubs exist; full offline report generation requires a running controller to supply recommendation data

### Performance Considerations
- **Recommendation Generation:** ~50ms for typical workload (1000 samples)
- **Storage Cleanup:** O(n×m) complexity, slow for >10k pods
- **Memory Usage:** ~1-2 MB per pod with 24h retention
- **API Calls:** 60+ calls per StatefulSet rollout (polling-based)

### Not Suitable For
- ❌ Multi-cluster optimization (single cluster only)
- ❌ Real-time scaling (<5 minute intervals)
- ❌ Stateless workloads with <1 hour uptime
- ❌ Clusters with >50k pods (memory constraints)
- ❌ Air-gapped environments (requires Metrics API)

### Compatibility
- ✅ **Kubernetes:** 1.20+ (tested on 1.24-1.28)
- ✅ **Metrics Server:** Required for data collection
- ✅ **Go:** 1.26.2+ for building from source
- ⚠️ **HPA:** Compatible but manual coordination needed
- ⚠️ **VPA:** May conflict, choose one or the other

---

## Technologies

- **Language**: Go 1.26.2+
- **Framework**: Kubernetes controller-runtime
- **APIs**: Kubernetes metrics API, custom CRDs
- **Policy Engine**: expr-lang/expr for expression evaluation
- **Testing**: Go testing, table-driven tests, CSV test data
- **GitOps**: Kustomize patches, Helm values generation
- **CI/CD**: GitHub Actions, golangci-lint, gosec, govulncheck, Codecov

---

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Ensure code passes all checks:
   ```bash
   go fmt ./...
   golangci-lint run
   go test -race ./...
   gosec ./...
   ```
4. Commit your changes using conventional commits (`git commit -m 'feat: add amazing feature'`)
5. Push to the branch (`git push origin feature/amazing-feature`)
6. Open a Pull Request

### Commit Message Format

We use conventional commits:
- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation changes
- `test:` - Adding or updating tests
- `refactor:` - Code refactoring
- `ci:` - CI/CD changes
- `chore:` - Maintenance tasks

---

## Security

See [SECURITY.md](SECURITY.md) for security policy and vulnerability reporting.

---

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## Authors

- **Azra Karakaya** - ML/Analytics (Pareto optimization, Holt-Winters prediction, anomaly detection)
- **Erva Şengül** - Infrastructure (SLA monitoring, GitOps, policy engine, controller)
