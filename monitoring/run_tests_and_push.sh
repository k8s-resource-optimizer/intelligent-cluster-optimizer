#!/usr/bin/env bash
# Run optimizer tests and push results to Prometheus Pushgateway
set -euo pipefail

PUSHGATEWAY="${PUSHGATEWAY_URL:-http://localhost:9091}"
# Test repo path: override with OPTIMIZER_TEST_DIR env var if needed
# Default: looks for optimizer-test as sibling of the repo root
_SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
_REPO_ROOT="$(cd "$_SCRIPT_DIR/.." && pwd)"
OPTIMIZER_TEST_DIR="${OPTIMIZER_TEST_DIR:-$(cd "$_REPO_ROOT/../optimizer-test" 2>/dev/null && pwd)}"
if [[ -z "$OPTIMIZER_TEST_DIR" || ! -d "$OPTIMIZER_TEST_DIR" ]]; then
  err "optimizer-test repo not found. Set OPTIMIZER_TEST_DIR env var."
  err "  export OPTIMIZER_TEST_DIR=/path/to/optimizer-test"
  exit 1
fi
TIMESTAMP=$(date +%s)
RUN_ID="${RUN_ID:-$TIMESTAMP}"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
log()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }
err()  { echo -e "${RED}[ERROR]${NC} $*"; }

