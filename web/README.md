# ARUN Web UI

React + TypeScript + Tailwind CSS frontend for the ARUN Web UI.

## Commands

```bash
npm ci
npm run dev
npm run lint
npm run build
npm run smoke
```

`npm run build` writes production assets to `../internal/server/static` so the
Go server can embed and serve the frontend.
