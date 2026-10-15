# Plan d'issues — Phase 1 MVP v0.1 → v0.3

Ce document propose les dix issues GitHub correspondant aux exigences F1 à
F10 du PRD §10. Il ne met en œuvre aucune de ces exigences.

## Jalon v0.1 — Fedora ↔ Fedora avec Flatpak

Le premier jalon valide capture, manifeste, plan et restauration dans la même
famille Fedora, avec Flatpak. Il n'inclut pas la résolution Fedora → Debian :
le chemin cross-family dépend du pipeline de données F8 et viendra ensuite.

### Ordre suggéré

| Ordre | Issue | Dépendances bloquantes | Jalon |
|---:|---|---|---|
| 1 | F1 — Capture native et doctor | Aucune | v0.1 |
| 2 | F2 — Capture Flatpak | Aucune (peut avancer en parallèle de F1) | v0.1 |
| 3 | F3 — Manifeste YAML versionné | F1, F2 | v0.1 |
| 4 | F4 — Résolution des paquets et confiance | F1, F3 ; F8 bloque le mode cross-family Repology | v0.1 même-famille, puis v0.2 cross-family |
| 5 | F5 — Plan d'installation modifiable | F3, F4 | v0.1 |
| 6 | F7 — Export des dépôts | F3, F5 | v0.1 |
| 7 | F6 — Exécution avec journal et reprise | F5, F7 | v0.1 |
| 8 | F9 — Archive portable | F3, F6, F7 | v0.2 |
| 9 | F10 — Tests d'intégration en conteneurs | F1 à F7 ; étendre la matrice selon les cibles activées | v0.1 puis continu |
| 10 | F8 — Base Repology générée et signée | F3 ; indépendante de l'exécution utilisateur | v0.2, avant cross-family |

F4 doit d'abord permettre la correspondance directe sûre dans une migration
Fedora ↔ Fedora. Son mode cross-family, basé sur la base Repology, ne sera
activé qu'après F8. Les numéros F1–F10 sont les identifiants PRD, pas des
numéros d'issues GitHub déjà créées.

---

## F1 — Capture native et doctor

**Titre :** `F1: Capture dnf/rpm et apt/dpkg avec repli en cascade et doctor`

Compléter les adaptateurs natifs pour inventorier les paquets installés sur
Fedora et Debian/Ubuntu, y compris lorsque la commande principale échoue ou
renvoie un résultat incohérent. La commande `doctor` doit présenter clairement
la disponibilité et la santé de chaque gestionnaire.

**Critères d'acceptation**
- La capture dnf/rpm et apt/dpkg renvoie nom, version et niveau de capture.
- Les replis sont déclenchés sur erreur, résultat vide ou incohérence détectée.
- `doctor` rapporte présence, fonctionnalité et compteurs sans échouer si un gestionnaire est absent.
- Les erreurs de commandes et les replis sont signalés sans masquer une capture invalide.
- Des tests mockés couvrent les chemins nominal, repli et échec complet.

**Dépendances :** aucune.  
**Ordre suggéré :** 1.

## F2 — Capture Flatpak

**Titre :** `F2: Capturer applications, remotes et overrides Flatpak`

Capturer les applications Flatpak, leurs portées système/utilisateur, les
remotes et les overrides pour alimenter une restauration portable. Les
divergences entre installations doivent rester visibles dans les avertissements.

**Critères d'acceptation**
- Les apps et remotes sont capturés séparément avec leur portée.
- Les overrides système et utilisateur sont inclus lorsqu'ils sont accessibles.
- Un repli en lecture seule sur les répertoires Flatpak fonctionne si la CLI échoue.
- Les différences de remotes entre portées sont signalées.
- Des tests mockés vérifient le chemin nominal et le repli.

**Dépendances :** aucune (travail en parallèle de F1 possible).  
**Ordre suggéré :** 2.

## F3 — Manifeste YAML versionné

**Titre :** `F3: Stabiliser le manifeste YAML et ses migrations de schéma`

Faire du manifeste le format d'échange stable des captures et des plans, avec
validation à la lecture et compatibilité explicite des champs ajoutés.

**Critères d'acceptation**
- Le schéma publié décrit les champs communs aux captures natives et Flatpak.
- La lecture rejette les versions inconnues et les champs obligatoires invalides.
- Les champs inconnus sont préservés lors d'un aller-retour lecture/écriture.
- Les avertissements de contenu sensible sont affichés sans inclure de secret dans les journaux.
- Des fixtures versionnées valident les formats émis par F1 et F2.

**Dépendances :** F1, F2.  
**Ordre suggéré :** 3.

## F4 — Résolution des paquets et confiance

**Titre :** `F4: Résoudre les paquets avec statuts et niveaux de confiance`

Fournir une résolution compréhensible par l'utilisateur, avec provenance,
statut et confiance. Le premier jalon cible Fedora ↔ Fedora ; le chemin
cross-family Repology est une étape ultérieure et ne doit pas dépendre de
l'API publique en production.

**Critères d'acceptation**
- Le chemin Fedora ↔ Fedora traite les noms disponibles sans requête réseau.
- Chaque résultat expose statut, confiance et explication exploitable.
- Les correspondances multiples restent ambiguës ; un candidat unique fuzzy n'est pas classé ambigu.
- Le mode cross-family consulte une base locale Repology par `project_name` et alias, jamais l'API en direct.
- Les tests couvrent exact, équivalent, ambigu, introuvable, normalisation et fallback.

