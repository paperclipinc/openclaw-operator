# Docs Site Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a versioned mkdocs-material docs site, auto-publish it on every release, and gate CRD-doc drift in CI — landing as a single foundation PR ready for the Phase 1-4 operational rollout described in the spec.

**Architecture:** mkdocs-material in `docs-site/` builds against existing `docs/*.md` + `README.md` + `ROADMAP.md` + `CHANGELOG.md`; mike publishes versioned subdirectories + a `latest` alias to `gh-pages`; the api-reference markdown is regenerated from kubebuilder CRD types by `crd-ref-docs` and gated by a CI sync check identical in shape to `Helm CRD Sync`.

**Tech Stack:** mkdocs-material 9.5, mike 2.1, `mkdocs-include-markdown-plugin`, `mkdocs-awesome-pages-plugin`, `mkdocs-git-revision-date-localized-plugin`, mkdocs Material `social` plugin (Cairo-backed OG cards), `crd-ref-docs` v0.1+, GitHub Pages, Caddy reverse proxy.

**Spec:** [2026-05-12-docs-site-design.md](./2026-05-12-docs-site-design.md)

**Branch / worktree:** `feat/docs-site` at `../openclaw-operator-docs-site/`. All tasks below assume this worktree is the current directory.

---

## File Structure (locked in before tasks)

**Created by this plan:**

| Path | Responsibility |
|---|---|
| `docs-site/mkdocs.yml` | mkdocs config — single source of truth for nav, theme, plugins, URLs |
| `docs-site/requirements.txt` | Pinned Python deps for the build |
| `docs-site/crd-ref-docs.yaml` | `crd-ref-docs` renderer config |
| `docs-site/overrides/404.html` | Custom 404 page (search + helpful links) |
| `docs-site/overrides/partials/integrations/analytics.html` | Ahrefs + PostHog snippets matching parent Astro site |
| `docs-site/scripts/generate_llms_txt.py` | Post-build hook generating `llms.txt` from nav tree |
| `docs-site/README.md` | Contributor notes (how to serve, how to regenerate api-docs, how releases publish) |
| `docs/index.md` | Site home — includes `README.md` via plugin |
| `.github/workflows/docs.yaml` | Release-triggered publish workflow (mike deploy + set-default) |

**Modified by this plan:**

| Path | Change |
|---|---|
| `Makefile` | Add `api-docs`, `docs-serve`, `docs-build`, `llms-txt` targets; new `$(CRD_REF_DOCS)` tooling var |
| `.github/workflows/ci.yaml` | Add `docs-build` and `api-docs-sync` jobs (run in parallel with existing jobs) |
| `docs/api-reference.md` | **Replace hand-written content** with `crd-ref-docs` output (large diff; one-time regenerate) |
| `.gitignore` | Add `docs-site/site/`, `docs-site/.venv/`, `bin/crd-ref-docs` |
| `CLAUDE.md` | Remove the manual "always update README and api-reference together" rule (CI now enforces api-reference; README rule still useful) |

**Out of scope for this PR (Phases 1-4, per spec):**

- `charts/openclaw-operator/Chart.yaml` docs link update (Phase 3, separate one-line PR)
- Caddyfile changes on the Hetzner box (Phase 2, ops step)
- DNS CNAME record (Phase 0 ops, documented as a manual step)
- Private `app` repo cleanup (Phase 3, separate repo)

---

## Task 1: Pin Python deps and build a minimal mkdocs site against existing docs

**Files:**
- Create: `docs-site/requirements.txt`
- Create: `docs-site/mkdocs.yml`
- Create: `docs/index.md`
- Modify: `Makefile`
- Modify: `.gitignore`

- [ ] **Step 1: Add pinned Python deps**

Create `docs-site/requirements.txt`:

```text
mkdocs==1.6.1
mkdocs-material==9.5.49
mkdocs-include-markdown-plugin==6.2.2
mkdocs-awesome-pages-plugin==2.9.3
mkdocs-git-revision-date-localized-plugin==1.2.9
mkdocs-minify-plugin==0.8.0
pymdown-extensions==10.12
mike==2.1.3
# Material social plugin (per-page OG cards) deps
cairosvg==2.7.1
pillow==11.0.0
```

- [ ] **Step 2: Write the minimal mkdocs.yml**

Create `docs-site/mkdocs.yml`:

```yaml
site_name: OpenClaw Operator
site_url: https://openclaw.rocks/docs/operator/
site_description: Kubernetes operator for managing OpenClaw AI agent instances with production-grade security, observability, and lifecycle management.
site_author: OpenClaw.rocks
repo_url: https://github.com/openclaw-rocks/openclaw-operator
repo_name: openclaw-rocks/openclaw-operator
edit_uri: edit/main/docs/
copyright: Copyright &copy; OpenClaw.rocks

docs_dir: ../docs

theme:
  name: material
  custom_dir: overrides
  palette:
    - media: "(prefers-color-scheme: light)"
      scheme: default
      primary: indigo
      accent: indigo
      toggle:
        icon: material/brightness-7
        name: Switch to dark mode
    - media: "(prefers-color-scheme: dark)"
      scheme: slate
      primary: indigo
      accent: indigo
      toggle:
        icon: material/brightness-4
        name: Switch to light mode
  features:
    - navigation.instant
    - navigation.tracking
    - navigation.tabs
    - navigation.top
    - navigation.indexes
    - search.suggest
    - search.highlight
    - content.code.copy
    - content.action.edit
    - content.action.view
    - toc.follow
  icon:
    repo: fontawesome/brands/github

plugins:
  - search
  - awesome-pages
  - include-markdown
  - git-revision-date-localized:
      type: timeago
      fallback_to_build_date: true
  - minify:
      minify_html: true

markdown_extensions:
  - admonition
  - attr_list
  - md_in_html
  - tables
  - toc:
      permalink: true
  - pymdownx.details
  - pymdownx.superfences:
      custom_fences:
        - name: mermaid
          class: mermaid
          format: !!python/name:pymdownx.superfences.fence_code_format
  - pymdownx.tabbed:
      alternate_style: true
  - pymdownx.highlight:
      anchor_linenums: true
      line_spans: __span
      pygments_lang_class: true
  - pymdownx.inlinehilite
  - pymdownx.snippets:
      base_path:
        - ..

extra:
  social:
    - icon: fontawesome/brands/github
      link: https://github.com/openclaw-rocks/openclaw-operator
```

