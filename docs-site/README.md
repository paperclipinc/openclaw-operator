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

`overrides/` mirrors the Material theme structure. Don't bloat overrides --
prefer config tweaks in `mkdocs.yml` first.
