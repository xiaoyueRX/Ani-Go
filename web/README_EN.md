# Ani-Go Web Frontend

Management dashboard frontend for Ani-Go, built with Vue3 + Vite + TypeScript.

## Tech Stack

- **Framework**: Vue 3.5+ (Composition API + `<script setup>`)
- **Build Tool**: Vite 8
- **Language**: TypeScript 6
- **CSS**: TailwindCSS v4 + DaisyUI v5 (dark theme)
- **Router**: Vue Router 4 (History mode + JWT route guard)
- **HTTP**: Axios (interceptor auto-injects Bearer Token, 401 redirect)

## Development

```bash
cd web

# Install dependencies (first time)
npm install

# Start dev server (HMR)
npm run dev

# Production build (output to dist/)
npm run build

# Type check
vue-tsc -b
```

Dev server runs at `http://localhost:5173`. API requests should be proxied to the Go backend (configure proxy in `vite.config.ts`).

## Project Structure

```
web/
├── src/
│   ├── App.vue              # Root component
│   ├── main.ts              # Entry point (mount App + Router)
│   ├── style.css            # Global styles (Tailwind + DaisyUI)
│   ├── router/index.ts      # Route definitions (login / subscriptions / downloads / settings)
│   ├── utils/request.ts     # Axios wrapper (token injection + 401 interception)
│   ├── components/          # Reusable components
│   │   └── SubscriptionEditForm.vue  # Subscription edit form
│   └── views/               # Page components
│       ├── Login.vue        # Login page
│       ├── Layout.vue       # Layout shell (sidebar nav + top bar)
│       ├── Subscriptions.vue      # Subscription list
│       ├── SubscriptionDetail.vue # Subscription detail + episodes
│       ├── SubscriptionForm.vue   # New subscription form
│       ├── Downloads.vue    # Download queue
│       └── SettingsPage.vue # Settings management
├── index.html               # HTML entry point
├── vite.config.ts           # Vite configuration
├── tsconfig.json            # TypeScript configuration
└── package.json             # Dependencies and scripts
```

## Production Deployment

The frontend build output (`dist/`) is embedded into the Go binary via `//go:embed`. No separate deployment is needed. See the project root README.md for details.

## Notes

- Under DaisyUI v5 + Node 24, `@plugin "daisyui"` in CSS fails. Use `@import "daisyui/daisyui.css"` in `style.css` instead.
- `index.html` sets `data-theme="dark"` for DaisyUI dark mode.
- Vue Router uses History mode. The Go backend's static file handler falls back to `index.html` for all non-`/api/*` paths.
