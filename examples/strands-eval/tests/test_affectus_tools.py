import json
from unittest.mock import patch, MagicMock

import pytest

from src.affectus_tools import affectus_show, affectus_feel, affectus_reset


@patch("src.affectus_tools.subprocess.run")
def test_affectus_show_returns_stripped_stdout(mock_run):
    mock_run.return_value = MagicMock(returncode=0, stdout="いまは穏やかで、心は凪いでいる。\n", stderr="")
    out = affectus_show(state_path="/tmp/s.json", config_path="/tmp/c.yaml")
    assert out == "いまは穏やかで、心は凪いでいる。"
    cmd = mock_run.call_args[0][0]
    assert cmd[0] == "affectus"
    assert "--config" in cmd and "/tmp/c.yaml" in cmd
    assert "--state" in cmd and "/tmp/s.json" in cmd
    assert cmd[-1] == "show"


@patch("src.affectus_tools.subprocess.run")
def test_affectus_show_omits_config_flag_when_none(mock_run):
    mock_run.return_value = MagicMock(returncode=0, stdout="x\n", stderr="")
    affectus_show(state_path="/tmp/s.json", config_path=None)
    cmd = mock_run.call_args[0][0]
    assert "--config" not in cmd


@patch("src.affectus_tools.subprocess.run")
def test_affectus_feel_serializes_deltas_as_json(mock_run):
    mock_run.return_value = MagicMock(returncode=0, stdout="いまはほどほどの喜びを感じている。\n", stderr="")
    out = affectus_feel({"joy": 0.3}, state_path="/tmp/s.json", config_path=None)
    assert out == "いまはほどほどの喜びを感じている。"
    cmd = mock_run.call_args[0][0]
    assert cmd[-2] == "feel"
    assert json.loads(cmd[-1]) == {"joy": 0.3}


@patch("src.affectus_tools.subprocess.run")
def test_affectus_reset_invokes_reset(mock_run):
    mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")
    affectus_reset(state_path="/tmp/s.json", config_path=None)
    cmd = mock_run.call_args[0][0]
    assert cmd[-1] == "reset"


@patch("src.affectus_tools.subprocess.run")
def test_affectus_show_raises_on_nonzero(mock_run):
    mock_run.return_value = MagicMock(returncode=1, stdout="", stderr="error: not found")
    with pytest.raises(RuntimeError, match="affectus show failed"):
        affectus_show(state_path="/tmp/s.json", config_path=None)
