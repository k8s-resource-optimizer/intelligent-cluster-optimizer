"""
Task 8: Stress-Testing — Anomaly Generation & Forecast Validation
==================================================================
Hammers CPU to create real anomaly moments, then validates the
forecasting service detects the spike.

Modes:
  local   — burns CPU via threads (no cluster needed, works on Mac)
  cluster — deploys a stress Job to Kubernetes via kubectl

Values sent to /predict are in 0-1 range (matching Azra's pipeline).

Usage:
    # Local stress + forecast validation
    python ml/tests/stress_test.py --mode local --duration 120 \
        --validate --forecast-url http://localhost:8080

    # Cluster stress (Kind)
    python ml/tests/stress_test.py --mode cluster --namespace default --duration 60
"""

import argparse
import json
import multiprocessing
import statistics
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Optional


# ── Kubernetes stress Job template ─────────────────────────────────────────────
_JOB_YAML = """
apiVersion: batch/v1
kind: Job
metadata:
  name: cpu-stress-{suffix}
  namespace: {namespace}
  labels:
    app: cluster-stress-test
spec:
  parallelism: {parallelism}
  completions: {parallelism}
  activeDeadlineSeconds: {duration}
  template:
    spec:
      restartPolicy: Never
      containers:
      - name: stress
        image: progrium/stress
        args: ["--cpu", "{cpu_workers}", "--timeout", "{duration}s"]
        resources:
          requests: {{cpu: "500m", memory: "64Mi"}}
          limits:   {{cpu: "1000m", memory: "128Mi"}}
"""


@dataclass
class StressResult:
    mode:               str
    duration_s:         int
    cpu_samples:        list[float] = field(default_factory=list)
    anomalies_detected: int         = 0
    forecast_ok:        bool        = False
    errors:             list[str]   = field(default_factory=list)

    @property
    def peak_cpu(self) -> float:
        return max(self.cpu_samples) if self.cpu_samples else 0.0

    @property
    def mean_cpu(self) -> float:
        return statistics.mean(self.cpu_samples) if self.cpu_samples else 0.0

    def summary(self) -> str:
        return (
            f"mode={self.mode}  duration={self.duration_s}s  "
            f"samples={len(self.cpu_samples)}  "
            f"mean={self.mean_cpu:.3f}  peak={self.peak_cpu:.3f}  "
            f"anomalies={self.anomalies_detected}  "
            f"forecast_ok={self.forecast_ok}  errors={len(self.errors)}"
        )


# ── Local CPU burn ─────────────────────────────────────────────────────────────

def _burn(stop: threading.Event) -> None:
    while not stop.is_set():
        _ = sum(i * i for i in range(10_000))


def _sample_cpu_fraction() -> float:
    """Returns CPU usage as 0-1 fraction (matches Azra's 0-1 value range)."""
    # macOS: parse `top -l 1`
    try:
        out = subprocess.check_output(
            ["top", "-l", "1", "-n", "0", "-s", "0"],
            stderr=subprocess.DEVNULL, timeout=5,
        ).decode()
        for line in out.splitlines():
            if "CPU usage" in line:
                parts = line.split(",")
                user = float(parts[0].split(":")[1].strip().replace("% user", ""))
                sys_ = float(parts[1].strip().replace("% sys", ""))
                return round((user + sys_) / 100.0, 4)
    except Exception:
        pass

    # Linux: /proc/stat
    try:
        with open("/proc/stat") as f:
            vals = list(map(int, f.readline().split()[1:]))
        idle = vals[3]
        return round(1.0 - idle / sum(vals), 4)
    except Exception:
        pass

    return 0.0


def run_local_stress(duration: int, workers: Optional[int] = None) -> list[float]:
    workers = workers or multiprocessing.cpu_count()
    print(f"[local] Burning {workers} CPU threads for {duration}s …")

    stop    = threading.Event()
    threads = [threading.Thread(target=_burn, args=(stop,), daemon=True) for _ in range(workers)]
    for t in threads:
        t.start()

    samples: list[float] = []
    deadline = time.time() + duration
    while time.time() < deadline:
        v = _sample_cpu_fraction()
        samples.append(v)
        ts = datetime.now(timezone.utc).strftime("%H:%M:%S")
        print(f"  [{ts}] CPU: {v:.3f}", end="\r")
        time.sleep(1)

    stop.set()
    for t in threads:
        t.join(timeout=3)
    print(f"\n[local] Done — {len(samples)} samples collected.")
    return samples


# ── Cluster stress ─────────────────────────────────────────────────────────────

