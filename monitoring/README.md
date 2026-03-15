# WP9 Monitoring — Test Observability

Runs the test suites, pushes results to Prometheus Pushgateway, and visualizes them in Grafana.

## Prerequisites

### Unit & Integration Tests
| Tool | Purpose |
|------|---------|
| Docker | Runs Prometheus, Pushgateway, Grafana |
| Go 1.21+ | Executes test suites |
| Python 3 | Parses test output and builds metrics payload |

### E2E Tests (additional)
| Tool | Purpose |
|------|---------|
| kind ≥ 0.23 | Creates a local Kubernetes cluster |
| kubectl | Applies CRDs, RBAC, and deployment manifests |
| Docker | Builds the controller image |

> E2E tests spin up a 3-node kind cluster (1 control-plane + 2 workers), deploy the optimizer controller, run smoke and dry-run tests, then tear down. This requires the main repo to be available as a sibling directory.

---

## Quick Start

### 1. Start monitoring stack
```bash
cd monitoring
docker compose up -d
```
Grafana → http://localhost:3000 (admin / admin)

### 2. Run tests and push results

**Unit + Integration only (no cluster needed):**
```bash
bash monitoring/run_tests_and_push.sh all
```

**E2E only (kind cluster required):**
```bash
# Start cluster and deploy controller
bash ../optimizer-test/scripts/setup-kind.sh

# Run E2E tests
bash monitoring/run_tests_and_push.sh e2e

# Tear down cluster when done
bash ../optimizer-test/scripts/teardown-kind.sh
```

**All suites:**
```bash
bash ../optimizer-test/scripts/setup-kind.sh
bash monitoring/run_tests_and_push.sh all
bash ../optimizer-test/scripts/teardown-kind.sh
```

---

## Directory Structure

```
monitoring/
├── docker-compose.yml                        # Prometheus + Pushgateway + Grafana
├── prometheus.yml                            # Scrape config
├── run_tests_and_push.sh                     # Test runner and metrics pusher
└── grafana/
    ├── dashboards/
    │   └── optimizer-tests.json              # WP9 dashboard
    └── provisioning/
        ├── dashboards/dashboards.yml
        └── datasources/prometheus.yml
```

---

## Expected Repository Layout

The test runner expects `optimizer-test` to be a sibling of this repository:

```
<workspace>/
├── intelligent-cluster-optimizer/   ← this repo
│   └── monitoring/
└── optimizer-test/                  ← test repo (clone separately)
```

```bash
git clone https://github.com/k8s-resource-optimizer/intelligent-cluster-optimizer
git clone https://github.com/k8s-resource-optimizer/optimizer-test
```

If your layout differs, set the `OPTIMIZER_TEST_DIR` environment variable:
```bash
export OPTIMIZER_TEST_DIR=/path/to/optimizer-test
bash monitoring/run_tests_and_push.sh all
```

---

## WP9 Success Criteria

| Suite | Coverage Target | Tests |
|-------|----------------|-------|
| Unit | > 80% | 438 |
| Integration | > 80% | 152 |
| E2E | smoke + dry-run | 15 |
