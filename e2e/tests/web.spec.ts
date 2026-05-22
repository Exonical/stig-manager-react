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
    await expect(page.getByTestId('collections-list-page')).toBeVisible()
    await expect(page.getByTestId('collections-search')).toBeVisible()
  })

  test('creates a Collection through the New Collection dialog', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/collections`)
    await expect(page.getByTestId('collections-list-page')).toBeVisible()

    // Unique name avoids collisions if the test is rerun against a
    // dirty database (the API rejects duplicate names).
    const name = `e2e-coll-${Date.now()}`

    await page.getByTestId('new-collection-button').click()
    const dialog = page.getByTestId('new-collection-dialog')
    await expect(dialog).toBeVisible()

    await dialog.getByTestId('new-collection-name-input').fill(name)
    await dialog
      .getByTestId('new-collection-description-input')
      .fill('created from the M18b playwright suite')
    await dialog.getByTestId('new-collection-submit').click()

    // Dialog closes and the SPA navigates to the new collection's
    // detail page; header heading shows the chosen name.
    await expect(page).toHaveURL(/\/collections\/\d+$/)
    await expect(
      page.getByRole('heading', { name }),
    ).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('collection-detail-page')).toBeVisible()

    // Tabs render and the Overview tab is active by default.
    await expect(page.getByTestId('collection-tab-overview')).toBeVisible()
    await expect(page.getByTestId('collection-role-badge')).toContainText(
      /owner/i,
    )

    // Back navigation lands on the list page and the new collection
    // shows up there too.
    await page.getByRole('link', { name: /back to collections/i }).click()
    await expect(page).toHaveURL(/\/collections$/)
    await expect(
      page.getByTestId('collections-table').getByText(name),
    ).toBeVisible({ timeout: 10_000 })
  })
})
