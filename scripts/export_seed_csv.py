#!/usr/bin/env python3

import csv
import datetime
import json
import sys
import urllib.request

PROMETHEUS_URL = "http://localhost:9090"
DURATION_MINUTES = 60
STEP_SECONDS = 60


def fetch_cpu_metrics():
    end = datetime.datetime.now(datetime.UTC)
    start = end - datetime.timedelta(minutes=DURATION_MINUTES)

    # Exclude idle mode to capture only active CPU usage
    query = 'rate(node_cpu_seconds_total{mode!="idle"}[1m])'
    params = (
        f"query={urllib.request.quote(query)}"
        f"&start={start.strftime('%Y-%m-%dT%H:%M:%SZ')}"
        f"&end={end.strftime('%Y-%m-%dT%H:%M:%SZ')}"
        f"&step={STEP_SECONDS}"
    )
    url = f"{PROMETHEUS_URL}/api/v1/query_range?{params}"

    with urllib.request.urlopen(url) as resp:
        return json.load(resp)


def export_csv(data, output_path):
    results = data.get("data", {}).get("result", [])
    if not results:
        print("ERROR: No data returned.")
        sys.exit(1)

    rows = []
    for series in results:
        cpu = series["metric"].get("cpu", "?")
        mode = series["metric"].get("mode", "?")
        for timestamp, value in series["values"]:
            dt = datetime.datetime.fromtimestamp(float(timestamp), datetime.UTC)
            rows.append({
                "timestamp": dt.strftime("%Y-%m-%d %H:%M:%S"),
                "cpu": cpu,
                "mode": mode,
                "value": float(value),
            })

    rows.sort(key=lambda r: r["timestamp"])

    with open(output_path, "w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=["timestamp", "cpu", "mode", "value"])
        writer.writeheader()
        writer.writerows(rows)

    print(f"Done: {len(rows)} rows written -> {output_path}")


if __name__ == "__main__":
    output = "scripts/seed_cpu_metrics.csv"
    data = fetch_cpu_metrics()
    export_csv(data, output)
