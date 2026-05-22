import { expect, test, type APIRequestContext } from '@playwright/test'

import { urls } from '../playwright.config'

/**
 * Extract the access_token persisted by oidc-client-ts. The library
 * stores a JSON blob under `oidc.user:<authority>:<client_id>`; we
 * pull the first one we find.
 */
async function readAccessToken(page: import('@playwright/test').Page): Promise<string> {
  return await page.evaluate(() => {
    for (let i = 0; i < window.localStorage.length; i++) {
      const k = window.localStorage.key(i) ?? ''
      if (k.startsWith('oidc.user:')) {
        const raw = window.localStorage.getItem(k) ?? '{}'
        const u = JSON.parse(raw) as { access_token?: string }
        if (u.access_token) return u.access_token
      }
    }
    throw new Error('no oidc.user:* record in localStorage')
  })
}

async function withAuth(
  request: APIRequestContext,
  token: string,
  method: 'get' | 'post' | 'patch' | 'delete',
  path: string,
  init: Parameters<APIRequestContext['post']>[1] = {},
) {
  return request[method](`${urls.api}${path}`, {
    ...init,
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      ...((init as { headers?: Record<string, string> }).headers ?? {}),
    },
  })
}

test.describe('API — anonymous surface', () => {
  test('GET /health returns 200', async ({ request }) => {
    const res = await request.get(`${urls.api}/health`)
    expect(res.status()).toBe(200)
  })

  test('GET /js/Env.js exposes the OIDC config', async ({ request }) => {
    const res = await request.get(`${urls.api}/js/Env.js`)
    expect(res.status()).toBe(200)
    const body = await res.text()
    // Defensive: the SPA reads `window.STIGMAN.Env`, so the script
    // must assign that exact identifier.
    expect(body).toContain('STIGMAN')
    expect(body).toContain('authority')
    expect(body).toMatch(/realms\/stigman/)
  })

  test('GET /api/op/state is open and reports both deps up', async ({ request }) => {
    const res = await request.get(`${urls.api}/api/op/state`)
    expect(res.status()).toBe(200)
    const body = await res.json()
    expect(body.currentState).toBe('available')
    expect(body.dependencies.db).toBe(true)
    expect(body.dependencies.oidc).toBe(true)
  })

  test('GET /api/collections without a token is rejected', async ({ request }) => {
    const res = await request.get(`${urls.api}/api/collections`)
    expect([401, 403]).toContain(res.status())
  })

  test('GET /api/user without a token is rejected', async ({ request }) => {
    const res = await request.get(`${urls.api}/api/user`)
    expect([401, 403]).toContain(res.status())
  })
})

test.describe('API — self-info', () => {
  test('GET /api/user returns the signed-in app_user row', async ({
    page,
    request,
  }) => {
    await page.goto(urls.web)
    const token = await readAccessToken(page)
    const res = await withAuth(request, token, 'get', '/api/user')
    expect(res.status(), await res.text()).toBe(200)
    const body = (await res.json()) as {
      userId: string
      username: string
      privileges?: { admin?: boolean }
    }
    expect(body.userId).toBeTruthy()
    // The admin realm user logs in as `admin` and is auto-upserted
    // on first /api/user hit.
    expect(body.username).toBe('admin')
  })
})

test.describe('API — authenticated golden path', () => {
  test('create user → list → audit', async ({ page, request }) => {
    // Visit the SPA once so the storageState is rehydrated into
    // localStorage — Playwright does not auto-load localStorage for
    // pages that have never been navigated to in the test.
    await page.goto(urls.web)
    const token = await readAccessToken(page)
    expect(token.split('.')).toHaveLength(3) // looks like a JWT

    // Use POST /api/users as the canonical mutation under test: it
    // doesn't depend on bootstrapping a userId for the caller (unlike
    // POST /api/collections which requires owner grants in the
    // payload) and the admin user's token carries the
    // `stig-manager:user` scope.
    const username = `e2e-${Date.now()}`
    const create = await withAuth(request, token, 'post', '/api/users', {
      data: { username },
    })
    expect(create.status(), await create.text()).toBe(201)
    const created = await create.json()
    expect(created.userId).toBeTruthy()
    expect(created.username).toBe(username)

    const list = await withAuth(
      request,
      token,
      'get',
      `/api/users?username=${encodeURIComponent(username)}&username-match=exact`,
    )
    expect(list.status()).toBe(200)
    const rows = (await list.json()) as Array<{ userId: string; username: string }>
    expect(rows.find((r) => r.username === username)).toBeTruthy()

    // The audit middleware records the POST asynchronously; poll the
    // read endpoint until our row shows up.
    let row: { method: string; path: string; payload?: { username?: string } } | undefined
    const deadline = Date.now() + 15_000
    while (Date.now() < deadline) {
      const audit = await withAuth(
        request,
        token,
        'get',
        `/api/op/audit-log?method=POST&path=users&limit=50`,
      )
      expect(audit.status()).toBe(200)
      const entries = (await audit.json()) as Array<{
        method: string
        path: string
        payload?: { username?: string }
      }>
      row = entries.find((e) => e.payload?.username === username)
      if (row) break
      await page.waitForTimeout(250)
    }
    expect(row, 'audit row never appeared for our POST').toBeTruthy()
    expect(row!.method).toBe('POST')
    expect(row!.path).toBe('/api/users')
  })
})
