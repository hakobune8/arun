import { chromium } from 'playwright'

const baseURL = trimRightSlash(process.env.AGENTOS_EVAL_LIVE_URL || '')
const storageState = process.env.AGENTOS_EVAL_AUTH_STORAGE_STATE || ''
const cookieHeader = process.env.AGENTOS_EVAL_AUTH_COOKIE || ''
const outputDir = process.env.AGENTOS_EVAL_AUTH_E2E_OUT || ''
const headless = process.env.AGENTOS_EVAL_AUTH_E2E_HEADLESS !== 'false'

if (!baseURL) {
  throw new Error('AGENTOS_EVAL_LIVE_URL is required')
}
if (!storageState && !cookieHeader) {
  throw new Error('AGENTOS_EVAL_AUTH_STORAGE_STATE or AGENTOS_EVAL_AUTH_COOKIE is required')
}

const checks = []
const artifacts = { url: baseURL }
const browser = await chromium.launch({ headless })

try {
  const contextOptions = { ignoreHTTPSErrors: true }
  if (storageState) contextOptions.storageState = storageState
  const context = await browser.newContext(contextOptions)
  if (cookieHeader) {
    await context.addCookies(cookiesFromHeader(cookieHeader, baseURL))
  }

  await runCheck('desktop', 'authenticated session', async () => {
    const page = await context.newPage()
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.goto(baseURL, { waitUntil: 'networkidle' })
    const session = await page.evaluate(async () => {
      const response = await fetch('/api/auth/session', { credentials: 'include' })
      return response.json()
    })
    if (!session.authenticated) throw new Error('session is not authenticated')
    await expectNoPageOverflow(page)
    await page.close()
  })

  await runCheck('desktop', 'main navigation', async () => {
    const page = await context.newPage()
    await page.setViewportSize({ width: 1280, height: 900 })
    await page.goto(baseURL, { waitUntil: 'networkidle' })
    await clickAndExpect(page, 'button', /Orchestrate/, /Repository|Task|Agents/)
    await clickAndExpect(page, 'button', /Schedules/, /Schedules/)
    await clickAndExpect(page, 'button', /Storage/, /Storage/)
    await clickAndExpect(page, 'button', /Agents/, /Agents/)
    await clickAndExpect(page, 'button', /Audit/, /Audit/)
    await expectNoPageOverflow(page)
    if (outputDir) {
      await page.screenshot({ path: `${outputDir}/auth-e2e-desktop.png`, fullPage: true })
      artifacts.desktopScreenshot = `${outputDir}/auth-e2e-desktop.png`
    }
    await page.close()
  })

  await runCheck('mobile', 'bottom navigation layout', async () => {
    const page = await context.newPage()
    await page.setViewportSize({ width: 390, height: 844 })
    await page.goto(baseURL, { waitUntil: 'networkidle' })
    for (const label of ['Run', 'Sched', 'Storage', 'Agents', 'Audit']) {
      await page.getByRole('button', { name: label }).click()
      await page.waitForTimeout(100)
      await expectNoPageOverflow(page)
      await expectBottomNavStable(page)
    }
    if (outputDir) {
      await page.screenshot({ path: `${outputDir}/auth-e2e-mobile.png`, fullPage: true })
      artifacts.mobileScreenshot = `${outputDir}/auth-e2e-mobile.png`
    }
    await page.close()
  })
} finally {
  await browser.close()
}

console.log(JSON.stringify({ checks, artifacts }, null, 2))

async function runCheck(page, action, fn) {
  const started = Date.now()
  try {
    await fn()
    checks.push({ page, action, passed: true, durationMs: Date.now() - started })
  } catch (error) {
    checks.push({ page, action, passed: false, durationMs: Date.now() - started, failure: String(error?.message || error) })
  }
}

async function clickAndExpect(page, role, name, expectedText) {
  await page.getByRole(role, { name }).first().click()
  await page.getByText(expectedText).first().waitFor({ timeout: 5000 })
}

async function expectNoPageOverflow(page) {
  const metrics = await page.evaluate(() => ({
    innerWidth: window.innerWidth,
    docWidth: document.documentElement.scrollWidth,
    bodyWidth: document.body.scrollWidth,
  }))
  const overflow = Math.max(metrics.docWidth, metrics.bodyWidth) - metrics.innerWidth
  if (overflow > 1) throw new Error(`horizontal page overflow: ${JSON.stringify(metrics)}`)
}

async function expectBottomNavStable(page) {
  const result = await page.evaluate(() => {
    const nav = document.querySelector('nav.fixed.inset-x-0.bottom-0')
    if (!nav) return { ok: false, reason: 'bottom nav not found' }
    const buttons = Array.from(nav.querySelectorAll('button'))
    const boxes = buttons.map((button) => {
      const rect = button.getBoundingClientRect()
      const label = button.querySelector('span')?.getBoundingClientRect()
      return {
        text: button.textContent?.trim(),
        left: rect.left,
        right: rect.right,
        top: rect.top,
        bottom: rect.bottom,
        labelLeft: label?.left ?? rect.left,
        labelRight: label?.right ?? rect.right,
      }
    })
    for (let i = 0; i < boxes.length; i++) {
      const box = boxes[i]
      if (box.labelLeft < box.left - 1 || box.labelRight > box.right + 1) {
        return { ok: false, reason: `${box.text} label overflows its button` }
      }
      const next = boxes[i + 1]
      if (next && box.right > next.left + 1) {
        return { ok: false, reason: `${box.text} overlaps ${next.text}` }
      }
    }
    return { ok: true }
  })
  if (!result.ok) throw new Error(result.reason)
}

function cookiesFromHeader(header, url) {
  const parsedURL = new URL(url)
  return header.split(';')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => {
      const index = part.indexOf('=')
      if (index <= 0) throw new Error('invalid cookie header')
      return {
        name: part.slice(0, index),
        value: part.slice(index + 1),
        domain: parsedURL.hostname,
        path: '/',
        httpOnly: true,
        secure: parsedURL.protocol === 'https:',
        sameSite: 'Lax',
      }
    })
}

function trimRightSlash(value) {
  return value.trim().replace(/\/+$/, '')
}
