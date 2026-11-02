# Contributing

Contributions are welcome. Please open an issue to discuss substantial changes
before submitting a pull request. Keep changes focused, add or update tests, and
run `go test ./...`, `go vet ./...`, and `go build ./...` before opening a
pull request.

## Tests d'intégration (Container Tests)

Les tests d'intégration valident le comportement de `xpose apply` dans un conteneur réel (`fedora:44`) :
- **Test A** : Mode interactif avec confirmation TTY (`script -qec`)
- **Test B** : Mode non interactif (`--apply --yes`)
- **Test C** : Reprise sur incident (`--resume --apply --yes`)
- **Test D** : Mode dry-run hermétique (aucune altération du système)
- **Test E** : Garde-fou anti-destruction (rejet d'actions comme `remove`)
- **Test F** : Refus interactif de restauration de dépôt tiers (`n`)
- **Test G** : Création et restauration d'archive portable `.tar.zst` (`xpose pack` + `xpose apply archive.tar.zst`)

### Exécution locale

Vous pouvez exécuter la suite d'intégration localement avec Podman (ou Docker) :

```bash
# Avec Podman (recommandé en local) :
./scripts/test-integration.sh podman

# Avec Docker :
./scripts/test-integration.sh docker
```

### Intégration Continue (CI) et workflow bloquant

Le pipeline GitHub Actions (`.github/workflows/integration.yml`) exécute deux jobs obligatoires :
1. `unit-tests` : Vérification du formatage (`gofmt`), analyse statique (`go vet`), compilation et tests unitaires Go.
2. `container-integration` : Compilation binaire statique (`CGO_ENABLED=0`) et exécution complète des tests A-F sous Docker dans `fedora:44`.

> **Workflow bloquant** : Sur les branches de production et les Pull Requests, les deux statuts `Unit Tests & Code Quality` et `Fedora 44 Container Integration (Tests A-F)` doivent obligatoirement être validés pour autoriser le merge.
