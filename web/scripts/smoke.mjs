import { spawn } from 'node:child_process'
import { mkdir } from 'node:fs/promises'
import { chromium } from 'playwright'

const port = 4173
const preview = spawn('npm', ['run', 'preview', '--', '--port', String(port)], {
  stdio: ['ignore', 'pipe', 'pipe'],
})

let output = ''
preview.stdout.on('data', (chunk) => {
  output += chunk.toString()
})
preview.stderr.on('data', (chunk) => {
  output += chunk.toString()
})

function wait(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

async function waitForPreview() {
  for (let i = 0; i < 60; i += 1) {
    try {
      const res = await fetch(`http://127.0.0.1:${port}/`)
      if (res.ok) return
    } catch {
      // keep waiting
    }
    await wait(500)
  }
  throw new Error(`vite preview did not start:\n${output}`)
}

async function checkViewport(browser, name, width, height) {
  const page = await browser.newPage({ viewport: { width, height } })
  await page.goto(`http://127.0.0.1:${port}/`, { waitUntil: 'networkidle' })
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth)
  if (overflow > 1) {
    throw new Error(`${name} has horizontal overflow of ${overflow}px`)
  }
  await page.screenshot({ path: `../tmp/webui-${name}.png`, fullPage: true })
  await page.close()
}

try {
  await mkdir('../tmp', { recursive: true })
  await waitForPreview()
  const browser = await chromium.launch()
  await checkViewport(browser, 'mobile', 390, 844)
  await checkViewport(browser, 'desktop', 1440, 1000)
  await browser.close()
} finally {
  preview.kill('SIGTERM')
}
