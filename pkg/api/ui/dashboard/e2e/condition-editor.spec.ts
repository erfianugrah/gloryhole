import { test, expect, type Page } from "@playwright/test";

// ─── Helpers ────────────────────────────────────────────────────────

/** Navigate to policies page and wait for it to load. */
async function goToPolicies(page: Page) {
  await page.goto("/policies");
  // Wait for React hydration — the Add Policy button indicates the page is ready
  await page.waitForSelector('[data-testid="add-policy"]', { timeout: 15000 });
}

/** Open the Create Policy dialog. */
async function openCreateDialog(page: Page) {
  await page.click('[data-testid="add-policy"]');
  await page.waitForSelector('[data-testid="condition-editor"]', {
    timeout: 5000,
  });
}

/** Helper to clean up any policies we created, so tests are independent. */
async function deleteAllPolicies(page: Page) {
  // Use the API directly to avoid UI state issues
  const resp = await page.request.get("/api/policies");
  if (!resp.ok()) return;
  const data = await resp.json();
  const policies = data.policies ?? [];
  // Delete in reverse order since IDs are indices
  for (let i = policies.length - 1; i >= 0; i--) {
    await page.request.delete(`/api/policies/${policies[i].id}`);
  }
}

// ─── Visual Builder Default Mode ────────────────────────────────────

test.describe("Policy Dialog — Visual Builder", () => {
  test.beforeEach(async ({ page }) => {
    await goToPolicies(page);
  });

  test("visual builder is the default mode", async ({ page }) => {
    await openCreateDialog(page);
    await expect(page.locator('[data-testid="condition-editor"]')).toBeVisible();
    await expect(page.locator('[data-testid="toggle-builder"]')).toContainText(
      "Text editor",
    );
  });

  test("can toggle between visual builder and text editor", async ({ page }) => {
    await openCreateDialog(page);
    await expect(page.locator('[data-testid="condition-editor"]')).toBeVisible();

    // Switch to text editor
    await page.click('[data-testid="toggle-builder"]');
    await expect(page.locator('[data-testid="logic-textarea"]')).toBeVisible();
    await expect(page.locator('[data-testid="condition-editor"]')).not.toBeVisible();

    // Switch back to visual builder
    await page.click('[data-testid="toggle-builder"]');
    await expect(page.locator('[data-testid="condition-editor"]')).toBeVisible();
  });

  test("default condition row has Domain / equals", async ({ page }) => {
    await openCreateDialog(page);
    const row = page.locator('[data-testid="condition-row"]').first();
    await expect(row.locator('[data-testid="field-select"]')).toContainText("Domain");
    await expect(row.locator('[data-testid="operator-select"]')).toContainText("equals");
  });

  test("generated expression updates as value changes", async ({ page }) => {
    await openCreateDialog(page);
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="value-input"]').fill("example.com");
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      'Domain == "example.com"',
    );
  });
});

// ─── Operator Expression Generation ─────────────────────────────────

test.describe("Operator expression generation", () => {
  test.beforeEach(async ({ page }) => {
    await goToPolicies(page);
    await openCreateDialog(page);
  });

  test("contains generates DomainMatches", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^contains$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("ads");
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      'DomainMatches(Domain, "ads")',
    );
  });

  test("not contains generates negated DomainMatches", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^not contains$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("tracking");
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      '!DomainMatches(Domain, "tracking")',
    );
  });

  test("not starts with generates negated DomainStartsWith", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^not starts with$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("ads");
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      '!DomainStartsWith(Domain, "ads")',
    );
  });

  test("not ends with generates negated DomainEndsWith", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^not ends with$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("evil.com");
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      '!DomainEndsWith(Domain, ".evil.com")',
    );
  });

  test("not matches (regex) generates negated DomainRegex", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^not matches \(regex\)$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("^ads\\.");
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      '!DomainRegex(Domain, "^ads\\.")',
    );
  });

  test("ClientIP not in CIDR generates negated IPInCIDR", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="field-select"]').click();
    await page.locator('[role="option"]', { hasText: "Client IP" }).click();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^not in CIDR$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("10.0.0.0/8");
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      '!IPInCIDR(ClientIP, "10.0.0.0/8")',
    );
  });

  test("Hour >= generates numeric expression", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="field-select"]').click();
    await page.locator('[role="option"]', { hasText: "Hour (0-23)" }).click();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: "at least" }).click();
    await row.locator('[data-testid="value-input"]').fill("22");
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      "Hour >= 22",
    );
  });
});

