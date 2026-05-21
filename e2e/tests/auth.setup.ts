import { test as setup, expect } from '@playwright/test'

import { urls } from '../playwright.config'

const STORAGE_FILE = 'storage/admin.json'

/**
 * One-shot Playwright auth setup. We perform the real browser-driven
 * OIDC flow against the local Keycloak instance — click "Sign in",
 * fill the login form, then wait for the SPA to reflect the
 * signed-in state. The resulting localStorage (including the
 * oidc-client-ts user record with `access_token`) is persisted to
 * `storage/admin.json` and reused by every other project via
 * `storageState`.
 */
setup('authenticate admin', async ({ page }) => {
  await page.goto(urls.web)

  // Wait for the SPA bootstrap to be done — Sign in renders only
  // after Env.js + the auth provider have initialised.
  await expect(page.getByRole('button', { name: /sign in/i })).toBeVisible()

  await Promise.all([
    page.waitForURL(/realms\/stigman\/protocol\/openid-connect/),
    page.getByRole('button', { name: /sign in/i }).click(),
  ])

  // Keycloak's default login form.
  await page.locator('#username').fill('admin')
  await page.locator('#password').fill('admin')
  await page.locator('#kc-login').click()

  // First login may present an "Update Account Information" form
  // because the realm seed leaves first/last name blank. Race the
  // form against the post-login SPA redirect so we don't time out
  // waiting for either deterministic outcome.
  const updateHeading = page.getByRole('heading', { name: /update account information/i })
  await Promise.race([
    updateHeading.waitFor({ state: 'visible', timeout: 30_000 }).catch(() => null),
    page.waitForURL(new RegExp('^' + urls.web), { timeout: 30_000 }).catch(() => null),
  ])
  if (await updateHeading.isVisible().catch(() => false)) {
    await page.locator('#firstName').fill('Admin')
    await page.locator('#lastName').fill('User')
    await page.getByRole('button', { name: /submit/i }).click()
  }

  // Back on the SPA — the auth provider should now report signed-in.
  await page.waitForURL(new RegExp('^' + urls.web), { timeout: 30_000 })
  await expect(page.getByText(/signed in as/i)).toBeVisible({ timeout: 30_000 })

  // Sanity: an OIDC user record exists in localStorage.
  const haveUser = await page.evaluate(() => {
    for (let i = 0; i < window.localStorage.length; i++) {
      const k = window.localStorage.key(i) ?? ''
      if (k.startsWith('oidc.user:')) return true
    }
    return false
  })
  expect(haveUser, 'oidc-client-ts did not persist a user record').toBe(true)

  await page.context().storageState({ path: STORAGE_FILE })
})
