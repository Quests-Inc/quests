#!/usr/bin/env python3
"""Гейт покрытия Go: покрытие пакета не должно падать относительно go/.coverage-baseline.json.

Профиль собирается с -coverpkg=./internal/..., поэтому интеграционные тесты засчитываются обработчикам
(http-пакеты контекстов покрываются тестами httpapi). Новый пакет должен сразу иметь >= NEW_MIN %.
Запуск: python3 tools/coverage_gate.py go/cover.out [--update]
"""
import collections
import json
import re
import sys
from pathlib import Path

NEW_MIN = 50.0   # покрытие нового пакета, %
SLACK = 0.5      # допустимое проседание пакета, п.п.
TINY = 30        # пакеты короче этого (операторов) не гейтятся поштучно

profile, baseline_path = Path(sys.argv[1]), Path(__file__).resolve().parent.parent / "go" / ".coverage-baseline.json"
blocks: dict[tuple[str, str], tuple[int, int]] = {}
for line in profile.read_text().splitlines()[1:]:
    m = re.match(r"(.+?):(\S+) (\d+) (\d+)$", line)
    if not m:
        continue
    key, n, c = (m.group(1), m.group(2)), int(m.group(3)), int(m.group(4))
    prev = blocks.get(key, (n, 0))
    blocks[key] = (n, max(prev[1], c))

stat: dict[str, list[int]] = collections.defaultdict(lambda: [0, 0])
for (path, _), (n, c) in blocks.items():
    pkg = path.rsplit("/", 1)[0].split("/go/", 1)[-1] if "/go/" in path else path.rsplit("/", 1)[0]
    pkg = pkg.split("internal/", 1)[-1] if "internal/" in pkg else pkg
    stat[pkg][0] += n
    stat[pkg][1] += n if c > 0 else 0

pct = {k: round(100.0 * v[1] / v[0], 1) for k, v in stat.items() if v[0]}
total = round(100.0 * sum(v[1] for v in stat.values()) / sum(v[0] for v in stat.values()), 1)

if "--update" in sys.argv:
    baseline_path.write_text(json.dumps({"total": total, "packages": dict(sorted(pct.items()))}, indent=2, ensure_ascii=False) + "\n")
    print(f"coverage baseline: total {total}%, packages {len(pct)}")
    sys.exit(0)

base = json.loads(baseline_path.read_text()) if baseline_path.exists() else {"total": 0, "packages": {}}
problems = []
for pkg, p in sorted(pct.items()):
    old = base["packages"].get(pkg)
    size = stat[pkg][0]
    if old is None:
        if size >= TINY and p < NEW_MIN:
            problems.append(f"{pkg}: новый пакет, покрытие {p}% < {NEW_MIN}%")
    elif p < old - SLACK:
        problems.append(f"{pkg}: покрытие упало {old}% → {p}%")
if total < base["total"] - 0.3:
    problems.append(f"итого: покрытие упало {base['total']}% → {total}%")
if problems:
    print("\n".join(problems) + f"\n\ncoverage: нарушений {len(problems)}", file=sys.stderr)
    sys.exit(1)
better = [k for k, p in pct.items() if k in base["packages"] and p > base["packages"][k] + 1]
print(f"coverage: итого {total}% (база {base['total']}%){' — можно поднять планку: --update' if better else ''}")
