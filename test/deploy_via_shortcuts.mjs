import { chromium } from 'playwright';

async function deployViaShortcuts() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext();
  const page = await context.newPage();

  try {
    console.log('1. Navigating to Dashboard...');
    await page.goto('http://localhost:3000/dashboard');
    await page.waitForLoadState('networkidle');
    await page.waitForTimeout(2000);

    console.log('2. Resetting state with Escape...');
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
    await page.click('body');

    console.log('3. Opening sidebar with "a"...');
    await page.keyboard.press('a');
    await page.waitForSelector('h2:has-text("New Want")', { timeout: 10000 });

    console.log('4. Focusing search with "/"...');
    await page.keyboard.press('/');
    await page.waitForTimeout(500);
    
    console.log('5. Searching for "execution"...');
    // Clear and type
    await page.evaluate(() => {
      const sidebar = document.querySelector('[data-sidebar="true"]');
      const input = sidebar.querySelector('input[placeholder*="keyword"]');
      if (input) {
        input.value = '';
        input.dispatchEvent(new Event('input', { bubbles: true }));
      }
    });
    await page.keyboard.type('execution', { delay: 50 });
    await page.waitForTimeout(1000);

    console.log('6. Selecting "Command Execution Result"...');
    // We'll click it directly to be sure, as ArrowDown depends on result order
    const typeBtnSelector = 'button:has-text("Command Execution Result")';
    await page.waitForSelector(typeBtnSelector);
    await page.click(typeBtnSelector);
    await page.waitForTimeout(2000);

    console.log('7. Navigating to Parameters section...');
    // Focus Name input first
    const nameInputSelector = 'input[placeholder*="custom name"]';
    await page.waitForSelector(nameInputSelector);
    await page.focus(nameInputSelector);
    
    // Move to Parameters Header using dynamic navigation (ArrowDown from Name)
    await page.keyboard.press('ArrowDown');
    await page.waitForTimeout(500);

    console.log('8. Expanding and typing command...');
    // Currently on Parameters Header, press ArrowRight to expand and focus input
    await page.keyboard.press('ArrowRight');
    await page.waitForTimeout(1000); // Wait for expansion and focus move
    
    const testId = Math.floor(Math.random() * 10000);
    const cmd = `echo "sc-final-${testId}"`;
    
    // Type into active element (the command input)
    await page.keyboard.type(cmd, { delay: 30 });
    await page.waitForTimeout(500);

    console.log('9. Submitting (double Enter for CommitInput)...');
    await page.keyboard.press('Enter'); // First Enter: commit input
    await page.waitForTimeout(500);
    await page.keyboard.press('Enter'); // Second Enter: submit form
    await page.waitForTimeout(2000); // Wait for sidebar to close
    
    console.log('10. Verifying deployment...');
    const expected = `sc-final-${testId}`;
    let confirmed = false;
    for (let i = 0; i < 20; i++) {
      const content = await page.evaluate(() => document.body.innerText);
      if (content.includes(expected)) {
        confirmed = true;
        break;
      }
      process.stdout.write('.');
      await page.waitForTimeout(1000);
    }
    console.log('');

    if (confirmed) {
      console.log(`✅ Successfully deployed! Found: ${expected}`);
    } else {
      throw new Error('Deployment confirmation failed - text not found on dashboard');
    }

  } catch (error) {
    console.error('❌ Error:', error.message);
    process.exit(1);
  } finally {
    await browser.close();
  }
}

deployViaShortcuts();