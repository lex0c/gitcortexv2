# TODO

Future features for v2, ordered by impact.

## 1. `gitcortex serve` — web dashboard

Local HTTP server rendering stats as interactive visualizations.

- Activity timeline chart
- Coupling as a force-directed graph
- Working patterns as a color heatmap
- Contributors as a ranked bar chart
- Churn risk as a treemap by directory

Transforms the CLI from a personal tool into a team tool. Could use embedded static assets (no external dependencies) with a lightweight charting library.

## 2. CI integration

Output format compatible with CI systems for automated quality gates.

```bash
gitcortex ci --input data.jsonl \
  --fail-on-busfactor 1 \
  --fail-on-churn-risk 500 \
  --format github-actions
```

Emit GitHub Actions annotations, GitLab code quality reports, or generic SARIF. Flag files with bus factor 1 and high churn risk as warnings in pull requests. Could also track metrics over time and fail on regressions.
