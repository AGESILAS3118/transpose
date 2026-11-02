# Transpose

[![Integration Tests](https://github.com/AGESILAS3118/transpose/actions/workflows/integration.yml/badge.svg)](https://github.com/AGESILAS3118/transpose/actions/workflows/integration.yml)

Transpose (`xpose`) is a Linux migration CLI that captures a system, translates
its software through a neutral manifest, and helps restore it on another
distribution.

See [PRD-Transpose.md](PRD-Transpose.md) for the product requirements and
[AVANCEMENT.txt](AVANCEMENT.txt) for project progress.

## Usage

```bash
# Capturer le système source
xpose capture -o manifest.yaml

# Créer une archive portable autonome (.tar.zst)
xpose pack manifest.yaml -o ma-machine.tar.zst

# Transporter l'archive sur la machine cible, puis :
# 1. Simuler (dry-run par défaut)
xpose apply ma-machine.tar.zst

# 2. Appliquer réellement
xpose apply ma-machine.tar.zst --apply --yes
```

