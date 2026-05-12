# Design: Versioned docs site at openclaw.rocks/docs/operator/

**Status:** Approved (brainstorming, 2026-05-12)
**Owner:** stubbi
**Authoring agent:** Claude Code (Opus 4.7)

## Problem

Operator documentation lives in `docs/*.md`, `README.md`, `ROADMAP.md`, and
`CHANGELOG.md`, but the only public rendering today is at
`https://openclaw.rocks/docs/operator/0.10` -- a snapshot of v0.10 baked into
the private `app` (Astro) repo and never updated. Current operator version is
v0.33.0, so the public docs are 23 minor versions out of date. The api-reference
markdown is also hand-maintained against the CRD types and has drifted at
least once already.

We need versioned operator documentation served under `openclaw.rocks/docs/operator/`,
auto-published from this repo on every release, with the api-reference
generated from the CRDs so it cannot drift.

## Goals

- Versioned docs (per minor release) under `openclaw.rocks/docs/operator/<version>/` with a `latest` alias.
- Fully automated release flow, parallel to the Helm chart release: tag the
  operator -> docs site publishes -> Chart-style "no human in the loop".
- api-reference auto-generated from CRD types; CI fails if a CRD-type change
  ships without regenerating the doc.
- Best-practice surface: search, dark mode, edit-on-GitHub, Open Graph cards,
  sitemap, `llms.txt`, code copy buttons, accessibility, mobile responsive.
- Preserve the established `openclaw.rocks/docs/operator/*` URL pattern
  (Chart.yaml and external references continue to work via redirects).

## Non-goals

- Multi-product docs site for openclaw.rocks. This design is operator-only;
  peer products (`/docs/cli/`, `/docs/skills/`) get their own docs sites and
  reverse-proxy entries when they need them.
- Backfilling docs for v0.10 - v0.32. Those tags pre-date `docs-site/`, so the
  build cannot work against them without ports that would mislead readers.
- i18n. The parent Astro site signals many locales in meta tags; docs ship
  English-only initially, addable later.
- PR previews. GH Pages does not natively support them; we ship downloadable
  build artifacts on PRs instead and rely on `make docs-serve` for local
  iteration.

## Architecture overview

```
                       openclaw-rocks/openclaw-operator
                       +-----------------------------+
                       |  docs/*.md, README, ROADMAP |
                       |  api/v1alpha1/*.go (CRDs)   |
                       |  docs-site/mkdocs.yml       |
                       +--------------+--------------+
                                      |
                                      | release: published
                                      v
                       +-----------------------------+
                       | docs.yaml workflow (CI)     |
                       | mike deploy <ver> latest    |
                       +--------------+--------------+
                                      |
                                      v
                       +-----------------------------+
                       |  gh-pages branch            |
                       |  /0.33/  /0.34/  /latest/   |
                       +--------------+--------------+
                                      |
                       served by GH Pages on custom domain
                                      |
                                      v
                       +-----------------------------+
                       | docs-operator.openclaw.rocks|
                       | (CNAME -> github.io)        |
                       +--------------+--------------+
                                      ^
                                      | reverse_proxy
                                      |
                       +-----------------------------+
            user ----->|  Caddy on Hetzner box       |
        openclaw.rocks |  /docs/operator/* path      |
                       |  Astro app for everything   |
                       |  else                       |
                       +-----------------------------+
```

User-facing URL is always `openclaw.rocks/docs/operator/<version>/...`. The
GH Pages origin (`docs-operator.openclaw.rocks`) is an implementation detail
visible only in proxy config and to anyone running `dig`.

## Repo layout

```
openclaw-operator/
+-- docs/
|   +-- api-reference.md             (AUTO-GENERATED from CRDs)
|   +-- architecture.md              (existing)
|   +-- deployment.md                (existing)
|   +-- troubleshooting.md           (existing)
|   +-- custom-providers.md          (existing)
|   +-- external-secrets.md          (existing)
|   +-- model-fallback.md            (existing)
|   +-- specs/                       (this directory; excluded from docs site nav)
|   +-- images/, monitoring/, runbooks/
+-- docs-site/
|   +-- mkdocs.yml
|   +-- crd-ref-docs.yaml            (config for the generator)
|   +-- requirements.txt             (pinned mkdocs-material, mike, plugins)
|   +-- overrides/                   (theme tweaks, custom 404, analytics partial)
+-- README.md                        (surfaced into docs via include-markdown)
+-- ROADMAP.md                       (surfaced into docs)
+-- CHANGELOG.md                     (surfaced into docs)
+-- Makefile                         (new targets: api-docs, docs-serve, docs-build)
+-- .github/workflows/docs.yaml      (new: release-triggered publish)
+-- .github/workflows/ci.yaml        (extended: Docs Build, API Docs Sync jobs)
+-- charts/openclaw-operator/Chart.yaml  (one-line update: docs link -> /latest/)
```

