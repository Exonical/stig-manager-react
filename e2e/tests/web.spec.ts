import { expect, test } from '@playwright/test'

import { urls } from '../playwright.config'

test.describe('Web SPA', () => {
  test('renders the app shell when signed in', async ({ page }) => {
    await page.goto(urls.web)

    // Topbar reflects the signed-in identity.
    await expect(page.getByText(/signed in as/i)).toBeVisible()
    const name = await page.locator('header strong').first().textContent()
    expect(name?.trim().length ?? 0).toBeGreaterThan(0)

    // Sidebar nav is rendered (Dashboard is always present once signed in).
    const sidebar = page.getByTestId('app-sidebar')
    await expect(sidebar).toBeVisible()
    await expect(sidebar.getByTestId('nav-dashboard')).toBeVisible()
  })

  test('dashboard surfaces /api/op/appinfo', async ({ page }) => {
    await page.goto(urls.web)
    const tile = page.getByTestId('dashboard-appinfo-tile')
    await expect(tile).toBeVisible()
    // The version line lands inside the card description; require a
    // string starting with "API " rendered by the appinfo hook.
    await expect(tile.getByText(/^API /)).toBeVisible({ timeout: 10_000 })
  })

  test('collections nav lands on the collections page', async ({ page }) => {
    await page.goto(urls.web)
    await page
      .getByTestId('app-sidebar')
      .getByTestId('nav-collections')
      .click()
    await expect(page).toHaveURL(/\/collections$/)
    await expect(
      page.getByRole('heading', { name: /collections/i }),
    ).toBeVisible()
  })
})