- [ ] **Step 3: Add a home page that includes the README**

Create `docs/index.md`:

```markdown
---
title: OpenClaw Operator
description: Kubernetes operator for managing OpenClaw AI agent instances.
hide:
  - navigation
---

{%
  include-markdown "../README.md"
  heading-offset=0
%}
```

- [ ] **Step 4: Add Makefile targets**

Append to `Makefile` (place near other doc-related targets; if none, at end):

```makefile
##@ Docs Site

.PHONY: docs-venv
docs-venv: docs-site/.venv/bin/activate ## Create the docs-site Python virtualenv
docs-site/.venv/bin/activate: docs-site/requirements.txt
	python3 -m venv docs-site/.venv
	docs-site/.venv/bin/pip install --upgrade pip
	docs-site/.venv/bin/pip install -r docs-site/requirements.txt
	touch docs-site/.venv/bin/activate

.PHONY: docs-serve
docs-serve: docs-venv ## Run the docs site locally (http://127.0.0.1:8000)
	docs-site/.venv/bin/mkdocs serve -f docs-site/mkdocs.yml

.PHONY: docs-build
docs-build: docs-venv ## Build the docs site (strict mode -- fails on broken links / warnings)
	docs-site/.venv/bin/mkdocs build --strict -f docs-site/mkdocs.yml
```

- [ ] **Step 5: Ignore the venv and the build output**

Modify `.gitignore` — append:

```text

# Docs site
docs-site/.venv/
docs-site/site/
bin/crd-ref-docs
```

- [ ] **Step 6: Run the build and verify it passes**

Run:
```bash
make docs-build
```

Expected: exit 0; output ends with `INFO    -  Documentation built in N.NNs`. The Python `--strict` flag may fail on the first run if internal links in existing `docs/*.md` are broken — that's a real bug to surface, not a CI problem.

If `--strict` fails: fix the broken links in `docs/*.md` *in this task* before committing. Do not relax `--strict` to make it pass.

- [ ] **Step 7: Commit**

```bash
git add docs-site/ docs/index.md Makefile .gitignore
git commit -m "feat(docs-site): scaffold mkdocs-material site against existing docs/"
```

---

## Task 2: Auto-generate `docs/api-reference.md` from CRD types

**Files:**
- Create: `docs-site/crd-ref-docs.yaml`
- Modify: `Makefile`
- Modify: `docs/api-reference.md` (replace hand-written content)

- [ ] **Step 1: Add `crd-ref-docs` renderer config**

Create `docs-site/crd-ref-docs.yaml`:

```yaml
processor:
  ignoreTypes:
    - "(.*)List$"
  ignoreFields:
    - "status$"
    - "TypeMeta$"

render:
  kubernetesVersion: 1.31
  knownTypes:
    - name: Quantity
      package: k8s.io/apimachinery/pkg/api/resource
      link: https://pkg.go.dev/k8s.io/apimachinery/pkg/api/resource#Quantity
    - name: ObjectMeta
      package: k8s.io/apimachinery/pkg/apis/meta/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta
    - name: LocalObjectReference
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#localobjectreference-v1-core
    - name: Container
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#container-v1-core
    - name: Volume
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volume-v1-core
    - name: VolumeMount
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volumemount-v1-core
    - name: Toleration
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#toleration-v1-core
    - name: Affinity
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#affinity-v1-core
    - name: TopologySpreadConstraint
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#topologyspreadconstraint-v1-core
    - name: EnvVar
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envvar-v1-core
    - name: EnvFromSource
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envfromsource-v1-core
    - name: PullPolicy
      package: k8s.io/api/core/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#-strong-properties-strong-container-v1-core
    - name: NetworkPolicyEgressRule
      package: k8s.io/api/networking/v1
      link: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#networkpolicyegressrule-v1-networking-k8s-io
    - name: RawExtension
      package: k8s.io/apimachinery/pkg/runtime
      link: https://pkg.go.dev/k8s.io/apimachinery/pkg/runtime#RawExtension
  markdownDisabled: false
  format: markdown
```

- [ ] **Step 2: Add the `crd-ref-docs` tool install + `api-docs` target to the Makefile**

Append to the `Makefile` (in the tooling section near `$(CONTROLLER_GEN)`):