Single source of truth: existing `docs/*.md` files are inputs. `README.md`,
`ROADMAP.md`, `CHANGELOG.md` are included into the site via
`mkdocs-include-markdown-plugin` so they remain canonical at their current
paths and don't need duplication.

## Framework

Material for MkDocs, with these plugins:

- `mike` -- versioning, version dropdown, `latest` alias.
- `search` -- built-in Lunr-backed instant search (Day 1); replaced by
  Algolia DocSearch in a Phase 2 once approved (free for OSS, ~1-2 week review).
- `include-markdown` -- surfaces README / ROADMAP / CHANGELOG without forking.
- `awesome-pages` -- auto-builds nav from filesystem; exclude `specs/`,
  `images/`, `runbooks/`, `monitoring/`.
- `git-revision-date-localized` -- "last updated" footer.
- `minify` (HTML), and Material's `social` plugin for per-page OG cards.

Theme overrides: brand palette (parent site primary `#050810`), custom 404,
analytics partial wiring Ahrefs + PostHog (already used on the parent Astro
site, so docs share visitor identity).

## API reference generation

- Tool: `crd-ref-docs` (Elastic). Same tool Cilium, KubeVirt, KEDA use.
- Config at `docs-site/crd-ref-docs.yaml`.
- New Makefile target:

  ```makefile
  api-docs: manifests $(CRD_REF_DOCS)
      $(CRD_REF_DOCS) --config docs-site/crd-ref-docs.yaml \
        --source-path api/v1alpha1 --output-path docs/api-reference.md \
        --renderer markdown
  ```

- Runs after `make manifests` so the CRD YAML is fresh first.
- CI job `API Docs Sync` fails the PR if regenerated output differs from
  committed `docs/api-reference.md` -- identical enforcement pattern to the
  existing `Helm CRD Sync` and `Helm RBAC Sync` jobs.

## Build pipeline (CI)

Two new jobs added to `.github/workflows/ci.yaml`, running in parallel with
existing jobs on every PR:

| Job             | Fails the PR when                                          |
|-----------------|------------------------------------------------------------|
| Docs Build      | `mkdocs build --strict` errors (broken internal links, missing pages, malformed YAML) |
| API Docs Sync   | `docs/api-reference.md` differs from `make api-docs` output |

Build time budget: Docs Build ~30s; API Docs Sync ~15s. Negligible against
the existing 5-10 min e2e job.

## Release pipeline

New file `.github/workflows/docs.yaml`. Triggers:

- `release: published` (chained off the existing release pipeline that already
  publishes the GH Release via `RELEASE_PLEASE_TOKEN`).
- `workflow_dispatch` with a `version` input for one-shot republishes.

Per-trigger steps:

1. Check out the *released tag* (not main) -- docs frozen at that commit's
   markdown state.
2. Install pinned mkdocs deps.
3. Extract `MAJOR.MINOR` from tag (`v0.33.0` -> `0.33`).
4. `mike deploy --push --update-aliases <minor> latest --title "v<full>"`.
5. `mike set-default --push latest` -- `gh-pages` root redirects to `/latest/`.

Concurrency: `group: docs-publish`, `cancel-in-progress: false`. Never
cancel a mid-flight deploy; sequential releases are rare and safe.

Permissions: `contents: write` is sufficient (default `GITHUB_TOKEN`); no PAT
needed because `gh-pages` is a leaf target and does not trigger downstream
workflows.

## URL & reverse-proxy strategy

### Origin

- DNS: `CNAME docs-operator.openclaw.rocks -> openclaw-rocks.github.io.`
- GH Pages: enabled on `openclaw-operator` repo, source `gh-pages` branch,
  custom domain `docs-operator.openclaw.rocks`, enforce HTTPS.
- Mike writes a `CNAME` file to the `gh-pages` root via `extra.cname` in
  `mkdocs.yml`, so each deploy reasserts the custom domain.

### `mkdocs.yml` URL config