def run_cluster_stress(
    namespace:   str,
    duration:    int,
    parallelism: int = 3,
    cpu_workers: int = 2,
) -> list[float]:
    import random
    suffix = f"{int(time.time())}-{random.randint(1000,9999)}"
    yaml   = _JOB_YAML.format(
        suffix=suffix, namespace=namespace,
        parallelism=parallelism, duration=duration, cpu_workers=cpu_workers,
    )
    print(f"[cluster] Deploying cpu-stress-{suffix} …")
    r = subprocess.run(["kubectl", "apply", "-f", "-"], input=yaml.encode(), capture_output=True)
    if r.returncode != 0:
        raise RuntimeError(f"kubectl apply failed: {r.stderr.decode()}")

    samples: list[float] = []
    deadline = time.time() + duration + 5
    try:
        while time.time() < deadline:
            out = subprocess.run(["kubectl", "top", "nodes", "--no-headers"],
                                 capture_output=True, timeout=10)
            if out.returncode == 0:
                for line in out.stdout.decode().splitlines():
                    parts = line.split()
                    if len(parts) >= 3:
                        try:
                            # kubectl top returns integer percentage, convert to 0-1
                            samples.append(float(parts[2].replace("%", "")) / 100.0)
                        except ValueError:
                            pass
            time.sleep(5)
    finally:
        subprocess.run(
            ["kubectl", "delete", "job", f"cpu-stress-{suffix}", "-n", namespace, "--ignore-not-found"],
            capture_output=True,
        )
        print("[cluster] Stress Job deleted.")
    return samples


# ── Forecast validation ────────────────────────────────────────────────────────

def validate_forecast(
    samples:          list[float],
    forecast_url:     str,
    anomaly_threshold: float = 0.75,   # 0-1 range, 0.75 = 75% CPU
) -> tuple[bool, int]:
    """POST recent samples to /predict and count anomalous forecast steps."""
    if len(samples) < 30:
        print("[validate] Need ≥ 30 samples, skipping.")
        return False, 0

    payload = json.dumps({"cpu_values": samples[-60:]}).encode()
    req     = urllib.request.Request(
        url     = forecast_url.rstrip("/") + "/predict",
        data    = payload,
        headers = {"Content-Type": "application/json"},
        method  = "POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            body = json.loads(resp.read())
    except urllib.error.HTTPError as e:
        print(f"[validate] HTTP {e.code}: {e.read().decode()}")
        return False, 0
    except Exception as e:
        print(f"[validate] Request failed: {e}")
        return False, 0

    forecast  = body.get("forecast", [])
    anomalies = sum(1 for pt in forecast if pt["median"] > anomaly_threshold)
    peak_p90  = max((pt["high"] for pt in forecast), default=0)

    print(f"[validate] Steps={len(forecast)}  anomalies (median>{anomaly_threshold})={anomalies}  peak_p90={peak_p90:.3f}")
    return True, anomalies


# ── CLI ────────────────────────────────────────────────────────────────────────

def main() -> None:
    p = argparse.ArgumentParser(description="CPU stress test + forecast validation")
    p.add_argument("--mode",          choices=["local", "cluster"], default="local")
    p.add_argument("--duration",      type=int,   default=120)
    p.add_argument("--workers",       type=int,   default=None)
    p.add_argument("--namespace",     type=str,   default="default")
    p.add_argument("--parallelism",   type=int,   default=3)
    p.add_argument("--validate",      action="store_true")
    p.add_argument("--forecast-url",  type=str,   default="http://localhost:8080")
    p.add_argument("--threshold",     type=float, default=0.75,
                   help="CPU fraction anomaly threshold (0-1, default 0.75)")
    args = p.parse_args()

    result = StressResult(mode=args.mode, duration_s=args.duration)

    print(f"\n{'='*55}\n  Stress Test | mode={args.mode} | duration={args.duration}s\n{'='*55}\n")

    try:
        if args.mode == "local":
            result.cpu_samples = run_local_stress(args.duration, args.workers)
        else:
            result.cpu_samples = run_cluster_stress(args.namespace, args.duration, args.parallelism)
    except Exception as e:
        result.errors.append(str(e))
        print(f"[ERROR] {e}", file=sys.stderr)

    if args.validate and result.cpu_samples:
        ok, n = validate_forecast(result.cpu_samples, args.forecast_url, args.threshold)
        result.forecast_ok        = ok
        result.anomalies_detected = n

    print(f"\n{'='*55}\n  RESULT: {result.summary()}\n{'='*55}\n")
    if result.errors:
        sys.exit(1)


if __name__ == "__main__":
    main()