// ─── AND/OR Separator Toggle ────────────────────────────────────────

test.describe("AND/OR separator toggle", () => {
  test.beforeEach(async ({ page }) => {
    await goToPolicies(page);
    await openCreateDialog(page);
  });

  test("second condition shows AND separator", async ({ page }) => {
    await expect(page.locator('[data-testid="condition-row"]')).toHaveCount(1);
    await expect(page.locator('[data-testid="join-separator"]')).toHaveCount(0);

    await page.locator('[data-testid="add-condition"]').first().click();
    await expect(page.locator('[data-testid="condition-row"]')).toHaveCount(2);
    await expect(page.locator('[data-testid="join-separator"]')).toHaveCount(1);
    await expect(page.locator('[data-testid="join-toggle"]').first()).toContainText("AND");
  });

  test("clicking separator toggles AND/OR and updates expression", async ({ page }) => {
    // Fill first condition
    const row1 = page.locator('[data-testid="condition-row"]').first();
    await row1.locator('[data-testid="value-input"]').fill("a.com");

    // Add second condition
    await page.locator('[data-testid="add-condition"]').first().click();
    const row2 = page.locator('[data-testid="condition-row"]').nth(1);
    await row2.locator('[data-testid="value-input"]').fill("b.com");

    const toggle = page.locator('[data-testid="join-toggle"]').first();
    const expr = page.locator('[data-testid="generated-expression"]');

    // Should start as AND with &&
    await expect(toggle).toContainText("AND");
    await expect(expr).toContainText("&&");

    // Toggle to OR
    await toggle.click();
    await expect(toggle).toContainText("OR");
    await expect(expr).toContainText("||");

    // Toggle back to AND
    await toggle.click();
    await expect(toggle).toContainText("AND");
    await expect(expr).toContainText("&&");
  });

  test("group data-attribute tracks operator", async ({ page }) => {
    await page.locator('[data-testid="add-condition"]').first().click();
    const group = page.locator('[data-testid="condition-group"]').first();
    await expect(group).toHaveAttribute("data-group-op", "AND");

    await page.locator('[data-testid="join-toggle"]').first().click();
    await expect(group).toHaveAttribute("data-group-op", "OR");
  });
});

// ─── Group Dropdown ─────────────────────────────────────────────────

