"""
Task 7: FastAPI Inference Server
==================================
Serves the fine-tuned Chronos-2 model for CPU-usage forecasting.

Values are in 0-1 range (matching Azra's data pipeline).

Endpoints:
    GET  /health    — liveness
    GET  /ready     — readiness (model loaded?)
    POST /predict   — forecast next 15 minutes of CPU usage
    GET  /metrics   — Prometheus metrics

Usage:
    uvicorn ml.serving.main:app --host 0.0.0.0 --port 8080
"""

import json
import time
from contextlib import asynccontextmanager
from pathlib import Path
from typing import TYPE_CHECKING, Any, Optional

import structlog
import numpy as np
from fastapi import FastAPI, HTTPException, status
from fastapi.responses import PlainTextResponse
from prometheus_client import Counter, Gauge, Histogram, generate_latest, CONTENT_TYPE_LATEST
from pydantic import BaseModel, Field, field_validator

if TYPE_CHECKING:  # pragma: no cover
    from chronos import ChronosPipeline

# ── Paths ──────────────────────────────────────────────────────────────────────
REPO_ROOT  = Path(__file__).parent.parent        # ml/
MODELS_DIR = REPO_ROOT / "models" / "chronos-finetuned"

# ── Logging ────────────────────────────────────────────────────────────────────
structlog.configure(
    processors=[
        structlog.processors.TimeStamper(fmt="iso"),
        structlog.processors.add_log_level,
        structlog.processors.JSONRenderer(),
    ]
)
log = structlog.get_logger()

# ── Prometheus ─────────────────────────────────────────────────────────────────
REQUEST_COUNT = Counter("forecast_requests_total", "Total /predict requests", ["status"])
INFERENCE_LAT = Histogram(
    "forecast_inference_seconds", "Inference latency",
    buckets=[0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.0],
)
MODEL_LOADED  = Gauge("forecast_model_loaded", "1 if model is ready")


# ── Model state ────────────────────────────────────────────────────────────────
class _State:
    pipeline:          Optional[Any] = None   # ChronosPipeline at runtime
    prediction_length: int           = 15     # Azra's FORECAST
    device:            str           = "cpu"

_state = _State()


# ── Schemas ────────────────────────────────────────────────────────────────────
class PredictRequest(BaseModel):
    """
    cpu_values: recent CPU usage samples in 0-1 range (matches Azra's data format).
                Minimum 30 values; recommended ≥ 60 (full lookback window).
    """
    cpu_values:  list[float] = Field(..., min_length=30)
    num_samples: int         = Field(default=20, ge=1, le=100)

    @field_validator("cpu_values")
    @classmethod
    def validate_range(cls, v: list[float]) -> list[float]:
        if any(x < 0.0 or x > 1.0 for x in v):
            raise ValueError("cpu_values must be in [0.0, 1.0] — use fractions, not percentages")
        return v


class ForecastPoint(BaseModel):
    step:   int    # 1-indexed future minute
    low:    float  # p10
    median: float  # p50
    high:   float  # p90


class PredictResponse(BaseModel):
    forecast:          list[ForecastPoint]
    context_length:    int
    prediction_length: int
    inference_ms:      float


# ── Startup ────────────────────────────────────────────────────────────────────
@asynccontextmanager
async def lifespan(app: FastAPI):
    _load_model()
    yield


def _load_model() -> None:
    if not MODELS_DIR.exists():
        log.warning("model_dir_missing", path=str(MODELS_DIR),
                    hint="Run ml/training/finetune.py on Colab first, then copy the checkpoint here.")
        MODEL_LOADED.set(0)
        return

    tok_cfg_path = MODELS_DIR / "tokenizer_config.json"
    if tok_cfg_path.exists():
        cfg = json.loads(tok_cfg_path.read_text())
        _state.prediction_length = cfg.get("prediction_length", 15)

    import torch
    from chronos import ChronosPipeline

    device = (
        "cuda" if torch.cuda.is_available()
        else "mps" if torch.backends.mps.is_available()
        else "cpu"
    )
    _state.device   = device
    _state.pipeline = ChronosPipeline.from_pretrained(
        str(MODELS_DIR),
        device_map  = device,
        torch_dtype = torch.float32,
    )
    MODEL_LOADED.set(1)
    log.info("model_loaded", device=device, prediction_length=_state.prediction_length)


# ── App ────────────────────────────────────────────────────────────────────────
app = FastAPI(
    title       = "Cluster CPU Forecaster",
    description = "Chronos-2 fine-tuned on Kubernetes CPU metrics (0-1 range)",
    version     = "1.0.0",
    lifespan    = lifespan,
)


@app.get("/health")
def health() -> dict:
    return {"status": "ok"}


@app.get("/ready")
def ready() -> dict:
    if _state.pipeline is None:
        raise HTTPException(status.HTTP_503_SERVICE_UNAVAILABLE, "Model not loaded")
    return {"status": "ready", "device": _state.device}


@app.post("/predict", response_model=PredictResponse)
def predict(req: PredictRequest) -> PredictResponse:
    if _state.pipeline is None:
        REQUEST_COUNT.labels(status="error").inc()
        raise HTTPException(status.HTTP_503_SERVICE_UNAVAILABLE, "Model not loaded")

    import torch

    t0      = time.perf_counter()
    context = torch.tensor(req.cpu_values, dtype=torch.float32).unsqueeze(0)  # (1, T)

    with INFERENCE_LAT.time():
        forecast_tensor = _state.pipeline.predict(
            context           = context,
            prediction_length = _state.prediction_length,
            num_samples       = req.num_samples,
        )

    samples = forecast_tensor[0].cpu().numpy()   # (num_samples, pred_len)
    pts = [
        ForecastPoint(
            step   = i + 1,
            low    = float(np.clip(np.percentile(samples[:, i], 10), 0, 1)),
            median = float(np.clip(np.percentile(samples[:, i], 50), 0, 1)),
            high   = float(np.clip(np.percentile(samples[:, i], 90), 0, 1)),
        )
        for i in range(_state.prediction_length)
    ]

    elapsed_ms = (time.perf_counter() - t0) * 1000
    REQUEST_COUNT.labels(status="ok").inc()
    log.info("predicted", context_len=len(req.cpu_values),
             peak_p90=max(p.high for p in pts), inference_ms=round(elapsed_ms, 1))

    return PredictResponse(
        forecast          = pts,
        context_length    = len(req.cpu_values),
        prediction_length = _state.prediction_length,
        inference_ms      = round(elapsed_ms, 1),
    )


@app.get("/metrics", response_class=PlainTextResponse)
def metrics() -> PlainTextResponse:
    return PlainTextResponse(generate_latest().decode(), media_type=CONTENT_TYPE_LATEST)
