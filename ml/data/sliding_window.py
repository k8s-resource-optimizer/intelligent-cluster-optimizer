#!/usr/bin/env python3

import numpy as np
import pandas as pd

INPUT_CSV = "ml/data/synthetic_cpu_metrics.csv"
OUTPUT_NPZ = "ml/data/windows.npz"
LOOKBACK = 60   # minutes of history fed to the model
FORECAST = 15   # minutes ahead the model predicts


def load_and_aggregate(path: str) -> np.ndarray:
    df = pd.read_csv(path, parse_dates=["timestamp"])
    # Sum all CPU cores and modes per timestamp to get total CPU load
    agg = df.groupby("timestamp")["value"].sum().sort_index()
    return agg.to_numpy(dtype=np.float32)


def create_windows(series: np.ndarray, lookback: int, forecast: int):
    x_list, y_list = [], []
    total = lookback + forecast
    for i in range(len(series) - total + 1):
        x_list.append(series[i : i + lookback])
        y_list.append(series[i + lookback : i + lookback + forecast])
    return np.array(x_list), np.array(y_list)


def main():
    series = load_and_aggregate(INPUT_CSV)
    print(f"Aggregated series length: {len(series)} timesteps")

    X, y = create_windows(series, LOOKBACK, FORECAST)
    print(f"Windows created: {X.shape[0]} samples")
    print(f"  X shape: {X.shape}  (samples x lookback)")
    print(f"  y shape: {y.shape}  (samples x forecast)")

    np.savez_compressed(OUTPUT_NPZ, X=X, y=y)
    print(f"Done -> {OUTPUT_NPZ}")


if __name__ == "__main__":
    main()