**Dépendances :** F1, F3 ; F8 est bloquante pour activer le cross-family.  
**Ordre suggéré :** 4 pour la résolution même-famille ; après F8 pour le cross-family.

## F5 — Plan d'installation modifiable

**Titre :** `F5: Générer un plan d'installation éditable`

Présenter avant toute modification la liste des actions, leur gestionnaire,
les commandes envisagées, les éléments ignorés et les motifs. L'utilisateur
doit pouvoir corriger le plan avant de le transmettre à l'exécuteur.

**Critères d'acceptation**
- Le plan terminal et Markdown regroupe les actions par gestionnaire et étape.
- Chaque paquet affiche sa résolution, son niveau de confiance et sa justification.
- Les paquets absents ou ambigus sont séparés des installations automatiques.
- Un plan sauvegardé peut être relu et modifié avant exécution.
- La génération du plan n'exécute aucune commande d'installation.

**Dépendances :** F3, F4.  
**Ordre suggéré :** 5.

## F6 — Exécution avec journal et reprise

**Titre :** `F6: Exécuter un plan avec journal, lots et reprise`

Exécuter uniquement les actions validées dans le plan, avec progression
persistante, reprise après interruption et repli individuel lorsqu'un paquet
échoue. Le comportement par défaut ne doit supprimer aucun paquet.

**Critères d'acceptation**
- Le mode simulation est le défaut et l'exécution réelle demande une validation explicite.
- Les dépôts sont traités avant les paquets, puis Flatpak.
- Chaque action et son résultat sont inscrits dans un journal reprenable.
- Un échec individuel n'interrompt pas les autres actions indépendantes.
- Aucune désinstallation n'est effectuée par défaut.

**Dépendances :** F5, F7.  
**Ordre suggéré :** 7.

## F7 — Export des dépôts

**Titre :** `F7: Capturer et restaurer les dépôts natifs`

Exporter les dépôts activés nécessaires à la restauration, en distinguant les
dépôts système et tiers, et produire une configuration cible vérifiable avant
activation.

**Critères d'acceptation**
- Les métadonnées utiles des dépôts dnf et apt sont capturées dans le manifeste.
- Les dépôts tiers sont explicitement identifiés et signalés dans le plan.
- Les clés ou sources tierces nécessitent une confirmation avant activation.
- Les fichiers générés sont validés et installés de manière atomique avec sauvegarde.
- Des tests vérifient le rendu sans modifier les répertoires système réels.

**Dépendances :** F3, F5.  
**Ordre suggéré :** 6, avant F6.

## F8 — Base Repology générée et signée

**Titre :** `F8: Construire et publier la base Repology en CI`

Construire une base SQLite reproductible depuis les dumps Repology, puis
publier une base versionnée et signée avec son attribution CC-BY-SA 4.0.
Cette base, et non l'API publique, sera la source du mode cross-family.

**Critères d'acceptation**
- Le pipeline CI importe le dump PostgreSQL Repology et sélectionne les dépôts cibles.
- Chaque candidat conserve son `project_name` Repology ainsi que les données nécessaires à la résolution.
- Une base SQLite reproductible est contrôlée par tests d'intégrité et de couverture.
- La version publiée est signée et comprend l'attribution et la licence requises.
- Le binaire utilisateur utilise cette base localement et ne contacte pas l'API Repology.

**Dépendances :** F3.  
**Ordre suggéré :** 10 ; doit précéder l'activation cross-family de F4.

## F9 — Archive portable

**Titre :** `F9: Créer et vérifier une archive portable de migration`

Emballer le manifeste et les fichiers de configuration explicitement
sélectionnés dans une archive portable, avec inventaire et vérification
d'intégrité avant extraction.

**Critères d'acceptation**
- `pack` produit une archive `.tar.zst` contenant manifeste et fichiers autorisés.
- Les chemins d'archive sont sûrs et ne permettent pas d'écrire hors du répertoire cible.
- L'inventaire et les sommes de contrôle sont vérifiés à la lecture.
- Les fichiers sensibles exclus par défaut ne sont pas ajoutés à l'archive.
- Une archive produite sur Fedora peut être inspectée et relue sur une autre cible de test.

**Dépendances :** F3, F6, F7.  
**Ordre suggéré :** 8.

## F10 — Tests d'intégration en conteneurs

**Titre :** `F10: Tester les migrations dans Fedora, Ubuntu et Debian`

Mettre en place des tests reproductibles qui valident capture, manifeste,
planification et chemins de restauration dans des environnements de
référence. Étendre la couverture cross-family au rythme de la disponibilité
de F8.

**Critères d'acceptation**
- Les workflows lancent des conteneurs de test Fedora, Ubuntu et Debian.
- Le jalon v0.1 vérifie au minimum capture et restauration Fedora ↔ Fedora avec Flatpak.
- Les tests ne modifient pas l'hôte et nettoient leurs ressources de test.
- Les scénarios d'échec des gestionnaires et les replis essentiels sont vérifiés.
- La matrice cross-family n'est activée qu'avec une base Repology CI disponible et versionnée.

**Dépendances :** F1 à F7 pour la suite fonctionnelle ; F8 pour les tests cross-family.  
**Ordre suggéré :** échafaudage en parallèle dès le début ; bloquant pour chaque release MVP.
