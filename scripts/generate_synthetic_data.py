#!/usr/bin/env python3

import datetime
import numpy as np
import pandas as pd

SEED_CSV = "scripts/seed_cpu_metrics.csv"
OUTPUT_CSV = "scripts/synthetic_cpu_metrics.csv"
TARGET_ROWS = 50_000
NOISE_STD = 0.01       # Standard deviation for Gaussian noise
SPIKE_PROB = 0.02      # Probability of a spike at any given point
SPIKE_MAGNITUDE = 0.7  # How much a spike adds to the value
RANDOM_SEED = 42


def load_seed(path: str) -> pd.DataFrame:
    df = pd.read_csv(path, parse_dates=["timestamp"])
    df = df.sort_values("timestamp").reset_index(drop=True)
    return df


def add_noise(values: np.ndarray, rng: np.random.Generator) -> np.ndarray:
    noise = rng.normal(loc=0.0, scale=NOISE_STD, size=len(values))
    return np.clip(values + noise, 0.0, 1.0)


def add_spikes(values: np.ndarray, rng: np.random.Generator) -> np.ndarray:
    mask = rng.random(len(values)) < SPIKE_PROB
    spikes = mask * rng.uniform(SPIKE_MAGNITUDE * 0.5, SPIKE_MAGNITUDE, size=len(values))
    return np.clip(values + spikes, 0.0, 1.0)


def generate(seed_df: pd.DataFrame, target_rows: int) -> pd.DataFrame:
    rng = np.random.default_rng(RANDOM_SEED)

    # Repeat seed data enough times to reach target row count
    repeats = -(-target_rows // len(seed_df))  # ceiling division
    base = pd.concat([seed_df] * repeats, ignore_index=True).iloc[:target_rows]

    # Recalculate timestamps as continuous 1-minute intervals
    start = base["timestamp"].iloc[0]
    base["timestamp"] = [
        start + datetime.timedelta(minutes=i) for i in range(len(base))
    ]

    values = base["value"].to_numpy(dtype=float)
    values = add_noise(values, rng)
    values = add_spikes(values, rng)
    base["value"] = np.round(values, 6)

    return base


def main():
    seed_df = load_seed(SEED_CSV)
    print(f"Seed rows: {len(seed_df)}")

    synthetic_df = generate(seed_df, TARGET_ROWS)
    synthetic_df.to_csv(OUTPUT_CSV, index=False)
    print(f"Done: {len(synthetic_df)} rows written -> {OUTPUT_CSV}")

    # Quick stats
    print(f"  value min={synthetic_df['value'].min():.4f}  "
          f"max={synthetic_df['value'].max():.4f}  "
          f"mean={synthetic_df['value'].mean():.4f}")
    spike_count = (synthetic_df["value"] > 0.5).sum()
    print(f"  spike-like points (value > 0.5): {spike_count}")


if __name__ == "__main__":
    main()
