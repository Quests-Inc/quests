"""deploy/docker/release-plan.sh: which images of a release can be re-tagged from the previous one."""

from __future__ import annotations

import subprocess
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent.parent / "deploy" / "docker" / "release-plan.sh"


def git(repo: Path, *args: str) -> str:
    return subprocess.run(
        ["git", "-c", "user.name=t", "-c", "user.email=t@t", *args],
        cwd=repo, check=True, capture_output=True, text=True,
    ).stdout


def commit(repo: Path, path: str, text: str, tag: str | None = None) -> None:
    f = repo / path
    f.parent.mkdir(parents=True, exist_ok=True)
    f.write_text(text)
    git(repo, "add", "-A")
    git(repo, "commit", "-q", "-m", f"change {path}")
    if tag:
        git(repo, "tag", tag)


def plan(repo: Path, version: str) -> dict[str, str]:
    out = subprocess.run(
        ["bash", str(SCRIPT), version], cwd=repo, check=True, capture_output=True, text=True
    ).stdout
    return dict(line.split("=", 1) for line in out.splitlines())


def make_repo(tmp_path: Path) -> Path:
    repo = tmp_path / "r"
    repo.mkdir(parents=True)
    git(repo, "init", "-q", "-b", "main")
    for p in ("src/quests/a.py", "pyproject.toml", "uv.lock", "README.md", "go/main.go",
              "deploy/docker/Dockerfile", "deploy/docker/entrypoint-bot.sh", ".dockerignore"):
        commit(repo, p, "x")
    git(repo, "tag", "v0.1.0")
    return repo


def test_no_previous_release_means_build_everything(tmp_path: Path) -> None:
    repo = tmp_path / "r"
    repo.mkdir()
    git(repo, "init", "-q", "-b", "main")
    commit(repo, "go/main.go", "x")
    assert plan(repo, "0.1.0") == {"prev": "", "unchanged": ""}


def test_bot_is_reused_when_only_go_or_frontend_changed(tmp_path: Path) -> None:
    repo = make_repo(tmp_path)
    commit(repo, "go/main.go", "new")
    commit(repo, "frontend/src/a.ts", "new")
    commit(repo, "docs/x.md", "new")
    assert plan(repo, "0.1.1") == {"prev": "0.1.0", "unchanged": "bot"}


def test_bot_is_rebuilt_when_any_of_its_inputs_changed(tmp_path: Path) -> None:
    for path in ("src/quests/a.py", "pyproject.toml", "uv.lock", "README.md",
                 "deploy/docker/Dockerfile", "deploy/docker/entrypoint-bot.sh", ".dockerignore"):
        repo = make_repo(tmp_path / path.replace("/", "_"))
        commit(repo, path, "changed")
        assert plan(repo, "0.1.1")["unchanged"] == "", path


def test_previous_release_is_the_newest_final_tag_before_this_one(tmp_path: Path) -> None:
    repo = make_repo(tmp_path)
    commit(repo, "go/main.go", "1", tag="v0.1.1-rc.1")  # a pre-release is never the base
    commit(repo, "go/main.go", "2", tag="v0.1.1")
    commit(repo, "go/main.go", "3")
    assert plan(repo, "0.1.2")["prev"] == "0.1.1"
    # re-planning the release that is already tagged looks at the one before it
    assert plan(repo, "0.1.1")["prev"] == "0.1.0"


def test_a_change_between_releases_wins_even_if_it_was_reverted_in_between(tmp_path: Path) -> None:
    repo = make_repo(tmp_path)
    commit(repo, "src/quests/a.py", "changed")
    commit(repo, "src/quests/a.py", "x")  # back to the tagged content: nothing differs from v0.1.0
    assert plan(repo, "0.1.1")["unchanged"] == "bot"
