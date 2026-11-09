# Isolated UI review

Run `bun run dev` from `frontend`, then open `/tests/review/index.html` for templates/Markdown or
`/tests/review/app.html` for the full shell. The pages intercept fetch with fixtures and do not modify the real workspace.
These pages are development-only; Vite's production build has the normal application entry point.

Check search, state/schedule filters, empty results/reset, list → edit → save/back, long titles, explicit row menus,
container widths around 360px and a 390px phone viewport, dark/light themes, Markdown quotes and formulas, script-result
jump/re-run, and a missing target. PDF verification is in Go's `internal/pdfdoc/math_test.go`:

```sh
PDFDOC_MATH_OUT=/tmp/quests-math.pdf go test ./internal/pdfdoc -run TestMathNote137 -count=1
```

The browser supports more TeX than the PDF subset; `docs/markdown.md` records the boundary and fallback behavior.