test.describe("Add group dropdown", () => {
  test.beforeEach(async ({ page }) => {
    await goToPolicies(page);
    await openCreateDialog(page);
  });

  test("dropdown shows OR and AND options, no NOT", async ({ page }) => {
    await page.locator('[data-testid="add-group-trigger"]').first().click();
    await expect(page.locator('[data-testid="add-or-group"]')).toBeVisible();
    await expect(page.locator('[data-testid="add-and-group"]')).toBeVisible();
    // NOT should not exist
    await expect(page.locator('[data-testid="add-not-group"]')).toHaveCount(0);
  });

  test("adding OR sub-group creates nested group", async ({ page }) => {
    await page.locator('[data-testid="add-group-trigger"]').first().click();
    await page.click('[data-testid="add-or-group"]');
    const groups = page.locator('[data-testid="condition-group"]');
    await expect(groups).toHaveCount(2);
    await expect(groups.nth(1)).toHaveAttribute("data-group-op", "OR");
  });

  test("nested group produces mixed AND/OR expression", async ({ page }) => {
    // Fill root condition
    const row1 = page.locator('[data-testid="condition-row"]').first();
    await row1.locator('[data-testid="value-input"]').fill("ads.com");

    // Add OR sub-group
    await page.locator('[data-testid="add-group-trigger"]').first().click();
    await page.click('[data-testid="add-or-group"]');

    // Fill first nested condition
    const nestedRow1 = page.locator('[data-testid="condition-row"]').nth(1);
    await nestedRow1.locator('[data-testid="value-input"]').fill("track.com");

    // Add another condition in nested group
    const nested = page.locator('[data-testid="condition-group"]').nth(1);
    await nested.locator('[data-testid="add-condition"]').click();
    const nestedRow2 = page.locator('[data-testid="condition-row"]').nth(2);
    await nestedRow2.locator('[data-testid="value-input"]').fill("spy.com");

    const expr = page.locator('[data-testid="generated-expression"]');
    await expect(expr).toContainText("&&");
    await expect(expr).toContainText("||");
  });
});

// ─── PillsInput ─────────────────────────────────────────────────────

test.describe("PillsInput for list operators", () => {
  test.beforeEach(async ({ page }) => {
    await goToPolicies(page);
    await openCreateDialog(page);
  });

  test("'in list' shows pills input", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^in list$/ }).click();
    await expect(row.locator('[data-testid="pills-input"]')).toBeVisible();
    await expect(row.locator('[data-testid="value-input"]')).toHaveCount(0);
  });

  test("can add pills with Enter and generates Domain in [...]", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^in list$/ }).click();

    const input = row.locator('[data-testid="pills-input"] input');
    await input.fill("example.com");
    await input.press("Enter");
    await input.fill("test.io");
    await input.press("Enter");

    // Verify pills appeared
    const pills = row.locator('[data-testid="pills-input"] .gap-0\\.5');
    await expect(pills).toHaveCount(2);
    await expect(pills.nth(0)).toContainText("example.com");
    await expect(pills.nth(1)).toContainText("test.io");

    // Verify expression
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      'Domain in ["example.com", "test.io"]',
    );
  });

  test("can remove pills", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^in list$/ }).click();

    const input = row.locator('[data-testid="pills-input"] input');
    await input.fill("a.com");
    await input.press("Enter");
    await input.fill("b.com");
    await input.press("Enter");

    const pills = row.locator('[data-testid="pills-input"] .gap-0\\.5');
    await expect(pills).toHaveCount(2);

    // Remove first pill
    await pills.nth(0).locator("button").click();
    await expect(pills).toHaveCount(1);
    await expect(pills.first()).toContainText("b.com");
  });

  test("'not in list' generates negated expression", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^not in list$/ }).click();

    const input = row.locator('[data-testid="pills-input"] input');
    await input.fill("ads.com");
    await input.press("Enter");
    await input.fill("track.io");
    await input.press("Enter");

    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      '!(Domain in ["ads.com", "track.io"])',
    );
  });

  test("QueryType 'in list' generates QueryTypeIn", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="field-select"]').click();
    await page.locator('[role="option"]', { hasText: "Query Type" }).click();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^in list$/ }).click();

    const input = row.locator('[data-testid="pills-input"] input');
    await input.fill("A");
    await input.press("Enter");
    await input.fill("AAAA");
    await input.press("Enter");

    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      'QueryTypeIn(QueryType, "A", "AAAA")',
    );
  });
});

// ─── Field Switching ────────────────────────────────────────────────

