import { chromium } from 'playwright';

/**
 * This script automates the deployment of a "Command Execution Result" Want
 * through the MyWant Dashboard UI.
 */
async function deployExecutionResult() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    console.log('Step 1: Navigating to MyWant Dashboard...');
    await page.goto('http://localhost:8080/dashboard');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);

    console.log('Step 1b: Resetting state with Escape...');
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
    await page.click('body');

    console.log('Step 2: Opening "Add Want" sidebar...');
    await page.getByRole('banner').getByRole('button', { name: 'Add Want' }).click();

    console.log('Step 3: Searching for "Command" want type...');
    // Clear and type
    await page.evaluate(() => {
      const sidebar = document.querySelector('[data-sidebar="true"]');
      const input = sidebar.querySelector('input[placeholder*="keyword"]');
      if (input) {
        input.value = '';
        input.dispatchEvent(new Event('input', { bubbles: true }));
      }
    });
    await page.keyboard.type('Command', { delay: 50 });
    
    // Wait for the search results to update
    await page.waitForTimeout(1000);

    console.log('Step 4: Selecting "Command Execution Result" want type...');
    // Wait for the search results to actually contain the desired type
    const resultItem = page.locator('button:has-text("Command Execution Result")').first();
    await resultItem.waitFor({ state: 'visible', timeout: 10000 });
    await resultItem.click();
    
    // Wait for the form to update
    await page.waitForTimeout(2000);
    console.log('Form updated, checking for inputs...');

    console.log('Step 5: Configuring parameters (command: date)...');
    
    // Function to find command input
    const findCommandInput = async () => {
      const candidates = page.locator('input, textarea, [role="textbox"]');
      const count = await candidates.count();
      for (let i = 0; i < count; i++) {
        const html = await candidates.nth(i).evaluate(el => el.outerHTML);
        const placeholder = await candidates.nth(i).getAttribute('placeholder') || '';
        const ariaLabel = await candidates.nth(i).getAttribute('aria-label') || '';
        const name = await candidates.nth(i).getAttribute('name') || '';
        
        if (placeholder.includes('command') || 
            ariaLabel.includes('command') || 
            name.includes('command') ||
            html.includes('command')) {
          return candidates.nth(i);
        }
      }
      return null;
    };

    let commandInput = await findCommandInput();
    
    if (!commandInput) {
      console.log('Command input not found, searching for Parameters header to expand...');
      // Look for the "Parameters" text specifically
      const paramsHeader = page.locator('h3:has-text("Parameters")');
      if (await paramsHeader.isVisible()) {
        console.log('Found Parameters header, clicking parent button...');
        await paramsHeader.locator('..').first().click();
        await page.waitForTimeout(1000);
        commandInput = await findCommandInput();
      }
    }

    if (!commandInput) {
      console.log('Still not found, trying to click any button that looks like a section header...');
      const sectionButtons = page.locator('button[aria-label*="section"]');
      const btnCount = await sectionButtons.count();
      for (let i = 0; i < btnCount; i++) {
        const label = await sectionButtons.nth(i).getAttribute('aria-label');
        if (label && label.includes('Parameters')) {
          await sectionButtons.nth(i).click();
          await page.waitForTimeout(1000);
          commandInput = await findCommandInput();
          if (commandInput) break;
        }
      }
    }
    
    if (commandInput) {
      await commandInput.fill('date');
      await commandInput.press('Enter'); // Commit input
      console.log('Successfully filled command input');
    } else {
      // Last resort: find by label text
      console.log('Trying last resort: find by label text');
      const label = page.locator('text=command*').first();
      if (await label.isVisible()) {
        await page.keyboard.press('Tab'); // Usually input is after label
        await page.keyboard.type('date');
        await page.keyboard.press('Enter'); // Commit
      } else {
        throw new Error('Absolutely could not find command input field');
      }
    }

    console.log('Step 5b: Customizing Want Name...');
    const nameInput = page.getByRole('textbox', { name: 'Auto-generated or enter custom name' });
    await nameInput.fill('pw-test');
    // Name input is standard input, no Enter needed to commit but good for consistency

    console.log('Step 6: Setting up schedule (every 10 seconds)...');
    // Use evaluate to find and click the When header to be sure
    await page.evaluate(() => {
      const headers = Array.from(document.querySelectorAll('h3'));
      const whenHeader = headers.find(h => h.textContent.includes('When'));
      if (whenHeader) {
        whenHeader.closest('button').click();
      }
    });
    await page.waitForTimeout(1000);

    // Find and click Add When
    const addWhenBtn = page.locator('button').filter({ hasText: /Add When/i }).first();
    await addWhenBtn.scrollIntoViewIfNeeded();
    await addWhenBtn.click();
    await page.waitForTimeout(500);

    // Fill "Every" field
    console.log('Filling schedule details...');
    const everyInput = page.locator('input[type="number"], input[placeholder="30"]').first();
    await everyInput.fill('10');
    await everyInput.press('Enter'); // Commit

    // Save the schedule item
    await page.getByRole('button', { name: 'Save' }).first().click();
    await page.waitForTimeout(500);

    console.log('Step 7: Deploying the Want...');
    // Click the final Add button in the sidebar header
    const finalAddBtn = page.locator('button').filter({ hasText: /^Add$/ }).first();
    await finalAddBtn.click();

    console.log('Step 8: Verifying deployment on dashboard...');
    // Wait for the new want to appear in the dashboard list
    await page.waitForSelector('text=pw-test', { timeout: 10000 });

    console.log('✅ Successfully deployed Execution Result Want!');
    
    // Keep browser open for a few seconds to visually confirm
    await page.waitForTimeout(3000);
  } catch (error) {
    console.error('❌ Deployment failed:', error);
  } finally {
    await browser.close();
  }
}

deployExecutionResult();