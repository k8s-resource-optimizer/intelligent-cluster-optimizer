"""
Tests for Azra's data pipeline output (windows.npz).
Verifies shape, dtype, and value range before fine-tuning.

Run: pytest ml/tests/test_data_pipeline.py -v
"""

from pathlib import Path

import numpy as np
import pandas as pd
import pytest

REPO_ROOT   = Path(__file__).parent.parent.parent   # repo root
DATA_DIR    = REPO_ROOT / "ml" / "data"
WINDOWS_NPZ = DATA_DIR / "windows.npz"
SEED_CSV    = DATA_DIR / "seed_cpu_metrics.csv"
SYNTH_CSV   = DATA_DIR / "synthetic_cpu_metrics.csv"

LOOKBACK  = 60
FORECAST  = 15


# ── windows.npz ────────────────────────────────────────────────────────────────

@pytest.fixture(scope="module")
def windows():
    assert WINDOWS_NPZ.exists(), f"Run sliding_window.py first: {WINDOWS_NPZ}"
    data = np.load(WINDOWS_NPZ)
    return data["X"], data["y"]


def test_windows_exist():
    assert WINDOWS_NPZ.exists()


def test_windows_shapes(windows):
    X, y = windows
    assert X.ndim == 2, "X should be 2D (samples, lookback)"
    assert y.ndim == 2, "y should be 2D (samples, forecast)"
    assert X.shape[1] == LOOKBACK,  f"Expected lookback={LOOKBACK}, got {X.shape[1]}"
    assert y.shape[1] == FORECAST,  f"Expected forecast={FORECAST}, got {y.shape[1]}"
    assert X.shape[0] == y.shape[0], "X and y must have the same number of samples"


def test_windows_dtype(windows):
    X, y = windows
    assert X.dtype == np.float32, f"Expected float32, got {X.dtype}"
    assert y.dtype == np.float32


def test_windows_value_range(windows):
    X, y = windows
    assert X.min() >= 0.0, "X values must be ≥ 0"
    assert X.max() <= 1.0, f"X values must be ≤ 1.0, got max={X.max()}"
    assert y.min() >= 0.0
    assert y.max() <= 1.0, f"y values must be ≤ 1.0, got max={y.max()}"


def test_windows_no_nan(windows):
    X, y = windows
    assert not np.isnan(X).any(), "X contains NaN"
    assert not np.isnan(y).any(), "y contains NaN"


def test_windows_reasonable_sample_count(windows):
    X, _ = windows
    # Expect at least 40k windows from 50k synthetic rows
    assert X.shape[0] >= 40_000, f"Expected ≥ 40k windows, got {X.shape[0]}"


def test_windows_continuity(windows):
    """Consecutive windows should overlap by LOOKBACK-1 steps."""
    X, _ = windows
    if X.shape[0] < 2:
        pytest.skip("Not enough windows to check continuity")
    # X[1] should equal X[0][1:] for a proper stride-1 sliding window
    np.testing.assert_array_equal(
        X[1, :LOOKBACK - 1],
        X[0, 1:LOOKBACK],
        err_msg="Windows are not stride-1 consecutive",
    )


# ── seed CSV ───────────────────────────────────────────────────────────────────

@pytest.fixture(scope="module")
def seed_df():
    assert SEED_CSV.exists(), f"Run export_seed_csv.py first: {SEED_CSV}"
    return pd.read_csv(SEED_CSV)


def test_seed_columns(seed_df):
    assert set(["timestamp", "cpu", "mode", "value"]).issubset(seed_df.columns)


def test_seed_value_range(seed_df):
    assert seed_df["value"].between(0.0, 1.0).all(), \
        "Seed CPU values should be in [0, 1]"


# ── synthetic CSV ──────────────────────────────────────────────────────────────

@pytest.fixture(scope="module")
def synth_df():
    assert SYNTH_CSV.exists(), f"Run generate_synthetic_data.py first: {SYNTH_CSV}"
    return pd.read_csv(SYNTH_CSV)


def test_synthetic_row_count(synth_df):
    assert len(synth_df) == 50_000, f"Expected 50k rows, got {len(synth_df)}"


def test_synthetic_value_range(synth_df):
    assert synth_df["value"].between(0.0, 1.0).all()


def test_synthetic_has_spikes(synth_df):
    """With SPIKE_PROB=0.02, expect at least a few hundred values above 0.5."""
    spike_count = (synth_df["value"] > 0.5).sum()
    assert spike_count >= 100, f"Expected spikes, got only {spike_count}"
