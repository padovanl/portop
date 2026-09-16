# Review of PR #1

Reviewed on 2026-09-15: [Add keyboard paging and mouse navigation](https://github.com/padovanl/portop/pull/1), commit `9dceea7d90496f28dea774091dbaef8021c04d22`, targeting `develop` (`b0356fd`).

## Recommendation

Fix the mouse coordinate mismatch before merging. The existing unit and end-to-end tests pass, but additional rendering regressions reproduce the following issues. No review comment, approval or merge was submitted to GitHub.

### P2: Mouse hit testing targets the wrong row when the title wraps

[internal/ui/mouse.go:167](https://github.com/MachinesWithThoughts/portop/blob/9dceea7d90496f28dea774091dbaef8021c04d22/internal/ui/mouse.go#L167)

`tableHeaderY()` assumes the title occupies one line. With an 80-column terminal and established connections visible, the rendered title wraps: the actual table header is at y=3, while hit testing uses y=2. Consequently, clicking a data row selects the next row, and right-click can prepare a kill confirmation for that other process. Keyboard confirmation is still required, but its target does not match the clicked process.

Derive hit testing and available table height from the same rendered layout, including wrapped titles and filter input. Add a regression that locates the header in the actual `View()` output at widths 80, 100 and 120, then clicks the rendered rows; tests that obtain their coordinates from `tableHeaderY()` cannot catch this mismatch.

### P2: Protocol sort direction is never visible

[internal/ui/table.go:134](https://github.com/MachinesWithThoughts/portop/blob/9dceea7d90496f28dea774091dbaef8021c04d22/internal/ui/table.go#L134)

The PROTO column is five cells wide. Appending ` ↑` or ` ↓` before `padTrunc` causes both the label and arrow to be truncated to `PRO…`. Clicking the heading changes sorting, but the user cannot see its direction. Reserve a cell for the indicator or use a shorter heading; verify both directions in rendered output.

## Validation

- Original PR: `go test -count=1 ./...` passed.
- Original PR: `go test -tags=e2e -count=1 ./e2e/...` passed.
- Additional regression: at width 80, rendered header y=3 versus mouse header y=2; widths 100 and 120 match.
- Additional regression: protocol sort header contains no direction arrow.
- GitHub's check-runs API returned no checks for the reviewed commit at review time.
- After installing a user-local GCC 13.3 toolchain, `go test -race -p 1 -count=1 ./...` passed on the original PR. No data race was reported.
- Simultaneous suites initially failed the existing baseline test because it observes all system listeners. Running each suite separately and its packages sequentially avoids interference. This is not one of the mouse regressions introduced by the PR.
- The PR author's reported release check was not taken as independent verification.

The PR was tested in `/tmp/portop-pr-review`, separately from the RPM/macOS changes in the working tree.