run_suite() {
  local suite="$1"
  local pkg_path="$2"

  log "Running ${suite} tests..."

  local json_out cover_out
  json_out=$(mktemp)
  cover_out=$(mktemp)

  local suite_result=1
  local start_sec end_sec duration
  start_sec=$(date +%s)

  cd "$OPTIMIZER_TEST_DIR"
  if ! go test -v -json -count=1 \
      -coverprofile="$cover_out" \
      -coverpkg=intelligent-cluster-optimizer/... \
      "$pkg_path" > "$json_out" 2>&1; then
    suite_result=0
    warn "${suite} suite: some tests FAILED"
  else
    log "${suite} suite: PASSED"
  fi
  end_sec=$(date +%s)
  duration=$(( (end_sec - start_sec) * 1000 ))

  # Parse coverage
  local coverage=0
  if [[ -s "$cover_out" ]]; then
    coverage=$(go tool cover -func="$cover_out" 2>/dev/null \
      | awk '/^total:/ { gsub(/%/,"",$3); printf "%.1f", $3 }')
    [[ -z "$coverage" ]] && coverage=0
    log "${suite} coverage: ${coverage}%"
  fi

  # Parse JSON, classify by component, build Prometheus payload
  local payload
  payload=$(python3 - "$json_out" "$suite" "$suite_result" "$duration" "$TIMESTAMP" "$RUN_ID" "$coverage" <<'PYEOF'
import sys, json
from collections import defaultdict

json_file    = sys.argv[1]
suite        = sys.argv[2]
suite_result = int(sys.argv[3])
duration     = int(sys.argv[4])
timestamp    = int(sys.argv[5])
run_id       = sys.argv[6]
coverage     = float(sys.argv[7])

def get_component(name):
    n = name.lower()
    if any(k in n for k in [
        'circuitbreaker','circuit_breaker','sla','leakdetector','leak_detector',
        'oomdetector','oom_detector','hpa','pdb','safety','healthchecker',
        'health_checker','controlchart','control_chart','rollback'
    ]):
        return 'Safety'
    if any(k in n for k in [
        'holtwinters','holt_winters','workloadpredictor','workload_predictor',
        'trend','decomposition','growthrate','growth_rate','confidence',
        'timepattern','forecast','prediction','predictor'
    ]):
        return 'Statistical_Analysis'
    if any(k in n for k in ['pareto','recommendation','capacity','cost']):
        return 'Pareto_Optimization'
    if any(k in n for k in ['policy']):
        return 'Policy_Engine'
    if any(k in n for k in ['storage','backup']):
        return 'Storage'
    if any(k in n for k in ['gitops','pipeline','kustomize','helm']):
        return 'GitOps_Integration'
    return 'Other'

total = passed = failed = skipped = 0
comp_total   = defaultdict(int)
comp_passed  = defaultdict(int)
comp_failed  = defaultdict(int)

per_test_result   = []
per_test_duration = []

with open(json_file) as f:
    for line in f:
        line = line.strip()
        if not line:
            continue
        try:
            d = json.loads(line)
        except json.JSONDecodeError:
            continue
        action    = d.get('Action', '')
        test_name = d.get('Test', '')
        elapsed   = d.get('Elapsed', 0)
        if not test_name:
            continue

        safe_name = test_name.replace('/', '_').replace(' ', '_')
        comp      = get_component(test_name)

        if action == 'pass':
            total += 1; passed += 1
            comp_total[comp] += 1; comp_passed[comp] += 1
            per_test_result.append(
                f'optimizer_test_result{{suite="{suite}",component="{comp}",test="{safe_name}"}} 1')
            per_test_duration.append(
                f'optimizer_test_duration_seconds{{suite="{suite}",component="{comp}",test="{safe_name}"}} {elapsed}')
        elif action == 'fail':
            total += 1; failed += 1
            comp_total[comp] += 1; comp_failed[comp] += 1
            per_test_result.append(
                f'optimizer_test_result{{suite="{suite}",component="{comp}",test="{safe_name}"}} 0')
        elif action == 'skip':
            skipped += 1

lines = []

# Suite-level metrics
lbl = f'suite="{suite}",run_id="{run_id}"'
lines += [
    f'# HELP optimizer_suite_tests_total Total tests',
    f'# TYPE optimizer_suite_tests_total gauge',
    f'optimizer_suite_tests_total{{{lbl}}} {total}',
    f'# HELP optimizer_suite_tests_passed Passed tests',
    f'# TYPE optimizer_suite_tests_passed gauge',
    f'optimizer_suite_tests_passed{{{lbl}}} {passed}',
    f'# HELP optimizer_suite_tests_failed Failed tests',
    f'# TYPE optimizer_suite_tests_failed gauge',
    f'optimizer_suite_tests_failed{{{lbl}}} {failed}',
    f'# HELP optimizer_suite_tests_skipped Skipped tests',
    f'# TYPE optimizer_suite_tests_skipped gauge',
    f'optimizer_suite_tests_skipped{{{lbl}}} {skipped}',
    f'# HELP optimizer_suite_duration_ms Suite duration ms',
    f'# TYPE optimizer_suite_duration_ms gauge',
    f'optimizer_suite_duration_ms{{{lbl}}} {duration}',
    f'# HELP optimizer_suite_result Suite result 1=pass 0=fail',
    f'# TYPE optimizer_suite_result gauge',
    f'optimizer_suite_result{{{lbl}}} {suite_result}',
    f'# HELP optimizer_suite_last_run_timestamp Unix timestamp of last run',
    f'# TYPE optimizer_suite_last_run_timestamp gauge',
    f'optimizer_suite_last_run_timestamp{{{lbl}}} {timestamp}',
    f'# HELP optimizer_suite_coverage_percent Coverage percentage',
    f'# TYPE optimizer_suite_coverage_percent gauge',
    f'optimizer_suite_coverage_percent{{{lbl}}} {coverage}',
    f'# HELP optimizer_wp9_coverage_target WP9 success threshold (80%)',
    f'# TYPE optimizer_wp9_coverage_target gauge',
    f'optimizer_wp9_coverage_target{{suite="{suite}"}} 80',
]

# Ensure every component appears in all three dicts (push 0 when missing)
for comp in comp_total:
    comp_passed.setdefault(comp, 0)
    comp_failed.setdefault(comp, 0)

# Component-level metrics
lines += [
    '# HELP optimizer_component_tests_total Tests per component',
    '# TYPE optimizer_component_tests_total gauge',
]
for comp, val in comp_total.items():
    lines.append(f'optimizer_component_tests_total{{suite="{suite}",component="{comp}",run_id="{run_id}"}} {val}')

lines += [
    '# HELP optimizer_component_tests_passed Passed tests per component',
    '# TYPE optimizer_component_tests_passed gauge',
]
for comp, val in comp_passed.items():
    lines.append(f'optimizer_component_tests_passed{{suite="{suite}",component="{comp}",run_id="{run_id}"}} {val}')

lines += [
    '# HELP optimizer_component_tests_failed Failed tests per component',
    '# TYPE optimizer_component_tests_failed gauge',
]
for comp, val in comp_failed.items():
    lines.append(f'optimizer_component_tests_failed{{suite="{suite}",component="{comp}",run_id="{run_id}"}} {val}')

# Per-test metrics
if per_test_result:
    lines += ['# HELP optimizer_test_result Per-test result 1=pass 0=fail',
              '# TYPE optimizer_test_result gauge']
    lines += per_test_result
if per_test_duration:
    lines += ['# HELP optimizer_test_duration_seconds Per-test duration',
              '# TYPE optimizer_test_duration_seconds gauge']
    lines += per_test_duration

print('\n'.join(lines))
print(f"stats: total={total} passed={passed} failed={failed} skipped={skipped}", file=sys.stderr)
for comp in sorted(comp_total):
    print(f"  {comp}: {comp_passed.get(comp,0)}/{comp_total[comp]}", file=sys.stderr)
PYEOF
)

  # Push metrics — her suite kendi grubuna gider, birbirini silmez
  echo "$payload" | curl -s --data-binary @- \
    "${PUSHGATEWAY}/metrics/job/optimizer_tests/suite/${suite}" \
    && log "Metrics pushed" \
    || warn "Push failed"

  rm -f "$json_out" "$cover_out"
  echo ""
}

# Check Pushgateway
log "Checking Pushgateway at ${PUSHGATEWAY}..."
if ! curl -sf "${PUSHGATEWAY}/-/healthy" > /dev/null; then
  err "Pushgateway not reachable. Run: cd monitoring && docker compose up -d"
  exit 1
fi
log "Pushgateway OK"

SUITES="${1:-all}"
case "$SUITES" in
  unit)        run_suite "unit"        "./unit/..."        ;;
  integration) run_suite "integration" "./integration/..." ;;
  e2e)         run_suite "e2e"         "./e2e/..."         ;;
  all|*)
    run_suite "unit"        "./unit/..."
    run_suite "integration" "./integration/..."
    run_suite "e2e"         "./e2e/..."
    ;;
esac

log "Done. Grafana → http://localhost:3000 (admin/admin)"
