# PRD : Transpose — *Your Linux, in any key.*

> **Transpose** capture ton système Linux, le traduit pour n'importe quelle distribution, et le rejoue en une commande. Un seul binaire. Zéro dépendance. Aucune surprise.

| | |
|---|---|
| **Statut** | v1.0 du document, 14 octobre 2026 |
| **Binaire / CLI** | `xpose` |
| **Nom du projet** | Transpose (nom à valider : disponibilité GitHub, domaine, marque, conflits de noms de paquets) |
| **Licence code** | GPL-3.0 (voir §17, décision à confirmer) |
| **Licence données** | CC-BY-SA 4.0 (base dérivée de Repology) |
| **Origine** | Fusion des PRD « Eidolon » et « DistroPort » |

---

## 1. Vision et résumé

La migration d'un système Linux (nouvelle machine, changement de distribution) reste un travail manuel : on jongle entre `dnf`, `apt`, `pacman`, `flatpak`, on oublie des applications, on perd ses réglages.

**Transpose** résout ça en quatre temps :

1. **Capture** : inventaire résilient de la machine source (paquets installés volontairement, dépôts, Flatpak, Snap, paquets de langages, configs, services).
2. **Traduction** : conversion des paquets vers la distribution cible via un modèle neutre, avec un score de confiance sur chaque correspondance.
3. **Plan** : rapport lisible, éditable, simulation par défaut. Rien ne s'installe sans validation.
4. **Restauration** : exécution journalisée, reprenable, sans suppression de paquets.

**Promesse produit :** *capture → traduction → plan → restauration*, de bout en bout, y compris entre familles différentes (Fedora → Ubuntu, Ubuntu → Arch…), et même quand le gestionnaire de paquets de la machine source est cassé.

**Cas d'usage fondateur :** un utilisateur Fedora (dnf + Flatpak) migre vers Ubuntu ou Arch et retrouve ses applications, ses dépôts et ses réglages sans refaire l'inventaire à la main.

## 2. Problème

- Les listes natives (`dnf repoquery --userinstalled`, `apt-mark showmanual`, `pacman -Qqe`) ne servent que dans la **même famille** de distributions.
- Un même logiciel porte des **noms différents** (`httpd` / `apache2`, `python3-devel` / `python3-dev`), et les paquets sont découpés différemment (un paquet Fedora peut devenir deux paquets Debian).
- Les dépôts tiers (RPM Fusion, Copr, PPA, AUR) n'ont pas d'équivalent direct.
- Flatpak est portable, mais sa liste, ses dépôts (`remotes`) et ses `overrides` s'exportent séparément, en portées `--system` et `--user`.
- **Fragilité des outils mono-interface.** Incident observé sur Fedora 44 KDE : `dnf5 list installed` ne renvoyait rien malgré une base RPM saine (3 455 paquets vérifiés). Un outil qui dépend uniquement de l'interface du gestionnaire tombe avec lui. *(Constat issu d'une analyse manuelle, à reproduire et documenter en phase 0.)*
- Résultat : migrations à la main, erreurs, oublis, perte des réglages.

## 3. Analyse concurrentielle

| Outil | Ce qu'il fait | Limite pour notre besoin |
|---|---|---|
| **Aptik** | Migre paquets, dépôts, réglages, données | Même distribution et même architecture seulement |
| **apt-clone** | Clone la liste APT | APT uniquement |
| **dnf-pkgsync** | Export/import de liste dnf, avec suppression des paquets absents | dnf uniquement |
| **inxport** | Multi-gestionnaire | Pas de traduction inter-distribution, pas de plan |
| **sysreplicate** | Approche modulaire en Go | Pas de traduction inter-distribution |
| **pkgmap** (PyPI) | Base SQLite de correspondance de noms, alimentée par Repology | Traduit des noms seulement : ni capture, ni plan, ni installation |
| **RepoSources** | JSON de correspondance Debian/Ubuntu/Alpine → Homebrew | Données seulement |
| **Nix / Home Manager, Ansible** | Configuration déclarative reproductible | Il faut tout réécrire à la main : pas de capture |

