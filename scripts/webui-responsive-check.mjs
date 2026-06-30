import { createServer } from 'node:http';
import { readFile } from 'node:fs/promises';
import { mkdir } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const html = await readFile(join(root, 'internal/server/static/index.html'), 'utf8');

const longRepo = 'https://github.com/kazyamaz200/agentos-example-with-a-very-long-repository-name.git';
const longTask = 'Investigate a long-running orchestration on a narrow mobile screen, preserve readable task text, logs, diffs, repository URLs, and action buttons without causing full-page horizontal overflow.';
const detail = {
  id: 'run-0123456789abcdef',
  status: 'running',
  repo: longRepo,
  repoPath: '/workspace/agentos-example-with-a-very-long-repository-name',
  baseBranch: 'feature/very-long-branch-name-for-mobile-layout-checks',
  strategy: 'sequential',
  llmPreset: 'default',
  task: longTask,
  agents: ['go-backend', 'reviewer'],
  events: [
    { timestamp: new Date().toISOString(), type: 'created', message: 'Orchestration created' },
    { timestamp: new Date().toISOString(), type: 'planning.finished', message: 'Planning finished with 2 subtasks' },
    { timestamp: new Date().toISOString(), type: 'subtask.started', subtaskId: 'step-1', message: 'go-backend started' },
  ],
  plan: {
    subtasks: [
      { id: 'step-1', agent_type: 'go-backend', description: longTask },
      { id: 'step-2', agent_type: 'reviewer', description: 'Review generated artifacts and summarize follow-up risks.' },
    ],
  },
  subtasks: [
    { id: 'step-1', agent_type: 'go-backend', description: longTask, status: 'running' },
    { id: 'step-2', agent_type: 'reviewer', description: 'Review generated artifacts and summarize follow-up risks.', status: 'pending' },
  ],
  results: [
    {
      subtask_id: 'step-1',
      success: false,
      error: 'example failure with a deliberately-long-token-that-should-wrap-instead-of-stretching-the-page',
      output: 'log line '.repeat(80),
      diff: '+'.repeat(120) + '\n-'.repeat(120),
    },
  ],
  github: {
    branchName: 'agentos/run-0123456789abcdef-with-long-generated-branch-name',
    issueUrl: 'https://github.com/kazyamaz200/agentos/issues/191',
    issueNumber: 191,
  },
};

const server = createServer((req, res) => {
  const url = new URL(req.url || '/', 'http://127.0.0.1');
  res.setHeader('Content-Type', 'application/json');
  if (url.pathname === '/') {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.end(html);
    return;
  }
  if (url.pathname === '/api/auth/session') {
    res.end(JSON.stringify({ authRequired: false, authenticated: true }));
    return;
  }
  if (url.pathname === '/api/settings/llm') {
    res.end(JSON.stringify({ defaultPreset: 'default', presets: [{ id: 'default', name: 'Default', model: 'gpt-5', provider: 'openai', baseUrl: 'https://api.openai.com', keyConfigured: true }] }));
    return;
  }
  if (url.pathname === '/api/agents') {
    res.end(JSON.stringify([{ Name: 'go-backend', Version: '1.0', Description: 'Go backend implementation agent', RequiredTools: ['go', 'git'] }, { Name: 'reviewer', Version: '1.0', Description: 'Code review agent', RequiredTools: ['go'] }]));
    return;
  }
  if (url.pathname === '/api/orchestrates') {
    res.end(JSON.stringify([detail]));
    return;
  }
  if (url.pathname === `/api/orchestrates/${detail.id}`) {
    res.end(JSON.stringify(detail));
    return;
  }
  if (url.pathname === '/api/audit') {
    res.end(JSON.stringify([]));
    return;
  }
  res.statusCode = 404;
  res.end(JSON.stringify({ error: 'not found' }));
});

await new Promise(resolveListen => server.listen(0, '127.0.0.1', resolveListen));
const { port } = server.address();
const baseURL = `http://127.0.0.1:${port}`;
if (process.argv.includes('--serve-only')) {
  console.log(baseURL);
  await new Promise(() => {});
}

const screenshotDir = resolve(process.env.AGENTOS_RESPONSIVE_OUT || '/tmp/agentos-webui-responsive');
await mkdir(screenshotDir, { recursive: true });

const { chromium } = await import('playwright');
const browser = await chromium.launch();
try {
  for (const viewport of [
    { name: 'mobile', width: 390, height: 844 },
    { name: 'desktop', width: 1280, height: 900 },
  ]) {
    const page = await browser.newPage({ viewport: { width: viewport.width, height: viewport.height } });
    await page.goto(baseURL, { waitUntil: 'networkidle' });
    await assertNoPageOverflow(page, `${viewport.name}:new`);
    await page.screenshot({ path: join(screenshotDir, `${viewport.name}-new.png`), fullPage: true });

    await page.getByRole('button', { name: 'List' }).click();
    await page.getByText(detail.id).click();
    await page.waitForSelector('#orchestrateDetail');
    await assertNoPageOverflow(page, `${viewport.name}:detail`);
    await page.screenshot({ path: join(screenshotDir, `${viewport.name}-detail.png`), fullPage: true });

    await page.getByRole('button', { name: 'Runs' }).click();
    await assertNoPageOverflow(page, `${viewport.name}:runs`);
    await page.screenshot({ path: join(screenshotDir, `${viewport.name}-runs.png`), fullPage: true });

    await page.getByText('Agents').click();
    await assertNoPageOverflow(page, `${viewport.name}:agents`);
    await page.screenshot({ path: join(screenshotDir, `${viewport.name}-agents.png`), fullPage: true });
    await page.close();
  }
  console.log(`responsive checks passed; screenshots: ${screenshotDir}`);
} finally {
  await browser.close();
  server.close();
}

async function assertNoPageOverflow(page, label) {
  const metrics = await page.evaluate(() => ({
    innerWidth: window.innerWidth,
    docWidth: document.documentElement.scrollWidth,
    bodyWidth: document.body.scrollWidth,
  }));
  const overflow = Math.max(metrics.docWidth, metrics.bodyWidth) - metrics.innerWidth;
  if (overflow > 1) {
    throw new Error(`${label} has horizontal page overflow: ${JSON.stringify(metrics)}`);
  }
}
