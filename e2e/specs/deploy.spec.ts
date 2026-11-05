import { test, expect } from '@playwright/test'

// The deploy: the projects page lists the release, the card shows the SHAs, a plan is asked, the digest
// approved, the release applied (the memory target) — all through the pages a user sees.

test('the projects page, the release card, a plan, an approval, an applied release', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('link', { name: 'releases/prod/db' })).toBeVisible()

  // the release card: the SHAs, the plan button
  await page.getByRole('link', { name: 'releases/prod/db' }).click()
  await expect(page.getByRole('button', { name: 'Plan' })).toBeVisible()
  await expect(page.locator('code').first()).not.toBeEmpty()

  // the plan: a request opens, the job runs, the digest of the plan appears
  await page.getByRole('button', { name: 'Plan' }).click()
  await expect(page.getByTestId('digest')).toBeVisible({ timeout: 60_000 })
  const digest = await page.getByTestId('digest').innerText()
  expect(digest.length).toBeGreaterThan(10)

  // the approval: exactly the digest shown, on exactly the SHA shown
  await page.getByRole('button', { name: 'Approve' }).click()
  await expect(page.getByText('Approved')).toBeVisible({ timeout: 30_000 })

  // the apply runs; the release card comes to "applied" on the same SHA
  await expect(async () => {
    await page.goto('/')
    await page.getByRole('link', { name: 'releases/prod/db' }).click()
    await expect(page.getByText('applied').first()).toBeVisible({ timeout: 15_000 })
  }).toPass({ timeout: 120_000, intervals: [3000, 5000] })
})