**Positionnement :** les briques existent (Repology, pkgmap), mais aucun outil identifié n'enchaîne capture résiliente, traduction, plan et restauration, Flatpak intégré. *(Conclusion issue de recherches limitées, à confirmer par une veille approfondie avant le lancement public. Les caractéristiques des outils ci-dessus sont à re-vérifier avant toute communication marketing.)*

**Source de données clé : Repology.** Il suit les versions de paquets dans plus de 120 dépôts et regroupe les paquets d'un même logiciel sous un nom de projet amont commun. Ses données sont sous CC-BY-SA 4.0 : l'attribution est obligatoire et la base dérivée héritera de cette licence.

## 4. Marché et priorités de couverture

Il n'existe pas de recensement fiable des usages de Linux sur poste de travail ; les chiffres suivants sont des indicateurs partiels, pas des parts de marché.

- **Steam, juillet 2026 (joueurs, opt-in)** : Arch 8,29 %, Linux Mint 22.3 8,12 %, Fedora 44 KDE 2,92 %, Ubuntu 26.04 LTS 2,92 %, Ubuntu 24.04 2,78 %, Debian 13 1,99 %, EndeavourOS 1,69 %.
- **Développeurs (Stack Overflow 2025, via synthèses secondaires)** : Ubuntu autour de 28 %, Debian et Arch en progression.
- **Tendance** : la famille Debian est le plus gros bloc ; Arch et dérivés montent ; Fedora et openSUSE sont plus petites mais très actives chez les développeurs.

| Priorité | Gestionnaire | Distributions | Raison |
|---|---|---|---|
| **P0 (MVP)** | `dnf` / `rpm` | Fedora, RHEL, Rocky, Alma | Point de départ de l'utilisateur fondateur |
| **P0 (MVP)** | `apt` / `dpkg` | Debian, Ubuntu, Mint, Pop!_OS | Plus grande base d'utilisateurs |
| **P0 (MVP)** | `flatpak` | Toutes | Universel, la partie la plus fiable |
| **P1 (V1)** | `pacman` (+ AUR via `paru`/`yay`) | Arch, Manjaro, EndeavourOS | Forte croissance, noms souvent différents |
| **P1 (V1)** | `zypper` | openSUSE | Famille rpm, noms différents |
| **P1 (V1)** | `snap` | Ubuntu | Présent par défaut sur Ubuntu |
| **P2** | `pipx`, `cargo`, `npm -g`, Homebrew, `nix` | Toutes | Outils de développeurs |
| **P2 (plugins)** | `apk`, `xbps`, `emerge`, `rpm-ostree` | Alpine, Void, Gentoo, Silverblue | Niches, ouvertes à la communauté |
| **Détection seule** | AppImage, binaires manuels | Toutes | Pas de gestionnaire : on liste et on signale |

## 5. Utilisateurs cibles

1. **Le distro-hopper** : passe de Fedora à Arch ou Ubuntu et veut retrouver ses outils en une soirée.
2. **Le dev / sysadmin** : réinstalle ou migre souvent des machines, veut du reproductible et du versionnable avec git.
3. **Le nouvel équipement** : nouveau PC, même environnement, parfois autre distribution.
4. **L'admin de petit parc** : socle commun sur machines hétérogènes.
5. **Le débutant venu de Windows** : veut un assistant simple (TUI/GUI, V2).

## 6. Objectifs et non-objectifs

### Objectifs