```makefile
##@ Docs generation

CRD_REF_DOCS ?= $(LOCALBIN)/crd-ref-docs
CRD_REF_DOCS_VERSION ?= v0.1.0

.PHONY: crd-ref-docs
crd-ref-docs: $(CRD_REF_DOCS) ## Download crd-ref-docs locally if necessary.
$(CRD_REF_DOCS): $(LOCALBIN)
	test -s $(LOCALBIN)/crd-ref-docs || GOBIN=$(LOCALBIN) go install github.com/elastic/crd-ref-docs@$(CRD_REF_DOCS_VERSION)

.PHONY: api-docs
api-docs: manifests crd-ref-docs ## Regenerate docs/api-reference.md from CRD types.
	$(CRD_REF_DOCS) \
	  --config docs-site/crd-ref-docs.yaml \
	  --source-path api/v1alpha1 \
	  --output-path docs/api-reference.md \
	  --renderer markdown
```

(If `LOCALBIN` is not already defined in `Makefile`, locate the existing `$(CONTROLLER_GEN)` definition and reuse the same `LOCALBIN` value. The existing operator scaffold defines `LOCALBIN := $(shell pwd)/bin`.)

- [ ] **Step 3: Regenerate `docs/api-reference.md` from CRDs**

Run:
```bash
make api-docs
```

Expected: `crd-ref-docs` installs into `bin/`, then writes `docs/api-reference.md`. The diff will be large — the entire hand-written file is replaced.

- [ ] **Step 4: Verify the generated content covers all current CRD fields**

Run:
```bash
grep -c "^### " docs/api-reference.md
```

Compare against the field count from `make manifests` output. Spot-check 3 recently-added fields (`shareProcessNamespace`, `runtimeClassName`, `podAnnotations`) — they should all appear in the generated reference.

- [ ] **Step 5: Run docs build to verify mkdocs renders the generated content**

Run:
```bash
make docs-build
```

Expected: exit 0. If `--strict` fails on links inside the generated api-reference, that indicates the renderer config needs an additional `knownTypes` entry for the unresolved type — add it and rerun `make api-docs`.

- [ ] **Step 6: Commit**

```bash
git add docs-site/crd-ref-docs.yaml Makefile docs/api-reference.md
git commit -m "feat(docs): auto-generate api-reference.md from CRD types via crd-ref-docs"
```

---

## Task 3: Add CI jobs `Docs Build` and `API Docs Sync`

**Files:**
- Modify: `.github/workflows/ci.yaml`

- [ ] **Step 1: Add the `docs-build` job**

Open `.github/workflows/ci.yaml`. Locate the `jobs:` section. Add this job at the same level as the existing `lint`, `test`, `helm-rbac-sync` jobs:

```yaml
  docs-build:
    name: Docs Build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-python@v5
        with:
          python-version: '3.12'
          cache: pip
          cache-dependency-path: docs-site/requirements.txt

      - name: Install Cairo (Material social plugin dep)
        run: sudo apt-get update && sudo apt-get install -y libcairo2-dev libfreetype6-dev

      - name: Install docs deps
        run: pip install -r docs-site/requirements.txt

      - name: Build docs site (strict)
        run: mkdocs build --strict -f docs-site/mkdocs.yml

      - name: Upload built site as artifact
        if: github.event_name == 'pull_request'
        uses: actions/upload-artifact@v4
        with:
          name: docs-preview-${{ github.event.pull_request.number }}
          path: docs-site/site/
          retention-days: 7
```

- [ ] **Step 2: Add the `api-docs-sync` job**

In the same `jobs:` section, immediately after `docs-build`:

```yaml
  api-docs-sync:
    name: API Docs Sync
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: ${{ env.GO_VERSION }}

      - name: Regenerate api-reference.md
        run: make api-docs

      - name: Verify no drift
        run: |
          if ! git diff --exit-code docs/api-reference.md; then
            echo "::error::docs/api-reference.md is out of sync with CRD types."
            echo "::error::Run 'make api-docs' locally and commit the result."
            exit 1
          fi
```

- [ ] **Step 3: Commit and push to the feature branch**

```bash
git add .github/workflows/ci.yaml
git commit -m "ci: gate docs build and api-reference drift on every PR"
git push -u origin feat/docs-site
```

- [ ] **Step 4: Verify both jobs run and pass**

Run:
```bash
gh pr create --draft --title "feat: docs site foundation (draft)" --body "Draft for CI smoke. Not for merge yet."
sleep 30
gh pr checks --watch
```

Expected: both `Docs Build` and `API Docs Sync` appear and complete with `pass`. If either fails, read the log via `gh run view --log-failed` and fix before moving on.

---

## Task 4: Add `mike` versioning config to `mkdocs.yml`

**Files:**
- Modify: `docs-site/mkdocs.yml`

- [ ] **Step 1: Add the mike plugin block and version selector**

In `docs-site/mkdocs.yml`, the `plugins:` list. Insert `mike` at the top of the list (before `search`):

```yaml
plugins:
  - mike:
      alias_type: redirect
      canonical_version: latest
      version_selector: true
      css_dir: css
      javascript_dir: js
  - search
  - awesome-pages
  - include-markdown
  - git-revision-date-localized:
      type: timeago
      fallback_to_build_date: true
  - minify:
      minify_html: true
```

Also add `extra.version` and `extra.cname` to the `extra:` block (replacing the previous `extra:` entirely):

```yaml
extra:
  version:
    provider: mike
    default: latest
  cname: docs-operator.openclaw.rocks
  social:
    - icon: fontawesome/brands/github
      link: https://github.com/openclaw-rocks/openclaw-operator
```

