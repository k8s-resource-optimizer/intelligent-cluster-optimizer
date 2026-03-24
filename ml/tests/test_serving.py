"""
Tests for ml/serving/main.py
Run: pytest ml/tests/test_serving.py -v
"""

import json
from pathlib import Path
from typing import Generator
from unittest.mock import MagicMock, patch

import numpy as np
import pytest
from fastapi.testclient import TestClient


# ── Fixtures ───────────────────────────────────────────────────────────────────

def _make_mock_pipeline(pred_len: int = 15) -> MagicMock:
    """
    Returns a ChronosPipeline mock whose .predict() returns a mock tensor.
    We avoid importing torch here so the tests run without a GPU environment.
    The serving code does: forecast_tensor[0].cpu().numpy() — we replicate that.
    """
    rng   = np.random.default_rng(42)
    # Build a mock that supports [0].cpu().numpy() chain
    arr   = rng.uniform(0.1, 0.6, size=(20, pred_len)).astype(np.float32)

    inner = MagicMock()
    inner.cpu.return_value.numpy.return_value = arr

    outer = MagicMock()
    outer.__getitem__ = MagicMock(return_value=inner)

    pipeline = MagicMock()
    pipeline.predict.return_value = outer
    return pipeline


def _mock_torch():
    """
    Returns a sys.modules-compatible torch stub.
    Provides only what serving/main.py uses: torch.tensor().unsqueeze().
    """
    tensor_mock = MagicMock()
    tensor_mock.unsqueeze.return_value = tensor_mock   # chained .unsqueeze(0)

    torch_stub = MagicMock()
    torch_stub.tensor.return_value = tensor_mock
    return torch_stub


@pytest.fixture()
def client_with_model() -> Generator[TestClient, None, None]:
    """TestClient with a loaded (mocked) model and a stubbed torch."""
    import sys

    torch_stub = _mock_torch()
    sys.modules.setdefault("torch", torch_stub)

    from ml.serving import main as serving

    mock_pipeline = _make_mock_pipeline(pred_len=15)
    serving._state.pipeline          = mock_pipeline
    serving._state.prediction_length = 15
    serving._state.device            = "cpu"

    # Patch the lazy `import torch` inside predict() to return the stub
    with patch.dict(sys.modules, {"torch": torch_stub}):
        with TestClient(serving.app, raise_server_exceptions=True) as c:
            yield c

    serving._state.pipeline = None


@pytest.fixture()
def client_no_model() -> Generator[TestClient, None, None]:
    """TestClient with no model loaded (simulates cold start)."""
    from ml.serving import main as serving

    serving._state.pipeline = None
    with TestClient(serving.app, raise_server_exceptions=False) as c:
        yield c


def cpu_payload(n: int = 60, value: float = 0.3) -> dict:
    return {"cpu_values": [value] * n}


# ── /health ────────────────────────────────────────────────────────────────────

def test_health_always_200(client_no_model):
    resp = client_no_model.get("/health")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"


def test_health_with_model(client_with_model):
    resp = client_with_model.get("/health")
    assert resp.status_code == 200


# ── /ready ─────────────────────────────────────────────────────────────────────

def test_ready_without_model_503(client_no_model):
    resp = client_no_model.get("/ready")
    assert resp.status_code == 503


def test_ready_with_model_200(client_with_model):
    resp = client_with_model.get("/ready")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ready"


# ── /predict — happy path ──────────────────────────────────────────────────────

def test_predict_returns_15_steps(client_with_model):
    resp = client_with_model.post("/predict", json=cpu_payload(60, 0.4))
    assert resp.status_code == 200
    body = resp.json()
    assert len(body["forecast"]) == 15
    assert body["prediction_length"] == 15
    assert body["context_length"] == 60


def test_predict_forecast_points_structure(client_with_model):
    resp = client_with_model.post("/predict", json=cpu_payload(60, 0.3))
    body = resp.json()
    for i, pt in enumerate(body["forecast"]):
        assert pt["step"] == i + 1
        assert 0.0 <= pt["low"]    <= 1.0, f"step {i+1} low out of range"
        assert 0.0 <= pt["median"] <= 1.0, f"step {i+1} median out of range"
        assert 0.0 <= pt["high"]   <= 1.0, f"step {i+1} high out of range"
        assert pt["low"] <= pt["median"] <= pt["high"]


def test_predict_with_min_samples(client_with_model):
    resp = client_with_model.post("/predict", json=cpu_payload(30, 0.2))
    assert resp.status_code == 200


def test_predict_with_full_lookback(client_with_model):
    """60 samples = exactly one context window."""
    resp = client_with_model.post("/predict", json=cpu_payload(60, 0.5))
    assert resp.status_code == 200


def test_predict_inference_ms_present(client_with_model):
    resp = client_with_model.post("/predict", json=cpu_payload())
    assert resp.json()["inference_ms"] >= 0


# ── /predict — validation errors ──────────────────────────────────────────────

def test_predict_too_few_samples_422(client_with_model):
    resp = client_with_model.post("/predict", json=cpu_payload(10))
    assert resp.status_code == 422


def test_predict_value_above_1_422(client_with_model):
    """Values must be in [0, 1] — percentages (e.g. 75.0) are rejected."""
    resp = client_with_model.post("/predict", json={"cpu_values": [75.0] * 60})
    assert resp.status_code == 422


def test_predict_negative_value_422(client_with_model):
    resp = client_with_model.post("/predict", json={"cpu_values": [-0.1] * 60})
    assert resp.status_code == 422


def test_predict_no_model_503(client_no_model):
    resp = client_no_model.post("/predict", json=cpu_payload())
    assert resp.status_code == 503


def test_predict_missing_body_422(client_with_model):
    resp = client_with_model.post("/predict", json={})
    assert resp.status_code == 422


# ── /metrics ───────────────────────────────────────────────────────────────────

def test_metrics_endpoint(client_with_model):
    # Trigger a prediction first so counters are non-zero
    client_with_model.post("/predict", json=cpu_payload())
    resp = client_with_model.get("/metrics")
    assert resp.status_code == 200
    assert "forecast_requests_total" in resp.text
    assert "forecast_inference_seconds" in resp.text
