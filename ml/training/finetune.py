"""
Task 6: Chronos-2 Fine-Tuning Script (Google Colab)
=====================================================
Fine-tunes amazon/chronos-t5-small on Azra's sliding-window CPU data.

Input:  ml/data/windows.npz
         - X: float32  (N, 60)  — 60-min CPU context  (values 0-1)
         - y: float32  (N, 15)  — 15-min future CPU    (values 0-1)

Output: ml/models/chronos-finetuned/  — HuggingFace-compatible checkpoint

Colab setup:
    !pip install -q chronos-forecasting==0.1.0 transformers accelerate
    !python ml/training/finetune.py
"""

import json
import logging
from dataclasses import dataclass
from pathlib import Path

import numpy as np
import torch
from torch.utils.data import Dataset, random_split
from transformers import (
    EarlyStoppingCallback,
    Trainer,
    TrainingArguments,
)
from chronos import ChronosPipeline, ChronosConfig, MeanScaleUniformBins

# ── Paths ──────────────────────────────────────────────────────────────────────
REPO_ROOT   = Path(__file__).parent.parent          # ml/
DATA_DIR    = REPO_ROOT / "data"
MODELS_DIR  = REPO_ROOT / "models" / "chronos-finetuned"
WINDOWS_NPZ = DATA_DIR / "windows.npz"

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger(__name__)


# ── Config ─────────────────────────────────────────────────────────────────────
@dataclass
class Cfg:
    pretrained:        str   = "amazon/chronos-t5-small"
    context_length:    int   = 60    # matches Azra's LOOKBACK
    prediction_length: int   = 15    # matches Azra's FORECAST
    n_tokens:          int   = 4096

    epochs:            int   = 10
    batch_size:        int   = 32
    lr:                float = 1e-4
    weight_decay:      float = 0.01
    warmup_ratio:      float = 0.05
    val_split:         float = 0.1
    seed:              int   = 42
    fp16:              bool  = False


CFG = Cfg()


# ── Dataset ────────────────────────────────────────────────────────────────────
class WindowDataset(Dataset):
    """Wraps X/y arrays from windows.npz as (past_values, future_values) dicts."""

    def __init__(self, X: np.ndarray, y: np.ndarray):
        self.X = torch.tensor(X, dtype=torch.float32)
        self.y = torch.tensor(y, dtype=torch.float32)

    def __len__(self) -> int:
        return len(self.X)

    def __getitem__(self, idx: int) -> dict:
        return {"past_values": self.X[idx], "future_values": self.y[idx]}


# ── Tokeniser ──────────────────────────────────────────────────────────────────
def build_tokenizer(cfg: Cfg) -> MeanScaleUniformBins:
    """MeanScaleUniformBins: maps real CPU values (0-1) → quantile bin IDs."""
    return MeanScaleUniformBins(
        low_limit=-1.001,
        high_limit=1.001,
        config=ChronosConfig(
            tokenizer_class   = "MeanScaleUniformBins",
            tokenizer_kwargs  = {"low_limit": -1.001, "high_limit": 1.001},
            context_length    = cfg.context_length,
            prediction_length = cfg.prediction_length,
            n_tokens          = cfg.n_tokens,
            n_special_tokens  = 2,
            pad_token_id      = 0,
            eos_token_id      = 1,
            use_eos_token     = True,
            model_type        = "seq2seq",
            num_samples       = 20,
            temperature       = 1.0,
            top_k             = 50,
            top_p             = 1.0,
        ),
    )


# ── Collator ───────────────────────────────────────────────────────────────────
class Collator:
    def __init__(self, tokenizer: MeanScaleUniformBins):
        self.tok = tokenizer

    def __call__(self, batch: list[dict]) -> dict:
        past   = torch.stack([b["past_values"]   for b in batch])
        future = torch.stack([b["future_values"] for b in batch])

        past_ids, past_mask, scales = self.tok.context_input_transform(past)
        future_ids, _               = self.tok.label_input_transform(future, scales)

        return {
            "input_ids":      past_ids,
            "attention_mask": past_mask,
            "labels":         future_ids,
        }


# ── Main ───────────────────────────────────────────────────────────────────────
def main(cfg: Cfg = CFG) -> None:
    torch.manual_seed(cfg.seed)
    np.random.seed(cfg.seed)
    MODELS_DIR.mkdir(parents=True, exist_ok=True)

    # 1. Load Azra's windows
    if not WINDOWS_NPZ.exists():
        raise FileNotFoundError(
            f"{WINDOWS_NPZ} not found — run ml/data/sliding_window.py first."
        )
    data = np.load(WINDOWS_NPZ)
    X, y = data["X"], data["y"]
    log.info("Loaded windows: X=%s  y=%s  value_range=[%.3f, %.3f]",
             X.shape, y.shape, float(X.min()), float(X.max()))

    # 2. Split
    dataset    = WindowDataset(X, y)
    val_size   = max(1, int(len(dataset) * cfg.val_split))
    train_size = len(dataset) - val_size
    train_ds, val_ds = random_split(
        dataset, [train_size, val_size],
        generator=torch.Generator().manual_seed(cfg.seed),
    )
    log.info("Split → train=%d  val=%d", train_size, val_size)

    # 3. Tokenizer & collator
    tokenizer = build_tokenizer(cfg)
    collator  = Collator(tokenizer)

    # 4. Load backbone
    log.info("Loading %s …", cfg.pretrained)
    pipeline = ChronosPipeline.from_pretrained(
        cfg.pretrained,
        device_map = "auto",
        dtype = torch.float16 if cfg.fp16 else torch.float32,
    )
    # pipeline.model is ChronosModel (inference wrapper).
    # pipeline.model.model is the underlying T5ForConditionalGeneration
    # which accepts the standard (input_ids, attention_mask, labels) signature
    # expected by HuggingFace Trainer.
    model = pipeline.model.model

    # 5. Train
    args = TrainingArguments(
        output_dir                  = str(MODELS_DIR),
        num_train_epochs            = cfg.epochs,
        per_device_train_batch_size = cfg.batch_size,
        per_device_eval_batch_size  = cfg.batch_size,
        learning_rate               = cfg.lr,
        weight_decay                = cfg.weight_decay,
        warmup_ratio                = cfg.warmup_ratio,
        eval_strategy               = "epoch",
        save_strategy               = "epoch",
        load_best_model_at_end      = True,
        metric_for_best_model       = "eval_loss",
        greater_is_better           = False,
        fp16                        = cfg.fp16,
        remove_unused_columns       = False,
        dataloader_num_workers      = 2,
        logging_steps               = 50,
        report_to                   = "none",
        seed                        = cfg.seed,
    )

    trainer = Trainer(
        model         = model,
        args          = args,
        train_dataset = train_ds,
        eval_dataset  = val_ds,
        data_collator = collator,
        callbacks     = [EarlyStoppingCallback(early_stopping_patience=3)],
    )

    log.info("Training …")
    trainer.train()

    # 6. Save checkpoint + tokenizer config
    trainer.save_model(str(MODELS_DIR))
    tok_meta = {
        "context_length":    cfg.context_length,
        "prediction_length": cfg.prediction_length,
        "n_tokens":          cfg.n_tokens,
        "value_range":       "0-1",   # important for serving
    }
    (MODELS_DIR / "tokenizer_config.json").write_text(
        json.dumps(tok_meta, indent=2)
    )
    log.info("Saved to %s  best_eval_loss=%.4f",
             MODELS_DIR,
             trainer.state.best_metric or float("nan"))


if __name__ == "__main__":
    main()
