# Ani-Go web frontend

[简体中文](README.md) · [Installation](../README_EN.md)

The dashboard uses Vue 3, Vue Router, TypeScript, Vite and pure Tailwind CSS v4 primitives. It ships 3 official theme presets (4 dark/light color styles each) and supports validated import/export of third-party `anigo-theme-v1` JSON theme packs, synced via `/api/settings` from Settings → Themes & Appearance or the quick Theme Studio drawer.

The plugins section in Settings handles manifest import, enablement and provenance filters. `PluginPanel.vue` renders credentials, actions and task lists from an Ani-Go panel declaration. [Author guide](../docs/02_PLUGIN_SPEC_EN.md) · [Import example](../examples/plugins/cloud-tasks.json)

## Local development

Run these commands in the repository's `web` directory with Node 24.

```sh
npm ci
npm run dev
```

The lockfile uses the public npm registry. To install from an organization mirror, pass `--registry` to `npm ci` or set `registry` in your local npm configuration.

Vite serves the UI at `http://localhost:5173` by default and proxies `/api` to `http://localhost:20001`. Use the running Go server's administrator account to sign in. The client in `src/utils/request.ts` attaches the JWT and handles expired sessions.

Browser tabs on the same origin share the login session. Signing out in one tab takes other management pages back to sign-in. Signing in again restores each tab’s previous internal page. The backend creates the administrator account on first startup.

When subscribing from a release in search results, review the original release title and the suggested series title, season, group and resolution. You can supply a series RSS feed. Selecting multiple episodes of one series and season produces one subscription. Conflicting group or resolution choices are left open, and failed submissions remain in the dialog for review. Both Mikan series pickers and the Schedule modal use multi-select subgroup checkboxes confirmed via the bottom "Subscribe" button, and subscribed rows link directly to the series detail page.

The Schedule quick modal and `/subscriptions/new` share a unified onboarding flow: Schedule lets you check one or more Mikan/common subgroups before clicking "Subscribe"; if none are checked, it matches your persisted default subgroups and prompts you to choose when none of your defaults exist on that series; the AI quick parser can hand off prefilled rules to `/subscriptions/new`; and `/subscriptions/new` offers both manual configuration and paste-to-preview RSS parsing, visual resolution/include/exclude/subgroup rule chips, and a live destination path preview.

Episode rows show the actual download or organization status alongside an optional manual mark. Use the selector to set or clear a mark, or select multiple episodes to apply changes together. Subscription details expose a one-click "Supplement Now" action, a confirmation delete dialog with optional local file cleanup, an active rule summary bar, and episode status/keyword filters.

Subscription details refresh every 30 seconds while visible and when you return to the tab. Background refresh pauses while editing or changing episodes. Refreshing Bangumi authorization in Settings preserves local unsaved changes.

In Downloads, Enter submits the new-download form. Escape dismisses an idle dialog and returns focus to its opening control. When auto-refresh is on, only visible tabs poll the download queue, and each poll waits for any active read to finish.

## Tests and production build

```sh
npm test
npm run typecheck
npm run build
```

`npm run build` checks types and writes `web/dist`. Building the Go binary from the repository root embeds those assets. The UI reads the running server version from `/api/health`. [Build configuration](vite.config.ts) · [Routes](src/router/index.ts) · [HTTP reference](../docs/API.md)