```yaml
site_url: https://openclaw.rocks/docs/operator/   # user-facing canonical
extra:
  cname: docs-operator.openclaw.rocks
```

`site_url` controls every internal link, `<link rel=canonical>`, OG, and
sitemap entry -- they all resolve to the user-facing host, not the origin.
Search engines see one canonical site; no duplicate-content penalty.

### Caddy (Hetzner box, in front of the Astro app)

```caddyfile
openclaw.rocks {
    # Bare path -> latest
    redir /docs/operator /docs/operator/latest/ 301
    redir /docs/operator/ /docs/operator/latest/ 301

    # Legacy 0.10 paths (Chart.yaml had this hardcoded) -> latest
    redir /docs/operator/0.10 /docs/operator/latest/ 301
    redir /docs/operator/0.10/* /docs/operator/latest{uri} 301

    # All other /docs/operator/* -> GH Pages origin (path-preserving)
    handle_path /docs/operator/* {
        reverse_proxy https://docs-operator.openclaw.rocks {
            header_up Host docs-operator.openclaw.rocks
            header_up X-Forwarded-Host openclaw.rocks
            header_down Cache-Control "public, max-age=300, s-maxage=600"
            header_down -Server
        }
    }

    # Existing Astro app handles everything else
    reverse_proxy localhost:4321
}
```

Notes:

- `handle_path` strips `/docs/operator/` so the request hits the origin at
  `/0.33/foo` -- exactly where mike published.
- Cache TTL of 300s/600s matches the Astro app's existing strategy and lets
  a release go live within ~10 min if a CDN later sits in front.
- `X-Forwarded-Host` so any future origin-side logic sees the real hostname.

## Best-practices polish

- **Search.** Day 1: Material built-in (Lunr, client-side). Phase 2: apply
  for Algolia DocSearch; swap a 6-line config.
- **`llms.txt`.** Generated post-build from the nav tree at
  `/docs/operator/latest/llms.txt` -- helps grounding by AI agents.
- **Open Graph / Twitter cards.** Material's `social` plugin renders branded
  per-page cards at build time. Matches parent site's existing OG strategy.
- **Analytics.** Drop the same Ahrefs + PostHog snippets from the parent
  Astro site via `overrides/partials/integrations/analytics.html`. Single
  visitor identity across marketing site and docs.
- **Custom 404** with search bar + links to home, latest API ref, GitHub
  issues. Mike handles unknown-version 404s with the version dropdown
  rendered alongside the error.
- **PR previews via build artifact.** CI uploads the built site as a
  workflow artifact when `docs/`, `README.md`, `ROADMAP.md`, or
  `CHANGELOG.md` changes. Local preview via `make docs-serve`.
- **Versioned API reference per release.** `make api-docs` runs inside the
  release workflow against the released tag's CRDs, so `/0.33/api-reference/`
  reflects the v0.33 CRDs exactly. No cross-version pollution.
- **Accessibility & performance.** Material defaults: WCAG-AA palette,
  `prefers-reduced-motion`, minified HTML/CSS/JS, lazy-loaded images,
  fingerprinted assets for long-cache safety.

## Migration plan

Phased so the current `/docs/operator/0.10` URL never breaks before the new
path is live.

### Phase 0: foundation PR (no user-visible change)

- Add `docs-site/`, mkdocs config, Makefile targets, the two new CI jobs,
  and the `docs.yaml` workflow. Gated behind `release:published` +
  `workflow_dispatch`; no auto-publish on merge.
- Replace `docs/api-reference.md` with `crd-ref-docs` output. Large diff,
  reviewable as a regenerate-and-stash.
- DNS: add `CNAME docs-operator.openclaw.rocks -> openclaw-rocks.github.io`.
  TTL 5 min.
- Enable GH Pages on the repo with custom domain and enforce-HTTPS.

### Phase 1: first publish, origin-only validation

- `gh workflow run docs.yaml -f version=v0.33.0`. Mike publishes `0.33` and
  `latest`.
- Smoke check directly against the origin (`https://docs-operator.openclaw.rocks/latest/`)
  before touching any reverse proxy. Verify version dropdown, search,
  dark mode, edit-on-GitHub, OG card preview.

### Phase 2: flip the front door

- Apply the Caddyfile changes on the Hetzner box. Reload Caddy
  (`caddy reload`, zero downtime).
- Verify in order: bare `/docs/operator/`, `/docs/operator/latest/`,
  `/docs/operator/0.10` redirect, `/docs/operator/0.33/api-reference/`,
  full browser session including search.