test.describe("Field switching", () => {
  test.beforeEach(async ({ page }) => {
    await goToPolicies(page);
    await openCreateDialog(page);
  });

  test("switching from Domain/contains to Hour resets operator to equals", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();

    // Set operator to contains (not valid for numeric)
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^contains$/ }).click();
    await expect(row.locator('[data-testid="operator-select"]')).toContainText("contains");

    // Switch field to Hour
    await row.locator('[data-testid="field-select"]').click();
    await page.locator('[role="option"]', { hasText: "Hour (0-23)" }).click();

    // Operator should reset to equals
    await expect(row.locator('[data-testid="operator-select"]')).toContainText("equals");
  });

  test("ClientIP field shows 4 operators", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="field-select"]').click();
    await page.locator('[role="option"]', { hasText: "Client IP" }).click();
    await row.locator('[data-testid="operator-select"]').click();
    await expect(page.locator('[role="option"]')).toHaveCount(4);
  });

  test("Weekday field shows 6 numeric operators", async ({ page }) => {
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="field-select"]').click();
    await page.locator('[role="option"]', { hasText: "Weekday (0-6)" }).click();
    await row.locator('[data-testid="operator-select"]').click();
    await expect(page.locator('[role="option"]')).toHaveCount(6);
  });
});

// ─── Condition Removal ──────────────────────────────────────────────

test.describe("Condition removal", () => {
  test.beforeEach(async ({ page }) => {
    await goToPolicies(page);
    await openCreateDialog(page);
  });

  test("can remove a condition row", async ({ page }) => {
    await page.locator('[data-testid="add-condition"]').first().click();
    await expect(page.locator('[data-testid="condition-row"]')).toHaveCount(2);

    await page
      .locator('[data-testid="condition-row"]')
      .first()
      .locator('[data-testid="remove-condition"]')
      .click();
    await expect(page.locator('[data-testid="condition-row"]')).toHaveCount(1);
  });

  test("can remove a nested group", async ({ page }) => {
    await page.locator('[data-testid="add-group-trigger"]').first().click();
    await page.click('[data-testid="add-or-group"]');
    await expect(page.locator('[data-testid="condition-group"]')).toHaveCount(2);

    await page.locator('[data-testid="remove-group"]').click();
    await expect(page.locator('[data-testid="condition-group"]')).toHaveCount(1);
  });
});

// ─── Full E2E: Create Policy via API + Verify in Table ──────────────

