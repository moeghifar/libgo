# libgo docs site

Static site for https://github.com/moeghifar/libgo: plain HTML/CSS and one small JS file, no build step. Syntax highlighting comes from highlight.js 11.9.0 on cdnjs; everything else is in this directory.

```
docs/
├── index.html            landing: philosophy, package cards, planned packages
├── getting-started.html  full service walkthrough, based on example/
├── envy.html, climd.html, foreverwork.html, xttp.html
├── changelog.html        derived from ../CHANGELOG.md
├── site.css, site.js     theme (dark/light), copy buttons, nav
└── .nojekyll             tells Pages to serve files as-is
```

## Publish on GitHub Pages

No workflow file is needed: Pages can serve this directory straight from the branch.

1. Push `docs/` to `main`.
2. On GitHub: **Settings → Pages**.
3. Under **Build and deployment**, set **Source** to *Deploy from a branch*.
4. Pick branch **main** and folder **/docs**, then **Save**.
5. The site appears at `https://moeghifar.github.io/libgo/` within a minute or two.

Every later push to `main` that touches `docs/` redeploys automatically.

If you prefer the Actions-based flow (needed only for a custom build or a non-`main` branch), set Source to *GitHub Actions* and add a workflow that uses `actions/upload-pages-artifact` with `path: docs` followed by `actions/deploy-pages`.

## Preview locally

```sh
cd docs && python3 -m http.server 8000
# open http://localhost:8000
```

## Updating

Pages are hand-maintained HTML. When a package README or `CHANGELOG.md` changes, update the matching page; all links are relative, so the site works at any path.
