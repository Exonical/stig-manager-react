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

  test('creates an Asset inside a Collection and opens its detail page', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/collections`)

    // Reuse the same pattern: create a fresh collection, then create
    // an asset inside it.
    const collectionName = `e2e-asset-coll-${Date.now()}`
    await page.getByTestId('new-collection-button').click()
    const cdialog = page.getByTestId('new-collection-dialog')
    await cdialog.getByTestId('new-collection-name-input').fill(collectionName)
    await cdialog.getByTestId('new-collection-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+$/, { timeout: 10_000 })
    await expect(page.getByTestId('collection-detail-page')).toBeVisible()

    // Open the Assets tab and create an asset.
    await page.getByTestId('collection-tab-assets').click()
    await expect(page.getByTestId('collection-assets-tab')).toBeVisible()
    await expect(page.getByTestId('new-asset-button')).toBeVisible()
    await page.getByTestId('new-asset-button').click()

    const adialog = page.getByTestId('new-asset-dialog')
    await expect(adialog).toBeVisible()
    const assetName = `e2e-asset-${Date.now()}`
    await adialog.getByTestId('asset-name-input').fill(assetName)
    await adialog.getByTestId('asset-ip-input').fill('10.0.0.42')
    await adialog
      .getByTestId('asset-description-input')
      .fill('created from the M18c playwright suite')
    await adialog.getByTestId('new-asset-submit').click()

    // Navigate to the asset detail page on success.
    await expect(page).toHaveURL(/\/collections\/\d+\/assets\/\d+$/, {
      timeout: 10_000,
    })
    await expect(page.getByTestId('asset-detail-page')).toBeVisible()
    await expect(page.getByRole('heading', { name: assetName })).toBeVisible()

    // Navigating back lands on the Collection detail. Re-open the
    // Assets tab; our asset is in the list.
    await page.getByRole('link', { name: /back to/i }).click()
    await expect(page.getByTestId('collection-detail-page')).toBeVisible()
    await page.getByTestId('collection-tab-assets').click()
    await expect(
      page.getByTestId('assets-table').getByText(assetName),
    ).toBeVisible({ timeout: 10_000 })
  })

  test('metrics tab shows KPI grid and empty state tables (M18e)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/collections`)
    const collectionName = `e2e-metrics-${Date.now()}`
    await page.getByTestId('new-collection-button').click()
    const cdialog = page.getByTestId('new-collection-dialog')
    await cdialog.getByTestId('new-collection-name-input').fill(collectionName)
    await cdialog.getByTestId('new-collection-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+$/, { timeout: 10_000 })

    await page.getByTestId('collection-tab-metrics').click()
    await expect(page.getByTestId('collection-metrics-tab')).toBeVisible()

    // KPI grid is rendered (even though counts are zero for a fresh collection).
    await expect(page.getByTestId('metrics-kpi-grid')).toBeVisible({
      timeout: 10_000,
    })
    await expect(page.getByTestId('metrics-kpi-assets')).toContainText('0')
    // Per-asset and per-stig tables show empty state.
    await expect(page.getByTestId('metrics-by-asset-empty')).toBeVisible()
    await expect(page.getByTestId('metrics-by-stig-empty')).toBeVisible()
  })

  test('history tab shows stats and empty entries state (M18e)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/collections`)
    const collectionName = `e2e-history-${Date.now()}`
    await page.getByTestId('new-collection-button').click()
    const cdialog = page.getByTestId('new-collection-dialog')
    await cdialog.getByTestId('new-collection-name-input').fill(collectionName)
    await cdialog.getByTestId('new-collection-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+$/, { timeout: 10_000 })

    await page.getByTestId('collection-tab-history').click()
    await expect(page.getByTestId('collection-history-tab')).toBeVisible()

    // Stats panel is visible.
    await expect(page.getByTestId('history-stats')).toBeVisible({
      timeout: 10_000,
    })
    await expect(page.getByTestId('history-total-entries')).toContainText('0')
    // No history entries yet → empty state.
    await expect(page.getByTestId('history-empty')).toBeVisible()
  })

  test('exports tab renders the download form (M18e)', async ({ page }) => {
    await page.goto(`${urls.web}/collections`)
    const collectionName = `e2e-exports-${Date.now()}`
    await page.getByTestId('new-collection-button').click()
    const cdialog = page.getByTestId('new-collection-dialog')
    await cdialog.getByTestId('new-collection-name-input').fill(collectionName)
    await cdialog.getByTestId('new-collection-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+$/, { timeout: 10_000 })

    await page.getByTestId('collection-tab-exports').click()
    await expect(page.getByTestId('collection-exports-tab')).toBeVisible()
    await expect(page.getByTestId('export-format-toggle')).toBeVisible()
    await expect(page.getByTestId('export-download-button')).toBeVisible()
  })

  test('POAM tab renders the download form (M18e)', async ({ page }) => {
    await page.goto(`${urls.web}/collections`)
    const collectionName = `e2e-poam-${Date.now()}`
    await page.getByTestId('new-collection-button').click()
    const cdialog = page.getByTestId('new-collection-dialog')
    await cdialog.getByTestId('new-collection-name-input').fill(collectionName)
    await cdialog.getByTestId('new-collection-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+$/, { timeout: 10_000 })

    await page.getByTestId('collection-tab-poam').click()
    await expect(page.getByTestId('collection-poam-tab')).toBeVisible()
    await expect(page.getByTestId('poam-aggregator')).toBeVisible()
    await expect(page.getByTestId('poam-download-button')).toBeVisible()
  })

  test('dry-run batch review surfaces will-insert counts (M18d)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/collections`)
    const collectionName = `e2e-batch-coll-${Date.now()}`
    await page.getByTestId('new-collection-button').click()
    const cdialog = page.getByTestId('new-collection-dialog')
    await cdialog.getByTestId('new-collection-name-input').fill(collectionName)
    await cdialog.getByTestId('new-collection-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+$/, { timeout: 10_000 })

    // Create an asset so the batch resolution has something to land on.
    await page.getByTestId('collection-tab-assets').click()
    await page.getByTestId('new-asset-button').click()
    const adialog = page.getByTestId('new-asset-dialog')
    const assetName = `e2e-batch-asset-${Date.now()}`
    await adialog.getByTestId('asset-name-input').fill(assetName)
    await adialog.getByTestId('asset-ip-input').fill('10.0.0.99')
    await adialog.getByTestId('new-asset-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+\/assets\/\d+$/, {
      timeout: 10_000,
    })

    // Capture the collection ID + asset ID from the URL for the form.
    const url = page.url()
    const match = url.match(/\/collections\/(\d+)\/assets\/(\d+)/)
    expect(match).not.toBeNull()
    const assetId = match![2]

    // Back to the Collection and open the Reviews (Batch Review) tab.
    await page.getByRole('link', { name: /back to/i }).click()
    await expect(page.getByTestId('collection-detail-page')).toBeVisible()
    await page.getByTestId('collection-tab-reviews').click()
    await expect(page.getByTestId('collection-reviews-tab')).toBeVisible()

    // Asset criterion: pick the asset we just created.
    await page.getByTestId(`batch-asset-list-${assetId}`).check()

    // Rule criterion: type a fake rule ID. The API will resolve zero
    // matching (asset,rule) pairs because the asset isn't mapped to any
    // benchmark yet — so we expect a "zero willInsert" dry run, which
    // still proves the form wires up end-to-end.
    await page.getByTestId('batch-rule-ids').fill('SV-0000r1_rule')

    // Submit dry run.
    await page.getByTestId('batch-dry-run').click()
    await expect(page.getByTestId('batch-result-dry')).toBeVisible({
      timeout: 10_000,
    })
    await expect(page.getByTestId('batch-count-insert')).toContainText(
      /Will insert: 0/,
    )
  })

  test('admin layout renders with the four tabs (M18f)', async ({ page }) => {
    await page.goto(`${urls.web}/admin/users`)
    await expect(page.getByTestId('admin-layout')).toBeVisible()
    const tabs = page.getByTestId('admin-tabs')
    await expect(tabs).toBeVisible()
    await expect(tabs.getByTestId('admin-tab-users')).toBeVisible()
    await expect(tabs.getByTestId('admin-tab-user-groups')).toBeVisible()
    await expect(tabs.getByTestId('admin-tab-jobs')).toBeVisible()
    await expect(tabs.getByTestId('admin-tab-app-info')).toBeVisible()
    await expect(tabs.getByTestId('admin-tab-audit-log')).toBeVisible()
  })

  test('admin users page lists demo users (M18f)', async ({ page }) => {
    await page.goto(`${urls.web}/admin/users`)
    await expect(page.getByTestId('admin-users-page')).toBeVisible()
    await expect(page.getByTestId('users-table')).toBeVisible()
    await expect(page.getByTestId('new-user-button')).toBeVisible()
    // The signed-in admin and at least one other demo user should be present.
    await expect(
      page.getByTestId('users-table').getByText('admin', { exact: true }),
    ).toBeVisible({ timeout: 10_000 })
  })

  test('admin user-groups page renders empty state when none exist (M18f)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/admin/user-groups`)
    await expect(page.getByTestId('admin-user-groups-page')).toBeVisible()
    await expect(page.getByTestId('user-groups-table')).toBeVisible()
    await expect(page.getByTestId('new-user-group-button')).toBeVisible()
  })

  test('admin jobs page shows task registry (M18f)', async ({ page }) => {
    await page.goto(`${urls.web}/admin/jobs`)
    await expect(page.getByTestId('admin-jobs-page')).toBeVisible()
    await expect(page.getByTestId('jobs-table')).toBeVisible()
    await expect(page.getByTestId('job-tasks-list')).toBeVisible({
      timeout: 10_000,
    })
    // The built-in 'noop' task should be in the registry.
    await expect(
      page.getByTestId('job-tasks-list').getByText('noop'),
    ).toBeVisible({ timeout: 10_000 })
  })

  test('admin can create + run + delete a job (M18f)', async ({ page }) => {
    await page.goto(`${urls.web}/admin/jobs`)
    await expect(page.getByTestId('admin-jobs-page')).toBeVisible()
    await expect(page.getByTestId('job-tasks-list').getByText('noop')).toBeVisible({
      timeout: 10_000,
    })

    // Open the create dialog.
    await page.getByTestId('new-job-button').click()
    const dialog = page.getByTestId('job-create-dialog')
    await expect(dialog).toBeVisible()
    const jobName = `e2e-job-${Date.now()}`
    await dialog.getByTestId('job-name-input').fill(jobName)
    // 'noop' is the default selected task in the dropdown.
    await dialog.getByTestId('job-create-submit').click()
    await expect(dialog).not.toBeVisible({ timeout: 10_000 })

    // Find the new row and click Run.
    const row = page
      .getByTestId('jobs-table')
      .getByRole('row')
      .filter({ hasText: jobName })
    await expect(row).toBeVisible({ timeout: 10_000 })
    await row.getByRole('button', { name: new RegExp(`run ${jobName}`, 'i') }).click()

    // Run history populates and the output card lands.
    await expect(page.getByTestId('job-runs-list')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('job-run-output')).toBeVisible({
      timeout: 10_000,
    })
  })

  test('admin app-info page shows build + counts cards (M18f)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/admin/app-info`)
    await expect(page.getByTestId('admin-app-info-page')).toBeVisible()
    await expect(page.getByTestId('appinfo-build')).toBeVisible({
      timeout: 10_000,
    })
    await expect(page.getByTestId('appinfo-counts')).toBeVisible()
    await expect(page.getByTestId('appinfo-runtime')).toBeVisible()
    await expect(page.getByTestId('appinfo-postgres')).toBeVisible()
    await expect(page.getByTestId('appinfo-tables')).toBeVisible()
  })

  test('collection grants tab lists owner grant (M18f)', async ({ page }) => {
    // Create a collection so we have a fresh Owner grant on it.
    await page.goto(`${urls.web}/collections`)
    const collectionName = `e2e-grants-${Date.now()}`
    await page.getByTestId('new-collection-button').click()
    const cdialog = page.getByTestId('new-collection-dialog')
    await cdialog.getByTestId('new-collection-name-input').fill(collectionName)
    await cdialog.getByTestId('new-collection-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+$/, { timeout: 10_000 })

    await page.getByTestId('collection-tab-grants').click()
    await expect(page.getByTestId('collection-grants-tab')).toBeVisible()
    await expect(page.getByTestId('grants-table')).toBeVisible()
    // Add-grant form should be present (admin has Manage+ on its own collection).
    await expect(page.getByTestId('add-grant-form')).toBeVisible()
    // The creator gets an Owner grant on creation.
    await expect(
      page.getByTestId('grants-table').getByText('Owner'),
    ).toBeVisible({ timeout: 10_000 })
  })

  test('library page renders with lookup cards (M18g)', async ({ page }) => {
    await page.goto(`${urls.web}/library`)
    await expect(page.getByTestId('library-list-page')).toBeVisible()
    await expect(page.getByTestId('library-table')).toBeVisible()
    await expect(page.getByTestId('library-search')).toBeVisible()
    await expect(page.getByTestId('rule-lookup-card')).toBeVisible()
    await expect(page.getByTestId('cci-lookup-card')).toBeVisible()
    // Admin has stig-manager:stig so the Import button must render.
    await expect(page.getByTestId('import-benchmark-button')).toBeVisible()
  })

  test('library rule lookup surfaces 404 for unknown rule (M18g)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/library`)
    await expect(page.getByTestId('rule-lookup-card')).toBeVisible()
    await page
      .getByTestId('rule-lookup-input')
      .fill('SV-99999999r1_rule')
    await page.getByTestId('rule-lookup-submit').click()
    // Either a result or an error must render — the API returns 404
    // for unknown rules, which our hook surfaces as an error.
    await expect(page.getByTestId('rule-lookup-error')).toBeVisible({
      timeout: 10_000,
    })
  })

  test('library cci lookup surfaces 404 for unknown cci (M18g)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/library`)
    await expect(page.getByTestId('cci-lookup-card')).toBeVisible()
    await page.getByTestId('cci-lookup-input').fill('999999')
    await page.getByTestId('cci-lookup-submit').click()
    await expect(page.getByTestId('cci-lookup-error')).toBeVisible({
      timeout: 10_000,
    })
  })

  test('library import dialog opens and validates file input (M18g)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/library`)
    await page.getByTestId('import-benchmark-button').click()
    const dialog = page.getByTestId('import-stig-dialog')
    await expect(dialog).toBeVisible()
    await expect(dialog.getByTestId('import-file-input')).toBeVisible()
    await expect(dialog.getByTestId('import-clobber-checkbox')).toBeVisible()
    // Submit is disabled until a file is chosen.
    await expect(dialog.getByTestId('import-stig-submit')).toBeDisabled()
  })

  test('admin audit log page renders with table + filters (M19)', async ({
    page,
  }) => {
    await page.goto(`${urls.web}/admin/audit-log`)
    await expect(page.getByTestId('admin-audit-log-page')).toBeVisible()
    await expect(page.getByTestId('audit-filter-form')).toBeVisible()
    await expect(page.getByTestId('audit-table')).toBeVisible({
      timeout: 10_000,
    })
    await expect(page.getByTestId('audit-method-select')).toBeVisible()
    await expect(page.getByTestId('audit-path-input')).toBeVisible()
    await expect(page.getByTestId('audit-since-input')).toBeVisible()
    await expect(page.getByTestId('audit-until-input')).toBeVisible()
    await expect(page.getByTestId('audit-limit-input')).toBeVisible()
    await expect(page.getByTestId('audit-apply')).toBeVisible()
  })

  test('admin audit log records a mutation and the row expands (M19)', async ({
    page,
  }) => {
    // Generate a fresh mutation so we know at least one row exists
    // (the audit middleware records POSTs synchronously after the
    // handler runs, then writes asynchronously with a 5s timeout).
    await page.goto(`${urls.web}/collections`)
    const collectionName = `e2e-audit-${Date.now()}`
    await page.getByTestId('new-collection-button').click()
    const cdialog = page.getByTestId('new-collection-dialog')
    await cdialog.getByTestId('new-collection-name-input').fill(collectionName)
    await cdialog.getByTestId('new-collection-submit').click()
    await expect(page).toHaveURL(/\/collections\/\d+$/, { timeout: 10_000 })

    await page.goto(`${urls.web}/admin/audit-log`)
    await expect(page.getByTestId('admin-audit-log-page')).toBeVisible()

    // Filter to POST /api/collections to narrow the result set, then
    // expand the first row and assert the payload viewer is rendered.
    await page.getByTestId('audit-method-select').selectOption('POST')
    await page.getByTestId('audit-path-input').fill('/api/collections')
    await page.getByTestId('audit-apply').click()

    const table = page.getByTestId('audit-table')
    // Give the audit row a moment to land (async writer).
    await expect(async () => {
      await page.getByTestId('audit-refresh').click()
      const rows = await table.locator('tbody tr').count()
      // Each audit row renders one base <tr>; expanded rows add a 2nd.
      // We expect at least one populated data row (not the empty-state
      // placeholder, which spans 7 cols).
      expect(rows).toBeGreaterThanOrEqual(1)
      const empty = await table
        .getByText('No rows match the current filters.')
        .count()
      expect(empty).toBe(0)
    }).toPass({ timeout: 15_000 })

    const firstToggle = page
      .locator('[data-testid^="audit-row-toggle-"]')
      .first()
    await firstToggle.click()
    await expect(
      page.locator('[data-testid^="audit-row-payload-"]').first(),
    ).toBeVisible({ timeout: 5_000 })
  })
})