- Rollback: revert Caddyfile, reload. ~30 second blast radius.

### Phase 3: cleanup

- PR on `openclaw-operator`: bump `Chart.yaml` docs link from
  `/docs/operator/0.10` -> `/docs/operator/latest/`. `chore:` commit, ships
  with the next release-please cycle naturally.
- PR on the private `app` repo: remove the stale `/docs/operator/0.10`
  page tree and the `/docs/operator -> /0.10` redirect (Caddy owns that now).

### Phase 4: auto-publish proven on next release

- Next conventional commit -> release-please cuts v0.34.0 -> release publishes
  -> `docs.yaml` fires -> new version live at
  `openclaw.rocks/docs/operator/0.34/` and `latest` alias flipped. Zero
  human action required.

## Acceptance criteria

| #  | Criterion |
|----|-----------|
| 1  | `openclaw.rocks/docs/operator/latest/` returns v0.33.0 docs |
| 2  | `openclaw.rocks/docs/operator/0.33/` returns frozen v0.33 docs |
| 3  | `openclaw.rocks/docs/operator/` -> 301 -> `/docs/operator/latest/` |
| 4  | `openclaw.rocks/docs/operator/0.10` (and `/0.10/*`) -> 301 -> `/docs/operator/latest/` |
| 5  | Version dropdown renders, includes `latest` + `0.33`, switches versions correctly |
| 6  | "Edit on GitHub" links resolve to the released tag, not main |
| 7  | API reference matches the CRD `kubectl get crd openclawinstances.openclaw.rocks -o yaml` field-for-field |
| 8  | A PR with a CRD-type change but no `docs/api-reference.md` update fails `API Docs Sync` |
| 9  | A PR with a broken internal markdown link fails `Docs Build` (`--strict`) |
| 10 | The release immediately following the implementation PR auto-publishes new docs with no manual step |
| 11 | `mkdocs serve` works locally for contributors against committed sources |
| 12 | `make api-docs` regenerates `docs/api-reference.md` deterministically |

## Risks & mitigations

| Risk | Likelihood | Mitigation |
|---|---|---|
| `crd-ref-docs` output format differs across versions | Medium | Pin `crd-ref-docs` version in Makefile via `go install`; commit the binary version into the toolchain pins |
| GH Pages origin downtime hides /docs/operator/* | Low | Caddy `lb_try_duration` short fallback to a static "docs temporarily unavailable" page; not blocking on Day 1 but trackable |
| Caddy reload fails on Hetzner box (config syntax) | Low | `caddy validate` in CI for the Caddyfile change PR before applying |
| Old 0.10 deep links in third-party blog posts | Medium | Wildcard 301 from `/0.10/*` to `/latest/{uri}` already in design; covers any prior path shape |
| mike on `gh-pages` race condition with concurrent releases | Low | `concurrency: group: docs-publish, cancel-in-progress: false` in `docs.yaml` |
| Algolia DocSearch rejected / delayed | Low | Day 1 ships with built-in Lunr search; swap is additive when DocSearch arrives |

## Open questions deferred

None blocking. Future considerations (logged here so they don't get lost):

- Add `/docs/cli/` for the `kubectl-openclaw` plugin once that repo wants a
  matching docs flow.
- Federated search across operator + CLI + skills sites if/when peer products land.
- Move origin from GH Pages to Cloudflare Pages if global latency becomes a
  user complaint; the `site_url`-based design means swapping origin only
  touches Caddy + DNS, not the build.

## Time budget

| Phase | Effort |
|-------|--------|
| 0 -- foundation PR (mkdocs + crd-ref-docs + 2 CI jobs + docs workflow + theme overrides) | 1 day |
| 1 -- first publish + origin smoke | 30 minutes |
| 2 -- Caddy update + verification | 30 minutes |
| 3 -- cleanup PRs (Chart.yaml + app repo) | 15 minutes |
| 4 -- verify next release auto-flow | 0 (happens on its own) |

## References

- Material for MkDocs: https://squidfunk.github.io/mkdocs-material/
- Mike (versioning): https://github.com/jimporter/mike
- crd-ref-docs: https://github.com/elastic/crd-ref-docs
- Brainstorm transcript: this session (2026-05-12)
- Related repo state: https://github.com/openclaw-rocks/openclaw-operator at v0.33.0
- Closed prior investigation that informed scope: this is greenfield; no prior issue.
