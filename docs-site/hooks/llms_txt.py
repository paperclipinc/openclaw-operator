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