test.describe("Full E2E: Policy CRUD with real API", () => {
  test.beforeEach(async ({ page }) => {
    await deleteAllPolicies(page);
    await goToPolicies(page);
  });

  test.afterEach(async ({ page }) => {
    await deleteAllPolicies(page);
  });

  test("create policy via visual builder and verify in table", async ({ page }) => {
    await openCreateDialog(page);

    // Fill name
    await page.locator('[data-testid="policy-name"]').fill("Block ads domains");

    // Action = BLOCK (default)

    // Set condition: Domain contains "ads"
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^contains$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("ads");

    // Verify generated expression
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText(
      'DomainMatches(Domain, "ads")',
    );

    // Save
    await page.click('[data-testid="save-policy"]');

    // Wait for dialog to close and table to update
    await page.waitForSelector('[data-testid="policy-row"]', { timeout: 10000 });

    // Verify the policy appears in the table
    const policyRow = page.locator('[data-testid="policy-row"]').first();
    await expect(policyRow).toContainText("Block ads domains");
    await expect(policyRow).toContainText("BLOCK");

    // Verify the summary is human-readable (not raw expression)
    const logic = policyRow.locator('[data-testid="policy-logic"]');
    await expect(logic).toContainText('Domain contains "ads"');
  });

  test("create policy with OR conditions and verify", async ({ page }) => {
    await openCreateDialog(page);

    await page.locator('[data-testid="policy-name"]').fill("Allow DNS services");

    // Change action to ALLOW
    await page.locator('[data-testid="policy-action"]').click();
    await page.locator('[role="option"]', { hasText: "Allow" }).click();

    // First condition: Domain == "icanhazip.com"
    const row1 = page.locator('[data-testid="condition-row"]').first();
    await row1.locator('[data-testid="value-input"]').fill("icanhazip.com");

    // Add second condition
    await page.locator('[data-testid="add-condition"]').first().click();
    const row2 = page.locator('[data-testid="condition-row"]').nth(1);
    await row2.locator('[data-testid="value-input"]').fill("ipinfo.io");

    // Toggle to OR
    await page.locator('[data-testid="join-toggle"]').first().click();
    await expect(page.locator('[data-testid="join-toggle"]').first()).toContainText("OR");

    // Verify expression uses ||
    await expect(page.locator('[data-testid="generated-expression"]')).toContainText("||");

    // Save
    await page.click('[data-testid="save-policy"]');
    await page.waitForSelector('[data-testid="policy-row"]', { timeout: 10000 });

    const policyRow = page.locator('[data-testid="policy-row"]').first();
    await expect(policyRow).toContainText("Allow DNS services");
    await expect(policyRow).toContainText("ALLOW");
  });

  test("edit policy opens with parsed visual builder", async ({ page }) => {
    // Create a policy via API first
    await page.request.post("/api/policies", {
      data: {
        name: "Test edit",
        logic: 'DomainMatches(Domain, "example")',
        action: "BLOCK",
        enabled: true,
      },
    });
    await page.reload();
    await page.waitForSelector('[data-testid="policy-row"]', { timeout: 10000 });

    // Click edit
    await page.locator('[data-testid="edit-policy"]').first().click();
    await page.waitForSelector('[data-testid="condition-editor"]', { timeout: 5000 });

    // The visual builder should be active (parsed the expression)
    await expect(page.locator('[data-testid="condition-editor"]')).toBeVisible();

    // The condition should be parsed: Domain / contains / "example"
    const row = page.locator('[data-testid="condition-row"]').first();
    await expect(row.locator('[data-testid="field-select"]')).toContainText("Domain");
    await expect(row.locator('[data-testid="operator-select"]')).toContainText("contains");
    await expect(row.locator('[data-testid="value-input"]')).toHaveValue("example");
  });

  test("test expression against real backend", async ({ page }) => {
    await openCreateDialog(page);

    // Fill name
    await page.locator('[data-testid="policy-name"]').fill("Test policy");

    // Set condition: Domain contains "example"
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^contains$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("example");

    // Test with matching domain
    await page.locator('[data-testid="test-domain"]').fill("ads.example.com");
    await page.click('[data-testid="test-button"]');
    await expect(page.locator('[data-testid="test-result"]')).toContainText(
      "Expression matched",
      { timeout: 5000 },
    );

    // Test with non-matching domain
    await page.locator('[data-testid="test-domain"]').fill("google.com");
    await page.click('[data-testid="test-button"]');
    await expect(page.locator('[data-testid="test-result"]')).toContainText(
      "Expression did not match",
      { timeout: 5000 },
    );
  });

  test("test negated expression against real backend", async ({ page }) => {
    await openCreateDialog(page);
    await page.locator('[data-testid="policy-name"]').fill("Test negated");

    // Set condition: Domain not contains "google"
    const row = page.locator('[data-testid="condition-row"]').first();
    await row.locator('[data-testid="operator-select"]').click();
    await page.locator('[role="option"]', { hasText: /^not contains$/ }).click();
    await row.locator('[data-testid="value-input"]').fill("google");

    // google.com should NOT match (negated — "not contains google" is false for google.com)
    await page.locator('[data-testid="test-domain"]').fill("google.com");
    await page.click('[data-testid="test-button"]');
    await expect(page.locator('[data-testid="test-result"]')).toContainText(
      "Expression did not match",
      { timeout: 5000 },
    );

    // example.com SHOULD match (doesn't contain "google", so negation is true)
    await page.locator('[data-testid="test-domain"]').fill("example.com");
    await page.click('[data-testid="test-button"]');
    await expect(page.locator('[data-testid="test-result"]')).toContainText(
      "Expression matched",
      { timeout: 5000 },
    );
  });
});
