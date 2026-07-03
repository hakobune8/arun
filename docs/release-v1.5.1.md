# ARUN v1.5.1 Release Notes

ARUN v1.5.1 is a Japanese Web UI localization patch after the v1.5.0
repository transfer and rename rollout.

## Changes

- Built-in scenario task-template bodies are localized in Japanese UI mode.
- Built-in agent descriptions are localized when the Web UI language is
  Japanese.
- The Web UI reloads agent metadata when the UI language changes.
- The README Web UI screenshot has been refreshed against the ARUN deployment.
- Helm chart `version`, chart `appVersion`, default image tag, and Web UI
  workspace label now identify v1.5.1.

## Upgrade

Use image tag `ghcr.io/hakobune8/arun:v1.5.1` and Helm chart version `1.5.1`.

```bash
helm upgrade --install arun oci://ghcr.io/hakobune8/charts/arun \
  --version 1.5.1 \
  --namespace arun \
  --set image.repository=ghcr.io/hakobune8/arun \
  --set image.tag=v1.5.1
```

## Verification Targets

- Web UI header shows `v1.5.1 workspace`.
- Japanese UI mode shows Japanese built-in scenario template names,
  descriptions, task-template bodies, and output language guidance.
- Japanese UI mode shows Japanese built-in agent descriptions.
- `/api/agents?uiLanguage=ja` returns localized built-in descriptions.