1. **Résilience** : capturer même si `dnf5`/`apt` est défaillant (repli sur `rpm`, `dpkg`, fichiers d'état).
2. **Traduction inter-distribution** avec statut et niveau de confiance explicites.
3. **Sécurité par défaut** : simulation d'abord, aucune suppression, aucun secret dans le manifeste.
4. **Exhaustivité raisonnable** : paquets, dépôts, Flatpak, Snap, langages, configs, services.
5. **Manifeste lisible**, versionnable avec git, modifiable à la main.
6. **Un seul binaire statique** sans dépendance, qui fonctionne sur une installation vierge et **hors ligne**.
7. **Extensible** : un adaptateur par gestionnaire, plugins communautaires.

### Non-objectifs (V1)

- Cloner un système à l'identique (noyau, pilotes, partitionnement, bootloader).
- Reproduire les versions exactes de paquets entre distributions (impossible en général).
- Migrer les données personnelles volumineuses (délégué à rsync, borg, restic ; lien documenté).
- Remplacer Nix, Ansible ou un gestionnaire de dotfiles complet.
- Windows, macOS, BSD.

## 7. Principes de conception

1. **Modèle neutre en pivot.** On ne traduit pas « dnf vers apt » : on passe par un manifeste neutre. Ajouter une distribution = écrire un seul adaptateur, pas N conversions.
2. **Résilience en cascade.** Chaque adaptateur de capture a plusieurs niveaux : (1) CLI haut niveau, (2) outil bas niveau (`rpm`, `dpkg-query`), (3) lecture directe des fichiers d'état en lecture seule. Le rapport indique quel niveau a servi et avec quelle fiabilité.
3. **Identité du logiciel = projet amont**, pas le nom du paquet (noms de projet Repology, identifiants AppStream/Flatpak pour les applications graphiques).
4. **Honnêteté.** Chaque correspondance a un statut ; l'outil dit ce qu'il ne sait pas faire.
5. **Sécurité par défaut.** Simulation, aucune suppression, aucun secret exporté, confirmation manuelle des clés de dépôts tiers.
6. **Flatpak d'abord** pour les applications graphiques quand l'équivalent natif est introuvable ou douteux.
7. **Idempotence.** Relancer une commande ne casse rien.

## 8. Architecture

```
 MACHINE SOURCE                                              MACHINE CIBLE

 Collecteurs (parallèles)      ┌───────────────┐
  dnf/rpm, apt/dpkg,   ──────► │  MANIFESTE    │ ──► Résolveur ──► Plan ──► Exécuteur
  pacman, flatpak, snap,       │  YAML neutre  │     (base         (rapport,  (adaptateurs
  langages, configs, services  │  + schéma     │     Repology      édition,   d'installation,
                               └───────┬───────┘     locale)       validation) journal)
                                       │
                               Archive portable
                              (.tar.zst, chiffrable)
```

### 8.1 Adaptateurs (plugins)

Interface commune :

| Méthode | Rôle |
|---|---|
| `detect()` | Le gestionnaire est-il présent et fonctionnel ? |
| `capture()` | Paquets **installés volontairement** (pas les dépendances), dépôts, clés, options |
| `search(nom_amont)` | Le paquet existe-t-il dans les dépôts de la cible ? |
| `install(liste)` | Installation non interactive, journalisée |
| `health()` | État du gestionnaire (détecte les cas du type « dnf5 list vide ») |

**Capture avec repli (le gestionnaire est cassé ou renvoie une liste vide) :**

| Famille | Niveau 1 (nominal) | Niveau 2 (repli) | Niveau 3 (dernier recours) |
|---|---|---|---|
| dnf/rpm | `dnf repoquery --userinstalled` | `rpm -qa` + calcul des paquets « de premier niveau » via `rpm -qa --qf '[%{REQUIRENAME}\n]'` | Lecture seule de l'historique de transactions DNF (à valider en phase 0) |
| apt/dpkg | `apt-mark showmanual` | `dpkg-query -W` croisé avec `/var/lib/apt/extended_states` | `dpkg-query -W` seul, marqué « fiabilité réduite » |
| pacman | `pacman -Qqe` (+ `-Qqem` pour AUR/étrangers) | `/var/lib/pacman/local` | — |
| flatpak | `flatpak list --app --columns=installation,ref`, `flatpak remotes`, `flatpak override --show` (portées système et utilisateur) | Lecture des répertoires d'installation | — |
| snap | `snap list` | — | — |
| langages | `pipx list`, `cargo install --list`, `npm ls -g` | — | — |

Règle : si un niveau renvoie un résultat vide ou incohérent avec le niveau inférieur (ex. 0 paquet vs. 3 455 dans la base RPM), Transpose **le signale** et bascule sur le niveau suivant au lieu de produire un export vide en silence.

### 8.2 Manifeste (format pivot)

YAML versionné, avec schéma publié et validation. Un export JSON est disponible (`--format json`) pour l'outillage.

```yaml
schema: 1
source:
  distro: fedora
  version: "44"
  arch: x86_64
  hostname: ancien-pc
  captured_at: "2026-10-14T12:00:00Z"
  capture_level: { dnf: 2 }          # niveau de repli utilisé
repos:
  - { kind: rpm, name: rpmfusion-free, url: "...", third_party: true }
packages:
  - { name: htop,  manager: dnf, upstream: htop }
  - { name: httpd, manager: dnf, upstream: apache-httpd }
flatpak:
  remotes:   [ { name: flathub, url: "...", scope: system } ]
  apps:      [ { ref: org.mozilla.firefox, scope: system } ]
  overrides: [ ... ]
snaps: []
language_packages: { pipx: [], cargo: [], npm: [] }
services_enabled: [ sshd.service ]
cron: [ ... ]
configs:
  dotfiles:    [ .bashrc, .gitconfig ]
  app_configs: [ ~/.config/alacritty/alacritty.yml ]
unmanaged:
  - { kind: appimage, path: "~/Applications/Foo.AppImage" }
```

Règles : aucun mot de passe, jeton ou clé privée dans le manifeste ; chaque entrée est modifiable ou supprimable à la main ; les champs inconnus sont conservés (évolution sans casse).

### 8.3 Résolveur (le cœur du produit)

Ordre de résolution pour chaque paquet, arrêt au premier succès :

1. **Règles manuelles** de l'utilisateur (`overrides.yaml`).
2. **Base locale générée depuis Repology** (SQLite) : nom amont → nom natif sur la cible.
3. **Identifiants AppStream / Flatpak** pour les applications graphiques.
4. **Recherche dans les dépôts de la cible** (`search()`), en dernier recours.
5. Sinon : **introuvable**, avec suggestion (Flatpak, Snap, Homebrew, Nix, compilation).

| Statut | Signification |
|---|---|
| `exact` | Même projet amont, nom natif identique ou trouvé sans ambiguïté |
| `équivalent` | Même projet amont, nom différent |
| `alternative` | Même logiciel disponible en Flatpak ou autre format universel |
| `ambigu` | Plusieurs candidats, choix demandé à l'utilisateur |
| `introuvable` | Aucune correspondance |

Points d'attention : paquets scindés (`-devel` / `-dev`, bibliothèques séparées), paquets virtuels, paquets fusionnés ou éclatés. Mode même-famille (Fedora → Fedora) : résolution directe sans passer par Repology.

### 8.4 Planificateur

Produit un **plan** lisible (terminal, Markdown, HTML) : ce qui sera installé et par quel gestionnaire, ce qui est ignoré et pourquoi, les commandes exactes, le niveau de confiance de chaque ligne. L'utilisateur peut éditer le plan avant exécution.

### 8.5 Exécuteur

- Ordre : dépôts → paquets natifs → Flatpak → Snap → langages → configs → services.
- Journal complet, **reprise après interruption**, **aucune suppression de paquet par défaut**.
- Installation par lots avec **repli individuel** : un paquet qui échoue ne bloque pas le reste.
- Élévation de privilèges ciblée (`sudo` uniquement pour les étapes qui l'exigent).
- Clés de dépôts tiers : affichées et confirmées manuellement.

### 8.6 Configurations et état système

- **Dotfiles et configs applicatives** : sélection par profils (`--profile minimal|dev|full`), liste blanche explicite. Les fichiers sensibles (`~/.ssh`, `~/.gnupg`, jetons, trousseaux) sont **exclus par défaut**.
- **Services** : `systemctl list-unit-files --state=enabled`, rejoués par `systemctl enable` sur la cible (après vérification que l'unité existe).
- **Tâches planifiées** : crontab et timers systemd, listés dans le rapport ; restauration optionnelle.
- **Sauvegarde avant écrasement** : tout fichier remplacé sur la cible est copié dans un dossier de sauvegarde horodaté.

### 8.7 Archive portable

`xpose pack` produit une archive `.tar.zst` contenant manifeste, fichiers de configuration sélectionnés, fichiers de dépôts et signatures. Option `--encrypt` (chiffrement par phrase de passe) pour toute archive contenant des configs sensibles. L'archive est autonome : `xpose apply archive.tar.zst` suffit sur la cible.

## 9. Interface en ligne de commande

```bash
xpose capture  -o manifeste.yaml        # inventaire de la machine source
xpose pack     manifeste.yaml -o ma-machine.tar.zst [--encrypt]
xpose plan     manifeste.yaml           # simulation sur la cible, rapport
xpose apply    manifeste.yaml           # exécution du plan validé
xpose resolve  htop apache2 ...         # traduire quelques noms de paquets
xpose doctor                            # santé des gestionnaires, dépôts, droits
xpose diff     a.yaml b.yaml            # comparer deux manifestes
xpose update-db                         # mettre à jour la base de correspondance
```

Options : `--dry-run` (**activé par défaut** pour `apply` tant que le plan n'est pas validé), `--only flatpak`, `--exclude`, `--profile`, `--format json`, `--yes`, `--offline`, `--lang fr|en`.

## 10. Exigences fonctionnelles

### MVP (v0.1 → v0.3)

| ID | Exigence | Priorité |
|---|---|---|
| F1 | Capture dnf/rpm et apt/dpkg **avec repli en cascade** + `doctor` | Critique |
| F2 | Capture Flatpak (portées système/utilisateur, remotes, overrides) | Critique |
| F3 | Manifeste YAML avec schéma et validation | Critique |
| F4 | Résolveur sur base Repology locale (dnf ↔ apt), statuts et confiance | Critique |
| F5 | Plan en terminal et Markdown, édition avant exécution | Critique |
| F6 | Exécution dnf, apt, Flatpak, avec journal et reprise | Critique |
| F7 | Export des dépôts (`/etc/yum.repos.d/`, `/etc/apt/sources.list*`) | Haute |
| F8 | Base de correspondance livrée avec l'outil + `update-db` signé | Haute |
| F9 | Archive `.tar.zst` | Haute |
| F10 | Tests automatisés en conteneurs (Fedora, Ubuntu, Debian) | Critique |

### V1 (v1.0)

| ID | Exigence |
|---|---|
| F11 | Adaptateurs pacman (+ AUR), zypper, Snap |
| F12 | Configs et dotfiles par profils, exclusions de sécurité, sauvegarde avant écrasement |
| F13 | Services systemd et tâches cron |
| F14 | Chiffrement de l'archive par phrase de passe |
| F15 | Détection des paquets de langages (pipx, cargo, npm) et des AppImage |
| F16 | Assistant interactif pour les cas `ambigu` |
| F17 | Rapport HTML partageable |
| F18 | Système de plugins pour adaptateurs communautaires |
| F19 | Intégration facultative avec chezmoi / GNU Stow |

### V2

- Interface TUI (puis GUI légère).
- Distributions immuables (Silverblue, Kinoite, Bazzite : rpm-ostree, Distrobox).
- Mode « flotte » : manifestes multiples, politiques, diff entre machines.
- Intégrations Ansible / Puppet (génération de playbooks à partir du manifeste).
- Service hébergé de synchronisation (voir §16).

## 11. Exigences non fonctionnelles

- **Portabilité** : binaire statique unique, x86_64 et aarch64.
- **Performance** : capture en moins de 10 s ; résolution de 1 000 paquets en moins de 5 s hors ligne ; collecteurs exécutés en parallèle (goroutines).
- **Fiabilité** : jamais d'état à moitié installé sans journal ; opérations idempotentes ; dégradation gracieuse quand un gestionnaire est défaillant.
- **Sécurité** : pas de télémétrie par défaut ; aucun secret dans le manifeste ; clés de dépôts confirmées manuellement ; signature de la base de correspondance et des releases ; chaîne de build reproductible.
- **Accessibilité / i18n** : messages en français et en anglais dès le départ.
- **Documentation** : guide de démarrage, guide d'écriture d'un adaptateur, schéma du manifeste, `CONTRIBUTING.md`.

## 12. Choix techniques

- **Langage : Go.** Binaire statique, bonne concurrence, écosystème CLI mature (cobra). Alternative évaluée : Rust (plus rigoureux, plus lent à développer). Python écarté (installation sur machine vierge).
- **Environnement de développement : Fedora**, système le plus en pointe (DNF5, SELinux, Flatpak) pour éprouver la résilience. **Cibles de test de référence : Ubuntu LTS, Debian, Arch**, plus Fedora, via conteneurs Podman/Docker et VM.
- **Base de correspondance** : SQLite, générée par un pipeline CI à partir des exports Repology, publiée en release signée. Les utilisateurs n'interrogent pas l'API Repology en direct.
- **Structure du dépôt** :

```
transpose/
├── cmd/                  # CLI (cobra) : capture, pack, plan, apply, resolve, doctor, diff
├── internal/
│   ├── adapter/          # un adaptateur par gestionnaire (dnf, apt, pacman, flatpak, snap…)
│   ├── resolver/         # base Repology, règles, scoring
│   ├── planner/          # génération des plans et rapports
│   ├── executor/         # exécution, journal, reprise
│   ├── archive/          # .tar.zst, chiffrement
│   └── sysutil/          # exécution de commandes, logging, élévation de privilèges
├── pkg/manifest/         # schéma public du manifeste
├── data/                 # pipeline de génération de la base
└── main.go
```

- **Tests** : conteneurs par distribution avec jeux de paquets de référence ; **tests de non-régression des correspondances** ; test « gestionnaire cassé » simulant une liste vide.
- **Distribution** : binaires GitHub, `.rpm`, `.deb`, AUR, Homebrew/Nix, Flatpak si pertinent.

## 13. Indicateurs de succès

| Indicateur | Objectif |
|---|---|
| Paquets résolus automatiquement (`exact`, `équivalent`, `alternative`) sur un échantillon Fedora → Ubuntu et Arch | ≥ 85 % |
| Paquets installés avec succès après plan validé | ≥ 90 % |
| Capture réussie quand le gestionnaire principal est volontairement dégradé | 100 % des cas du jeu de tests |
| Durée d'une migration complète (hors téléchargements) | < 15 min d'interaction |
| Adoption | Étoiles GitHub, téléchargements, forks |
| Communauté | Nombre de contributeurs d'adaptateurs et de règles de correspondance |
| Qualité | Rapports « mauvaise correspondance » par release, en baisse |

## 14. Risques et réponses

| Risque | Impact | Réponse |
|---|---|---|
| Mauvaises correspondances de noms | Mauvais paquet installé | Statuts de confiance, plan validé, `overrides.yaml`, tests de non-régression |
| Paquets scindés ou fusionnés différemment | Fonctionnalités manquantes | Règles de regroupement dans la base, signalement dans le rapport |
| Dépôts tiers non transposables | Applications manquantes | Suggestion Flatpak / Nix / Homebrew, liste explicite des échecs |
| Dépendance à Repology (disponibilité, licence) | Base obsolète | Base locale versionnée, attribution CC-BY-SA, pipeline reproductible, repli sur `search()` |
| Gestionnaire source cassé ou renvoyant une liste vide | Export vide en silence | Repli en cascade, contrôle de cohérence, `doctor` |
| Fuite de secrets dans l'archive | Sécurité | Exclusions par défaut, chiffrement, avertissements |
| Paquets malveillants via dépôts tiers | Sécurité | Confirmation manuelle des dépôts et clés, simulation par défaut |
| Écrasement de configs sur la cible | Perte de données | Sauvegarde horodatée avant écrasement, mode simulation |
| Périmètre trop ambitieux | Retard | MVP limité à dnf + apt + Flatpak, plugins pour le reste |
| Concurrence (pkgmap, outils similaires) | Dilution | Se positionner sur l'expérience complète, collaborer ou réutiliser quand c'est possible |

## 15. Feuille de route

| Phase | Durée indicative | Contenu |
|---|---|---|
| **Phase 0 : Validation** | 2 semaines | Reproduire et documenter le cas dnf5 ; prototype de capture dnf + Flatpak avec repli ; échantillon de 200 correspondances vérifiées à la main pour mesurer la qualité réelle de Repology ; validation du schéma du manifeste |
| **Phase 1 : MVP (v0.1 → v0.3)** | 6 à 8 semaines | F1 à F10, alpha publique, premiers retours. Premier jalon (v0.1) : capture + restauration même famille Fedora ↔ Fedora avec Flatpak |
| **Phase 2 : V1 (v1.0)** | 8 à 12 semaines | pacman, zypper, Snap, configs, services, chiffrement, assistant interactif, plugins, rapport HTML |
| **Phase 3 : Stabilisation** | continue | Documentation, packaging dans les distributions, communauté de contributeurs |
| **V2** | après V1 | TUI/GUI, immuables, mode flotte, service hébergé |

*Les durées sont des estimations pour une petite équipe ; la Phase 0 sert précisément à les confirmer.*

## 16. Modèle open source et monétisation (V2, à étudier)

Le cœur reste gratuit et open-source. Pistes après la V1 :

- **Dons et parrainage** : GitHub Sponsors, Open Collective (faible revenu, bon pour la confiance).
- **Service hébergé de synchronisation** : sauvegarde des manifestes dans le cloud, historique, partage d'équipe (open-core).
- **Offre entreprise** : gestion de parc hétérogène, politiques de paquets, audit, intégrations Ansible/Puppet, support prioritaire.
- **API de correspondance enrichie** : données validées par la communauté, mises à jour plus fréquentes.
- **Interface graphique premium** pour les débutants.
- **Support et intégration** pour entreprises qui migrent un parc de postes.

**À éviter :** publicité, télémétrie cachée, tout ce qui détruirait la confiance de la communauté Linux.

## 17. Décisions à trancher

| # | Question | Recommandation |
|---|---|---|
| 1 | **Nom** | Valider « Transpose » / `xpose` (GitHub, domaine, marque, conflits avec des paquets existants). Prévoir un nom de secours. |
| 2 | **Licence du code** | GPL-3.0 (protège contre la fermeture du code) vs Apache-2.0 (adoption large, usage commercial libre). Recommandation : GPL-3.0, avec une politique de contribution claire (DCO ou CLA) si une offre open-core est envisagée. Le choix pèse sur la monétisation : à confirmer avant la première release. |
| 3 | **Licence des données** | CC-BY-SA 4.0 (obligatoire pour la base dérivée de Repology). |
| 4 | **Langage** | Go (recommandé). Rust si l'équipe y est plus à l'aise. |
| 5 | **Interface** | CLI seule pour le MVP ; TUI en V2. |
| 6 | **Données personnelles** | S'arrêter aux paquets et configs ; lien documenté vers rsync/borg/restic. |
| 7 | **Distributions immuables** | Reportées en V2 (ou plugin communautaire). |
| 8 | **Cible de test supplémentaire** | Arch après Ubuntu et Debian. |

## 18. Prochaines étapes

1. Valider ce PRD (périmètre MVP, licence, nom).
2. Lancer la **Phase 0** : reproduire le cas dnf5, prototype de capture avec repli, mesure de qualité des correspondances Repology sur ta propre liste de paquets Fedora.
3. Créer le dépôt GitHub : schéma du manifeste, `CONTRIBUTING.md`, `LICENSE`, premiers tests en conteneurs, pipeline CI de génération de la base.
4. Réserver le nom (GitHub, domaine) et préparer la landing page.

---

*Sources de fond : PRD « Eidolon » (résilience, configs, archive, état système) et PRD « DistroPort » (traduction Repology, plan, statuts de confiance, plugins, risques). Les chiffres de marché et les caractéristiques des outils concurrents proviennent de ces documents et sont à re-vérifier avant diffusion.*
