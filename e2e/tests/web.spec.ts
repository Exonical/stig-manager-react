import { expect, test } from '@playwright/test'

import { urls } from '../playwright.config'

test.describe('Web SPA', () => {
  test('loads and reflects signed-in state', async ({ page }) => {
    await page.goto(urls.web)
    await expect(page.getByText(/signed in as/i)).toBeVisible()
    // The auth header includes a <strong> with the principal name —
    // grab whatever <strong> appears next to "Signed in as" and
    // verify it is non-empty rather than asserting a specific value
    // (Keycloak's account-update step may rewrite the display name).
    const name = await page
      .locator('header strong')
      .first()
      .textContent()
    expect(name?.trim().length ?? 0).toBeGreaterThan(0)
  })

  test('fetch app info button surfaces /api/op/appinfo', async ({ page }) => {
    await page.goto(urls.web)
    await page.getByRole('button', { name: /fetch app info/i }).click()

    // The payload is rendered as a JSON <pre> block; rather than
    // asserting on specific keys (which evolve with M15/M16/…) just
    // confirm the rendering happened by checking for a non-empty pre.
    const pre = page.locator('pre').first()
    await expect(pre).toBeVisible()
    const text = (await pre.textContent()) ?? ''
    expect(text.trim().length).toBeGreaterThan(2)
  })

  test('list collections renders the API response', async ({ page }) => {
    await page.goto(urls.web)
    await page.getByRole('button', { name: /list collections/i }).click()

    const pre = page.locator('pre').last()
    await expect(pre).toBeVisible()
    const text = (await pre.textContent()) ?? ''
    // Either an empty-state message or a JSON array — both are
    // acceptable depending on whether the API test ran first.
    expect(text).toMatch(/(no collections|\[)/i)
  })
})