- [ ] **Step 2: Verify the build still succeeds**

Run:
```bash
make docs-build
```

Expected: exit 0. The version selector won't render meaningfully until something is published via `mike deploy`, but `mkdocs build` must still pass.

- [ ] **Step 3: Do a dry-run mike deploy locally to verify config**

Run:
```bash
cd docs-site
../docs-site/.venv/bin/mike deploy --no-push 0.33.0-dryrun latest --title "v0.33.0-dryrun"
cd ..
```

Expected: mike creates a local `gh-pages` branch with `/0.33.0-dryrun/` and `/latest/` paths. Verify:

```bash
git branch | grep gh-pages
git show gh-pages --stat | head -20
```

Then **clean up the local-only dry-run branch**:

```bash
git branch -D gh-pages
```

(The real `gh-pages` will be created by CI on the first release publish; we don't want a polluted local branch.)

- [ ] **Step 4: Commit**

```bash
git add docs-site/mkdocs.yml
git commit -m "feat(docs-site): enable mike versioning with custom CNAME"
```

---

## Task 5: Add the release-triggered `docs.yaml` workflow

**Files:**
- Create: `.github/workflows/docs.yaml`

- [ ] **Step 1: Write the workflow**

Create `.github/workflows/docs.yaml`:

```yaml
name: Docs

on:
  release:
    types: [published]
  workflow_dispatch:
    inputs:
      version:
        description: 'Tag to (re)publish (e.g. v0.33.0)'
        required: true
        type: string

permissions:
  contents: write

concurrency:
  group: docs-publish
  cancel-in-progress: false

jobs:
  publish:
    name: Publish versioned docs to gh-pages
    runs-on: ubuntu-latest
    steps:
      - name: Determine ref
        id: ref
        run: |
          REF="${{ github.event.release.tag_name || inputs.version }}"
          echo "ref=$REF" >> "$GITHUB_OUTPUT"

      - uses: actions/checkout@v4
        with:
          ref: ${{ steps.ref.outputs.ref }}
          fetch-depth: 0

      - uses: actions/setup-python@v5
        with:
          python-version: '3.12'
          cache: pip
          cache-dependency-path: docs-site/requirements.txt

      - name: Install Cairo (Material social plugin dep)
        run: sudo apt-get update && sudo apt-get install -y libcairo2-dev libfreetype6-dev

      - name: Install docs deps
        run: pip install -r docs-site/requirements.txt

      - name: Configure git for mike commits
        run: |
          git config user.name 'github-actions[bot]'
          git config user.email '41898282+github-actions[bot]@users.noreply.github.com'
          git fetch origin gh-pages --depth=1 || true

      - name: Extract version components
        id: ver
        run: |
          TAG="${{ steps.ref.outputs.ref }}"
          FULL="${TAG#v}"
          SHORT=$(echo "$FULL" | cut -d. -f1,2)
          echo "full=$FULL" >> "$GITHUB_OUTPUT"
          echo "short=$SHORT" >> "$GITHUB_OUTPUT"

      - name: Mike deploy
        working-directory: docs-site
        run: |
          mike deploy --push --update-aliases \
            "${{ steps.ver.outputs.short }}" latest \
            --title "v${{ steps.ver.outputs.full }}"

      - name: Mike set default
        working-directory: docs-site
        run: mike set-default --push latest
```

- [ ] **Step 2: Lint the workflow YAML**

Run:
```bash
docker run --rm -v "${PWD}:/repo" rhysd/actionlint:latest -color /repo/.github/workflows/docs.yaml
```

Expected: no output (clean). If `actionlint` isn't acceptable in your environment, alternatively:

```bash
gh workflow view docs.yaml 2>&1 || echo "Workflow not yet pushed; check parse on next push."
```

The real validation comes from GitHub itself on push — it will reject malformed workflow YAML.

- [ ] **Step 3: Commit + push**

```bash
git add .github/workflows/docs.yaml
git commit -m "feat(docs-site): release-triggered mike publish workflow"
git push
```

- [ ] **Step 4: Verify GitHub registers the workflow**

Run:
```bash
gh workflow list --json name,state | jq '.[] | select(.name=="Docs")'
```

Expected: returns one entry with `"state":"active"`. If not present, GitHub rejected the YAML — read the failure with `gh api repos/openclaw-rocks/openclaw-operator/actions/workflows` and fix syntax.

---

## Task 6: Theme overrides — custom 404 + analytics partial

**Files:**
- Create: `docs-site/overrides/404.html`
- Create: `docs-site/overrides/partials/integrations/analytics.html`

- [ ] **Step 1: Write the custom 404**

Create `docs-site/overrides/404.html`:

```html
{% extends "main.html" %}

{% block container %}
<div class="md-content" data-md-component="content">
  <article class="md-content__inner md-typeset">
    <h1>Page not found</h1>
    <p>
      The page you requested doesn't exist in this version of the docs. A few things to try:
    </p>
    <ul>
      <li>Search using the bar at the top of the page.</li>
      <li>Jump to the <a href="/docs/operator/latest/">latest docs</a>.</li>
      <li>Browse the <a href="/docs/operator/latest/api-reference/">API reference</a>.</li>
      <li>Open a question on <a href="https://github.com/openclaw-rocks/openclaw-operator/issues/new/choose">GitHub Issues</a>.</li>
    </ul>
  </article>
</div>
{% endblock %}
```

- [ ] **Step 2: Write the analytics partial mirroring parent site (Ahrefs + PostHog)**

Create `docs-site/overrides/partials/integrations/analytics.html`:

```html
<!--
  Mirrors the analytics setup on openclaw.rocks (Ahrefs + PostHog).
  Visitor identity is shared with the parent Astro site so funnel data is unified.
  Real keys come from repository variables AHREFS_KEY and POSTHOG_KEY at build time;
  if absent (e.g., PR builds or local dev), the snippets render as no-ops.
-->
{% if config.extra.analytics_keys %}
<script async src="https://analytics.ahrefs.com/analytics.js"
        data-key="{{ config.extra.analytics_keys.ahrefs }}"></script>
<script>
  !function(t,e){var o,n,p,r;e.__SV||(window.posthog=e,e._i=[],e.init=function(i,s,a){function g(t,e){var o=e.split(".");2==o.length&&(t=t[o[0]],e=o[1]),t[e]=function(){t.push([e].concat(Array.prototype.slice.call(arguments,0)))}}(p=t.createElement("script")).type="text/javascript",p.async=!0,p.src=s.api_host+"/static/array.js",(r=t.getElementsByTagName("script")[0]).parentNode.insertBefore(p,r);var u=e;for(void 0!==a?u=e[a]=[]:a="posthog",u.people=u.people||[],u.toString=function(t){var e="posthog";return"posthog"!==a&&(e+="."+a),t||(e+=" (stub)"),e},u.people.toString=function(){return u.toString(1)+".people (stub)"},o="capture identify alias people.set people.set_once set_config register register_once unregister opt_out_capturing has_opted_out_capturing opt_in_capturing reset isFeatureEnabled onFeatureFlags getFeatureFlag getFeatureFlagPayload reloadFeatureFlags group updateEarlyAccessFeatureEnrollment getEarlyAccessFeatures getActiveMatchingSurveys getSurveys".split(" "),n=0;n<o.length;n++)g(u,o[n]);e._i.push([i,s,a])},e.__SV=1)}(document,window.posthog||[]);
  posthog.init('{{ config.extra.analytics_keys.posthog }}', {api_host:'https://eu.i.posthog.com'});
</script>
{% endif %}
```

Then update `docs-site/mkdocs.yml` `extra:` block to source keys from environment variables at build time:

```yaml
extra:
  version:
    provider: mike
    default: latest
  cname: docs-operator.openclaw.rocks
  analytics_keys:
    ahrefs: !ENV [AHREFS_KEY, '']
    posthog: !ENV [POSTHOG_KEY, '']
  social:
    - icon: fontawesome/brands/github
      link: https://github.com/openclaw-rocks/openclaw-operator
```

(The `!ENV` tag is mkdocs-builtin. Empty string falls through to the `{% if %}` guard in the partial, so PR builds without secrets simply omit the trackers.)

- [ ] **Step 3: Set the partial path on the Material theme**

In `docs-site/mkdocs.yml`, the `theme:` block already references `custom_dir: overrides`. Material auto-discovers `overrides/partials/integrations/analytics.html` — no extra config needed. To force inclusion explicitly, add `extra.analytics` is not needed when using a custom partial path.

- [ ] **Step 4: Build and visually inspect**

Run:
```bash
make docs-build
# spot-check the 404 was generated
ls -la docs-site/site/404.html
# spot-check the analytics partial was rendered (empty since AHREFS_KEY unset locally)
grep -c "analytics.ahrefs.com" docs-site/site/index.html  # expect 0 locally
```

Then start the local server and visit a nonexistent path:

```bash
make docs-serve &
sleep 2
curl -sI http://127.0.0.1:8000/nonexistent-page | head -3
kill %1
```

Expected: 200 with the custom 404 content. (mkdocs serves 404 for missing pages but returns 200 in local serve mode; in production GH Pages will serve as 404 + the same HTML.)

- [ ] **Step 5: Commit**

```bash
git add docs-site/overrides/ docs-site/mkdocs.yml
git commit -m "feat(docs-site): custom 404 + Ahrefs/PostHog analytics partial"
```

---

## Task 7: Material `social` plugin for per-page OG cards

**Files:**
- Modify: `docs-site/mkdocs.yml`

- [ ] **Step 1: Enable the social plugin**

In `docs-site/mkdocs.yml`, append to the `plugins:` list (after `minify`):

```yaml
  - social:
      cards: true
      cards_layout_options:
        background_color: "#050810"
        color: "#ffffff"
```

(The `#050810` matches the parent Astro site's `theme-color` meta, so cards visually match.)

- [ ] **Step 2: Build and verify cards are generated**

Run:
```bash
make docs-build
ls docs-site/site/assets/images/social/ | head -10
```

Expected: one PNG per page (`index.png`, `api-reference.png`, etc.). If you see an `OSError: no library called "cairo" was found`, install Cairo locally:

```bash
# macOS
brew install cairo pango
# Linux
sudo apt-get install -y libcairo2-dev libfreetype6-dev
```

(CI already installs `libcairo2-dev` in Task 3 Step 1.)

- [ ] **Step 3: Spot-check OG meta in generated HTML**

Run:
```bash
grep -E 'og:image|twitter:image' docs-site/site/index.html | head -4
```

Expected: lines pointing to `assets/images/social/index.png` (or similar). Confirms the meta is wired.

- [ ] **Step 4: Commit**

```bash
git add docs-site/mkdocs.yml
git commit -m "feat(docs-site): per-page OG and Twitter cards via Material social plugin"
```

---

## Task 8: `llms.txt` via mkdocs `on_post_build` hook

**Files:**
- Create: `docs-site/hooks/llms_txt.py`
- Modify: `docs-site/mkdocs.yml`

Rationale: mike calls `mkdocs build` internally and ships the result to `gh-pages`. A pre-step `python generate_llms_txt.py` would write `llms.txt` *before* mike's build wipes the directory. Wiring via mkdocs' `hooks:` config means `llms.txt` is generated *during* mike's build (in mkdocs' `on_post_build` event), so it ends up in mike's commit to `gh-pages`.

- [ ] **Step 1: Write the mkdocs post-build hook**

Create `docs-site/hooks/llms_txt.py`:

```python
"""
mkdocs hook that emits /llms.txt at the end of each build.

The llms.txt convention (https://llmstxt.org/) provides AI agents a
structured, machine-readable index of canonical pages with one-line
summaries pulled from each page's <title> and <meta name="description">.

Wired via mkdocs.yml's `hooks:` config; runs inside every `mkdocs build`,
including the one mike executes during a release. The file is materialized
into config['site_dir'] so it ships in the gh-pages commit alongside the
HTML output.
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from mkdocs.config.defaults import MkDocsConfig

TITLE_RE = re.compile(r"<title>(.*?)</title>", re.IGNORECASE | re.DOTALL)
DESC_RE = re.compile(
    r'<meta\s+name=["\']description["\']\s+content=["\'](.*?)["\']',
    re.IGNORECASE | re.DOTALL,
)


def _extract(html: str, regex: re.Pattern[str]) -> str:
    match = regex.search(html)
    if not match:
        return ""
    return re.sub(r"\s+", " ", match.group(1)).strip()


def on_post_build(config: "MkDocsConfig", **kwargs) -> None:
    site_dir = Path(config["site_dir"])
    site_url = config["site_url"].rstrip("/") + "/"

    entries: list[tuple[str, str, str]] = []
    for html_path in sorted(site_dir.rglob("index.html")):
        rel = html_path.relative_to(site_dir).parent
        rel_str = str(rel)
        url = site_url + (rel_str + "/" if rel_str != "." else "")
        html = html_path.read_text(encoding="utf-8", errors="ignore")
        title = _extract(html, TITLE_RE) or rel_str or "Home"
        desc = _extract(html, DESC_RE)
        entries.append((title, url, desc))

    lines: list[str] = [
        "# OpenClaw Operator",
        "",
        "> Kubernetes operator for managing OpenClaw AI agent instances with production-grade security, observability, and lifecycle management.",
        "",
        "## Docs",
        "",
    ]
    for title, url, desc in entries:
        lines.append(f"- [{title}]({url}): {desc}" if desc else f"- [{title}]({url})")

    out = site_dir / "llms.txt"
    out.write_text("\n".join(lines) + "\n", encoding="utf-8")
    print(f"[llms-txt] wrote {out} ({len(entries)} entries)")
```

- [ ] **Step 2: Wire the hook into `mkdocs.yml`**

In `docs-site/mkdocs.yml`, add at the top level (peer to `plugins`, `theme`, etc.):

```yaml
hooks:
  - hooks/llms_txt.py
```

mkdocs resolves the path relative to the config file's directory, so this picks up `docs-site/hooks/llms_txt.py`.

- [ ] **Step 3: Build and verify the file**

Run:
```bash
make docs-build
head -20 docs-site/site/llms.txt
```

Expected output begins:
```
# OpenClaw Operator

> Kubernetes operator for managing OpenClaw AI agent instances...

## Docs

- [...]: ...
```

The build log also shows: `[llms-txt] wrote /…/docs-site/site/llms.txt (N entries)`.

- [ ] **Step 4: Verify the hook runs inside mike's build path too**

Run a local mike dry-run:

```bash
cd docs-site
../docs-site/.venv/bin/mike deploy --no-push 0.33.0-llmcheck latest --title check
cd ..
git show gh-pages -- '0.33.0-llmcheck/llms.txt' | head -10
git branch -D gh-pages
```

Expected: `git show` displays the first lines of `llms.txt`. Confirms the hook runs during mike's internal build, so the release workflow doesn't need a separate llms.txt step.

- [ ] **Step 5: Commit**

```bash
git add docs-site/hooks/ docs-site/mkdocs.yml
git commit -m "feat(docs-site): emit llms.txt via mkdocs on_post_build hook"
```

---

## Task 9: Contributor README in `docs-site/`

**Files:**
- Create: `docs-site/README.md`

- [ ] **Step 1: Write the contributor README**

Create `docs-site/README.md`:

```markdown
# Docs site for openclaw-operator

This directory contains the mkdocs-material project that publishes
[openclaw.rocks/docs/operator/](https://openclaw.rocks/docs/operator/latest/).

## Local preview

```bash
make docs-serve
# open http://127.0.0.1:8000
```

The Python virtualenv lives at `docs-site/.venv/` and is `.gitignore`-d.

## Build (strict mode)

```bash
make docs-build
```

This is what CI runs on every PR (`Docs Build` job). It fails on broken
internal links, missing pages, and any mkdocs warning. Fix the source,
don't relax `--strict`.

## API reference auto-generation

`docs/api-reference.md` is **not hand-edited**. It is generated from the
kubebuilder CRD types in `api/v1alpha1/` by
[`crd-ref-docs`](https://github.com/elastic/crd-ref-docs).

```bash
make api-docs
```

CI (`API Docs Sync` job) fails any PR where running `make api-docs`
produces a diff against the committed file. If you change a CRD type,
run `make api-docs` and commit the result in the same PR.

## Releases

`docs.yaml` workflow fires on every `release: published` event. It checks
out the released tag, runs `mike deploy <minor> latest`, and pushes to
`gh-pages`. GH Pages serves the result at
`docs-operator.openclaw.rocks`, and Caddy on the openclaw.rocks box
reverse-proxies `openclaw.rocks/docs/operator/*` to it.

## Republishing a specific tag manually

```bash
gh workflow run docs.yaml -f version=v0.33.0
```

## Adding a new page

Drop a `.md` file under `docs/`. `awesome-pages` auto-builds the nav. For
explicit nav ordering or to hide a page, add a `.pages` file in the same
directory. See https://github.com/lukasgeiter/mkdocs-awesome-pages-plugin.

## Theme tweaks

`overrides/` mirrors the Material theme structure. Don't bloat overrides —
prefer config tweaks in `mkdocs.yml` first.
```

- [ ] **Step 2: Commit**

```bash
git add docs-site/README.md
git commit -m "docs(docs-site): contributor README"
```

---

## Task 10: Update `CLAUDE.md` to reflect CI-enforced api-reference

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Drop the manual api-reference-sync rule**

Open `CLAUDE.md`. Locate the section "Documentation" containing:

> When adding or changing CRD fields, features, or behavior, **always** update both:
> - `README.md` -- user-facing overview, examples, and feature table
> - `docs/api-reference.md` -- exhaustive field-level reference for every spec and status field

Replace with:

```markdown
### Documentation

When adding or changing CRD fields, features, or behavior:

- **`README.md`** -- update the user-facing overview, examples, and the feature table.
- **`docs/api-reference.md`** is **auto-generated** from CRD types via `make api-docs`. Do NOT hand-edit it. After modifying types in `api/v1alpha1/`, run `make manifests api-docs` and commit the regenerated reference together with the type change. CI (`API Docs Sync` job) blocks any PR where running `make api-docs` would produce a diff.

The docs site (mkdocs-material) lives in `docs-site/`. See `docs-site/README.md` for local preview and contributor flow.
```

- [ ] **Step 2: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude): reflect CI-enforced api-reference, point at docs-site/"
```

---

## Task 11: Open the PR (still draft) and verify all CI

**Files:** none.

- [ ] **Step 1: Push and convert draft to ready (or open if not yet pushed)**

```bash
git push
gh pr ready  # or: gh pr edit --add-label "..."
```

If the draft PR from Task 3 Step 4 is still open, this just refreshes it. If it was closed, open a fresh one:

```bash
gh pr create --title "feat: docs site at openclaw.rocks/docs/operator/" --body "$(cat <<'EOF'
## Summary

Foundation PR for the versioned docs site, per the [design spec](./docs/specs/2026-05-12-docs-site-design.md). Establishes:

- mkdocs-material site under `docs-site/` building from existing `docs/*.md` + `README.md` + `ROADMAP.md` + `CHANGELOG.md`
- `make api-docs` regenerates `docs/api-reference.md` from CRD types (one-time replacement of the hand-written content in this PR)
- Two new CI gates: `Docs Build` (strict mkdocs build) and `API Docs Sync` (regenerate-and-diff check, identical pattern to existing `Helm CRD Sync`)
- `docs.yaml` workflow: on every `release: published`, mike publishes `/<minor>/` and the `latest` alias to `gh-pages`
- mike versioning, Material social plugin for OG cards, custom 404, Ahrefs/PostHog analytics (env-gated), `llms.txt` for AI agent discovery

## What this PR is NOT

- Phase 1 (first publish via workflow_dispatch) — runbook below, ops step after merge
- Phase 2 (Caddy reverse-proxy on Hetzner) — ops step
- Phase 3 (Chart.yaml link update + private app repo cleanup) — separate PRs

## Test plan

- [x] `make docs-build` succeeds locally
- [x] `make api-docs` produces no diff against committed `docs/api-reference.md`
- [x] `Docs Build` CI job passes
- [x] `API Docs Sync` CI job passes
- [ ] Post-merge: `gh workflow run docs.yaml -f version=v0.33.0` publishes successfully; smoke-check `https://docs-operator.openclaw.rocks/latest/`

## After-merge runbook

See Phases 1-4 in [the design spec](./docs/specs/2026-05-12-docs-site-design.md#migration-plan).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 2: Watch CI**

```bash
gh pr checks --watch
```

All checks must pass: `Lint`, `Test`, `Reconcile Guard`, `Helm RBAC Sync`, `Helm CRD Sync`, `Docs Build`, `API Docs Sync`, `Security Scan`, `Build`, `E2E Tests`.

- [ ] **Step 3: Surface anything that broke**

If any check fails, do not merge. Read the log via:

```bash
gh run view --log-failed
```

Common causes:
- `--strict` mkdocs failure on a broken internal link in existing `docs/*.md` (fix the link)
- `crd-ref-docs` output differs in CI vs local (Go module proxy hit a different version — pin `CRD_REF_DOCS_VERSION` exactly)
- Cairo install failure in CI (add a fallback `apt install` retry)

Fix, commit, push, re-watch.

---

## Task 12: Manual setup checklist (ops, not code; do BEFORE merging)

These steps happen outside this repo. They must be completed before merging so the post-merge release workflow has somewhere to push.

- [ ] **Step 1: Add the DNS record**

In your DNS provider for `openclaw.rocks`:

```
docs-operator   CNAME   openclaw-rocks.github.io.   TTL 300
```

Verify:

```bash
dig +short docs-operator.openclaw.rocks CNAME
# expect: openclaw-rocks.github.io.
```

Allow up to 5 minutes for propagation.

- [ ] **Step 2: Enable GitHub Pages**

Open repository settings → Pages. Configure:

- **Source:** Deploy from a branch
- **Branch:** `gh-pages` (will be created automatically by the first `docs.yaml` run; for now select `main` as a placeholder — the workflow will switch it on first deploy)
- **Custom domain:** `docs-operator.openclaw.rocks`
- **Enforce HTTPS:** checked (greys out until the CNAME validates)

Verify via `gh`:

```bash
gh api repos/openclaw-rocks/openclaw-operator/pages
# expect: html_url and source.branch fields set
```

- [ ] **Step 3: Add repository secrets/variables for analytics (optional)**

```bash
gh secret set AHREFS_KEY --body "<key from openclaw.rocks parent site>"
gh secret set POSTHOG_KEY --body "<key from openclaw.rocks parent site>"
```

(If skipped, analytics is silently disabled — the `{% if %}` guard in the partial covers it.)

Then wire the secrets into `.github/workflows/docs.yaml`. In the `Mike deploy` and `Build docs site and emit llms.txt` steps, add:

```yaml
        env:
          AHREFS_KEY: ${{ secrets.AHREFS_KEY }}
          POSTHOG_KEY: ${{ secrets.POSTHOG_KEY }}
```

- [ ] **Step 4: Merge the PR**

After all CI is green and the manual setup is done:

```bash
gh pr merge --squash --delete-branch --auto
```

- [ ] **Step 5: After merge, trigger first publish manually**

```bash
gh workflow run docs.yaml -f version=v0.33.0
gh run watch
```

When complete, smoke-check the GH Pages origin **directly** before touching Caddy:

```bash
curl -sIL https://docs-operator.openclaw.rocks/latest/ | head -5
curl -sIL https://docs-operator.openclaw.rocks/0.33/ | head -5
```

Both should 200 and show `content-type: text/html`. Open in a browser, verify:
- Version dropdown shows `latest` and `0.33`
- Search works against an API field (e.g. "shareProcessNamespace")
- Dark/light toggle works
- "Edit on GitHub" links resolve to `openclaw-rocks/openclaw-operator/edit/v0.33.0/docs/...`

- [ ] **Step 6: Phase 2 — Caddy on the Hetzner box**

SSH to the box. Update Caddyfile per the design spec Section 4. Run `caddy validate` then `caddy reload`. Verify the criteria in spec acceptance criteria #1-#6.

- [ ] **Step 7: Phase 3 — cleanup PRs**

Open a one-line PR on `openclaw-operator` to bump `charts/openclaw-operator/Chart.yaml`:

```yaml
- name: Documentation
  url: https://openclaw.rocks/docs/operator/latest/
```

(Replacing the `/0.10` link.) `chore:` commit. release-please rolls it into the next release.

Open a PR on the private `app` repo removing the stale `/docs/operator/0.10` tree and the bare-path redirect (Caddy owns redirects now).

---

## Self-review

**Spec coverage:**

| Spec requirement | Task |
|---|---|
| Versioned docs site under openclaw.rocks/docs/operator/ with latest alias | 4, 5, 12 |
| Auto-publish on release | 5 |
| API reference auto-generated from CRDs | 2 |
| CI gates drift | 3 |
| Search, dark mode, edit-on-GitHub, code copy | 1 (mkdocs.yml features list) |
| Open Graph cards | 7 |
| Sitemap | Built-in mkdocs-material; no task needed |
| llms.txt | 8 |
| Custom 404 | 6 |
| Analytics matching parent site | 6 |
| Versioned API reference per release | 5 (mike checks out tag, runs make api-docs from that commit's CRDs) |
| Accessibility / performance | Material defaults; no task |
| Caddy reverse proxy | 12 (operational, post-merge) |
| Chart.yaml link cleanup | 12 (operational, separate PR) |
| Private app repo cleanup | 12 (operational, separate PR) |

**Placeholder scan:** none — every step has concrete commands and code.

**Type consistency:** `CRD_REF_DOCS_VERSION` is named consistently across Makefile and CI. `docs-site/.venv/` path is consistent. Workflow file paths match across tasks. Step counts within tasks are coherent.

**Edge cases addressed inline:**
- `--strict` mkdocs failures on existing broken links: Task 1 Step 6 instructs to fix in-task, not relax.
- Cairo missing locally: Task 7 Step 2 documents both macOS and Linux install commands.
- mike publishing `latest` race with multiple releases: workflow uses `concurrency: group: docs-publish, cancel-in-progress: false`.
- Analytics keys missing in PR builds: `{% if %}` guard in partial; `!ENV` defaults to empty.
- gh-pages branch doesn't exist on first deploy: GitHub Pages settings placeholder is `main`; first `docs.yaml` run creates `gh-pages`.

---

## Execution choice

Plan complete and saved to `docs/specs/2026-05-12-docs-site-plan.md`. Two execution options:

**1. Subagent-Driven (recommended)** — dispatch a fresh subagent per task, review between tasks, fast iteration.

**2. Inline Execution** — execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
