"""Subprocess wrappers around the `affectus` CLI."""

from __future__ import annotations

import json
import subprocess
from typing import Mapping


def _common_args(state_path: str, config_path: str | None) -> list[str]:
    args = ["affectus"]
    if config_path:
        args += ["--config", config_path]
    args += ["--state", state_path]
    return args


def affectus_show(state_path: str, config_path: str | None = None) -> str:
    """Return the current emotion state rendered as a short Japanese fragment."""
    cmd = _common_args(state_path, config_path) + ["show"]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"affectus show failed: {result.stderr.strip()}")
    return result.stdout.strip()


def affectus_feel(
    deltas: Mapping[str, float],
    state_path: str,
    config_path: str | None = None,
) -> str:
    """Apply self-reported emotion deltas and return the rendered new state."""
    cmd = _common_args(state_path, config_path) + ["feel", json.dumps(dict(deltas))]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"affectus feel failed: {result.stderr.strip()}")
    return result.stdout.strip()


def affectus_reset(state_path: str, config_path: str | None = None) -> None:
    """Reset the state file to all-zero baseline."""
    cmd = _common_args(state_path, config_path) + ["reset"]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"affectus reset failed: {result.stderr.strip()}")
