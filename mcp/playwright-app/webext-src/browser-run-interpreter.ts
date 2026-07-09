// Runs a @puppeteer/replay UserFlow's steps against the CURRENT page via
// plain DOM operations — no Puppeteer, no CDP, no eval. This is the
// extension-side counterpart to POST /api/v1/web-wants/browser-run
// (engine/server/handlers_web_wants.go): the backend queues a claim
// (url + steps), the extension's handleBrowserRun (background.js) opens a
// tab and injects the bundle this file compiles into, and the returned
// result gets POSTed back to /browser-run-result.
//
// @puppeteer/replay (dependencies: {}) supplies the Step/UserFlow schema and
// the Runner class that walks flow.steps calling extension.runStep(step) —
// exactly the same schema Chrome DevTools' built-in Recorder panel exports,
// so a recorded flow can be used here with zero translation. We only ever
// call createRunner(flow, extension) with OUR OWN extension instance, so
// @puppeteer/replay's own PuppeteerRunnerExtension fallback (which dynamic-
// imports the real `puppeteer` package) is never reached — see
// createPuppeteerRunnerOwningBrowserExtension in its source, only invoked
// when no extension is supplied.
//
// read/readAll/loop/sleep/reactChange/if/setResult/forEachClick aren't part
// of the upstream schema (it's a replay format for recorded UI actions, not
// a scraping/looping DSL) — added via the schema's own official
// extensibility point, CustomStep{type:'customStep', name, parameters}.
import { RunnerExtension, createRunner, StepType } from '@puppeteer/replay';
import type { Step, UserFlow, ClickStep, ChangeStep, WaitForElementStep, CustomStep, Selector } from '@puppeteer/replay';

const DEFAULT_TIMEOUT_MS = 5000;

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

// CSS selectors resolve via plain querySelector. A leading "xpath:" prefix
// evaluates via document.evaluate instead — needed for the smartgolf-book
// plugin's "click the button whose exact text is 'Next'" and "select the
// <li> containing a label with this room name" cases, neither expressible
// as CSS. The delimiter is ":" rather than the more common-looking "/"
// specifically because a real xpath expression itself almost always starts
// with "/" or "//" (an absolute path) — "xpath/" + "//li" collapses to
// "xpath///li", and naively slicing a fixed-length "xpath/" prefix off
// THAT leaves "//li" only by accident of matching slash counts; get the
// count wrong (e.g. write "xpath//li" expecting the obvious result) and the
// slice instead leaves "/li" — a single-slash absolute-child path that can
// never match a real element. ":" can't collide with anything XPath itself
// produces, so this whole class of off-by-one-slash bug isn't reachable.
// XPath's `//...` searches from the document root regardless of the
// supplied context node, so this only composes correctly as the first hop
// of a selector chain (fine — every current caller uses it that way, never
// nested inside a shadow-DOM chain). ARIA/Text/Pierce candidates are still
// attempted as plain CSS and simply won't match, falling through to the
// next candidate group like a normal miss.
function resolveSingle(scope: Document | Element, sel: string): Element | null {
  if (sel.startsWith('xpath:')) {
    const expr = sel.slice('xpath:'.length);
    try {
      const doc = scope.ownerDocument || (scope as Document);
      const result = doc.evaluate(expr, scope, null, XPathResult.FIRST_ORDERED_NODE_TYPE, null);
      return (result.singleNodeValue as Element) || null;
    } catch {
      return null;
    }
  }
  try {
    return scope.querySelector(sel);
  } catch {
    return null;
  }
}

function resolveOnce(selectors: Selector[]): Element | null {
  for (const group of selectors) {
    const chain = Array.isArray(group) ? group : [group];
    let scope: Document | Element = document;
    let el: Element | null = null;
    for (const sel of chain) {
      el = resolveSingle(scope, sel);
      if (!el) break;
      scope = (el.shadowRoot as unknown as Element) || el;
    }
    if (el) return el;
  }
  return null;
}

async function waitForSelectors(selectors: Selector[], timeoutMs: number): Promise<Element | null> {
  const existing = resolveOnce(selectors);
  if (existing) return existing;
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    await sleep(100);
    const el = resolveOnce(selectors);
    if (el) return el;
  }
  return null;
}

// Sets .value through the native setter so frameworks that wrap onChange
// (React etc.) still see the update — a direct el.value= assignment is
// invisible to their synthetic event system. Same trick already proven in
// content.js's mywantFillAndSubmit for the want-card auto-fill path.
function setNativeValue(el: HTMLInputElement | HTMLTextAreaElement, value: string): void {
  const proto = el.tagName === 'TEXTAREA' ? window.HTMLTextAreaElement.prototype : window.HTMLInputElement.prototype;
  const setter = Object.getOwnPropertyDescriptor(proto, 'value')?.set;
  if (setter) setter.call(el, value); else (el as any).value = value;
  el.dispatchEvent(new Event('input', { bubbles: true }));
  el.dispatchEvent(new Event('change', { bubbles: true }));
}

type ExtractMode = 'text' | 'value' | 'html' | 'attr' | 'computedStyle';

interface ReadParams {
  selector: string;
  as?: string;
  extract?: ExtractMode;
  attr?: string;
  style_prop?: string;
  timeout_ms?: number;
}
interface LoopUntil {
  selector_exists?: string;
  selector_gone?: string;
}
interface LoopParams {
  max_iterations?: number;
  until?: LoopUntil;
  body?: Step[];
}

// FieldSpec resolves a value relative to some element el: el itself when
// selector is omitted, el.querySelector(selector) for a plain CSS selector,
// or an xpath:-prefixed expression (see resolveSingle) evaluated with el as
// the context node — covers ancestor/descendant traversal CSS can't express
// (e.g. "xpath:ancestor::label[1]//*[local-name()='svg']" to reach a row's
// related icon — see resolveSingle's doc comment for why local-name() is
// needed for svg specifically).
interface FieldSpec {
  selector?: string;
  extract?: ExtractMode;
  attr?: string;
  style_prop?: string;
}

// FilterSpec resolves like FieldSpec, then compares the extracted value —
// used by readAll/forEachClick to skip elements not matching a computed
// condition (e.g. an svg's computed fill color denoting availability).
interface FilterSpec extends FieldSpec {
  exists?: boolean;
  equals?: string;
  not_equals?: string;
  contains?: string;
}

interface ReadAllParams extends ReadParams {
  fields?: Record<string, FieldSpec>;
  filter?: FilterSpec;
}

interface ForEachClickParams {
  selector: string;
  as: string;
  trigger_field?: FieldSpec;
  trigger_key?: string;
  wait_after_click?: { selector: string; timeout_ms?: number };
  read: ReadAllParams;
}

// IfCondition covers the two shapes control flow in the surveyed plugins
// actually needs: plain existence ("has this loop found its target yet?")
// and a resolved-value comparison ("is this specific element disabled?").
interface IfCondition extends FilterSpec {
  selector_exists?: string;
  selector_gone?: string;
}
interface IfParams {
  condition?: IfCondition;
  then?: Step[];
  else?: Step[];
}

function extractFrom(el: Element, mode: ExtractMode | undefined, attr?: string, styleProp?: string): unknown {
  switch (mode) {
    case 'value':
      return (el as HTMLInputElement).value;
    case 'html':
      return el.outerHTML;
    case 'attr':
      return attr ? el.getAttribute(attr) : null;
    case 'computedStyle':
      return styleProp ? getComputedStyle(el).getPropertyValue(styleProp) : null;
    case 'text':
    default: {
      // innerText requires an up-to-date layout/style pass and returns ''
      // for elements the browser hasn't rendered — happens for off-screen
      // virtualized list rows (confirmed with Gmail's inbox list) and can
      // happen more generally for a background (non-active) tab, which
      // browser_run opens by default (see background.js's handleBrowserRun
      // background param). textContent doesn't depend on layout at all, so
      // it's used as a fallback — but not the default, since it lacks
      // innerText's synthetic newline-per-block-boundary behavior that
      // several plugins rely on for line-based text scraping (e.g.
      // smartgolf-check-reserved's "Approved\n<store>\n<room>\n<date>"
      // parsing only works because innerText inserts those line breaks).
      const text = (el as HTMLElement).innerText;
      return text || el.textContent || '';
    }
  }
}

// Resolves a FieldSpec relative to el — see the FieldSpec doc comment above.
function resolveFieldEl(el: Element, spec?: FieldSpec): Element | null {
  if (!spec || !spec.selector) return el;
  return resolveSingle(el, spec.selector);
}

// exists means two different things depending on whether extract is also
// set: alone, it's "does the resolved element exist" (e.g. a plain presence
// check); paired with extract (e.g. extract:'attr', attr:'disabled'), it's
// "does the resolved *value* exist" — needed for boolean-attribute checks
// where the attribute's string value is unpredictable ("", "true",
// "disabled", ...) and only its presence/absence is meaningful.
function passesFilter(el: Element, filter?: FilterSpec): boolean {
  if (!filter) return true;
  const target = resolveFieldEl(el, filter);
  if (filter.exists === true && filter.extract === undefined) return target !== null;
  if (filter.exists === false && filter.extract === undefined) return target === null;
  if (!target) return false;
  const value = extractFrom(target, filter.extract, filter.attr, filter.style_prop);
  if (filter.exists === true) return value !== null && value !== undefined;
  if (filter.exists === false) return value === null || value === undefined;
  if (filter.equals !== undefined) return String(value) === filter.equals;
  if (filter.not_equals !== undefined) return String(value) !== filter.not_equals;
  if (filter.contains !== undefined) return String(value ?? '').includes(filter.contains);
  return true;
}

// Evaluates an 'if' step's condition against the whole document (unlike
// FilterSpec/FieldSpec above, which resolve relative to one already-matched
// element) — selector_exists/selector_gone for plain presence checks,
// selector+extract+equals/not_equals/contains/exists for a specific
// element's resolved value (e.g. smartgolf-book's disabled-attribute check
// on a specific time-slot input).
function evaluateCondition(cond?: IfCondition): boolean {
  if (!cond) return false;
  if (cond.selector_exists !== undefined) {
    return resolveSingle(document, cond.selector_exists) !== null;
  }
  if (cond.selector_gone !== undefined) {
    return resolveSingle(document, cond.selector_gone) === null;
  }
  if (cond.selector !== undefined) {
    const target = resolveSingle(document, cond.selector);
    // See passesFilter's doc comment — exists means "element exists" alone,
    // or "extracted value exists" when paired with extract.
    if (cond.exists === true && cond.extract === undefined) return target !== null;
    if (cond.exists === false && cond.extract === undefined) return target === null;
    if (!target) return false;
    const value = extractFrom(target, cond.extract, cond.attr, cond.style_prop);
    if (cond.exists === true) return value !== null && value !== undefined;
    if (cond.exists === false) return value === null || value === undefined;
    if (cond.equals !== undefined) return String(value) === cond.equals;
    if (cond.not_equals !== undefined) return String(value) !== cond.not_equals;
    if (cond.contains !== undefined) return String(value ?? '').includes(cond.contains);
    return target !== null;
  }
  return false;
}

// Mirrors Playwright's `page` surface just enough for our two "write" step
// types (click/change) plus the DOM-level wait every step type needs — NOT
// a Playwright reimplementation, just named the same way since every
// surveyed plugin (gmail/smartgolf) already thinks in these terms.
export class ExtensionRunnerExtension extends RunnerExtension {
  result: Record<string, unknown> = {};

  async runStep(step: Step, _flow?: UserFlow): Promise<void> {
    switch (step.type) {
      case StepType.Click: {
        const s = step as ClickStep;
        const el = await waitForSelectors(s.selectors, s.timeout ?? DEFAULT_TIMEOUT_MS);
        (el as HTMLElement | null)?.click();
        return;
      }
      case StepType.Change: {
        const s = step as ChangeStep;
        const el = await waitForSelectors(s.selectors, s.timeout ?? DEFAULT_TIMEOUT_MS);
        if (el) {
          (el as HTMLElement).focus();
          setNativeValue(el as HTMLInputElement, s.value);
        }
        return;
      }
      case StepType.WaitForElement: {
        const s = step as WaitForElementStep;
        // We only support the common case (count==1, operator=='=='/undefined
        // i.e. "wait until present") — surveyed plugins never needed the
        // absence/count-range variants.
        await waitForSelectors(s.selectors, s.timeout ?? DEFAULT_TIMEOUT_MS);
        return;
      }
      case StepType.CustomStep: {
        await this.runCustomStep(step as CustomStep);
        return;
      }
      default:
        // Unsupported step types (navigate/scroll/hover/keyDown/... ) are
        // silently skipped rather than throwing — none of the plugins this
        // was built for need them, and failing the whole flow over an
        // optional step (e.g. a recorded mouse hover) would be worse than
        // just not performing it.
        return;
    }
  }

  private async runCustomStep(step: CustomStep): Promise<void> {
    const params = (step.parameters ?? {}) as ReadAllParams & LoopParams & { name?: string };
    switch (step.name) {
      case 'read': {
        const el = await waitForSelectors([params.selector], params.timeout_ms ?? DEFAULT_TIMEOUT_MS);
        const key = params.as || params.selector;
        this.result[key] = el ? extractFrom(el, params.extract, params.attr, params.style_prop) : null;
        return;
      }
      case 'readAll': {
        const els = Array.from(document.querySelectorAll(params.selector)).filter((el) => passesFilter(el, params.filter));
        const key = params.as || params.selector;
        if (params.fields) {
          const fields = params.fields;
          this.result[key] = els.map((el) => {
            const record: Record<string, unknown> = {};
            for (const [fieldName, spec] of Object.entries(fields)) {
              const target = resolveFieldEl(el, spec);
              record[fieldName] = target ? extractFrom(target, spec.extract, spec.attr, spec.style_prop) : null;
            }
            return record;
          });
        } else {
          this.result[key] = els.map((el) => extractFrom(el, params.extract, params.attr, params.style_prop));
        }
        return;
      }
      case 'loop': {
        const max = params.max_iterations || 1;
        const body = params.body || [];
        for (let i = 0; i < max; i++) {
          if (params.until?.selector_exists && resolveSingle(document, params.until.selector_exists)) break;
          if (params.until?.selector_gone && !resolveSingle(document, params.until.selector_gone)) break;
          for (const nested of body) {
            await this.runStep(nested);
          }
        }
        return;
      }
      case 'sleep': {
        const p = (step.parameters ?? {}) as { ms?: number };
        await sleep(p.ms ?? 500);
        return;
      }
      case 'setResult': {
        const p = (step.parameters ?? {}) as { key?: string; value?: unknown };
        if (p.key) this.result[p.key] = p.value;
        return;
      }
      case 'reactChange': {
        // Dispatches a controlled input's React onChange handler directly
        // via its internal fiber props, bypassing native DOM events —
        // needed for React apps (e.g. smartgolf.stores.jp's XState-driven
        // booking flow) that only react to the synthetic event, not a real
        // 'change'/'input' DOM event (setNativeValue's approach, used by
        // the Change step above, doesn't trigger these).
        const p = (step.parameters ?? {}) as { selector: string; timeout_ms?: number; debug_as?: string };
        const el = await waitForSelectors([p.selector], p.timeout_ms ?? DEFAULT_TIMEOUT_MS);
        if (el) {
          const anyEl = el as unknown as Record<string, any>;
          const allKeys = Object.keys(anyEl);
          const propsKey = allKeys.find((k) => k.startsWith('__reactProps'));
          const ok = !!(propsKey && anyEl[propsKey].onChange);
          if (ok) {
            anyEl[propsKey!].onChange({ target: el, currentTarget: el });
          }
          if (p.debug_as) {
            this.result[p.debug_as] = { allKeys, propsKey: propsKey ?? null, hadOnChange: ok };
          }
        }
        return;
      }
      case 'if': {
        const p = (step.parameters ?? {}) as IfParams;
        const branch = evaluateCondition(p.condition) ? p.then || [] : p.else || [];
        for (const nested of branch) {
          await this.runStep(nested);
        }
        return;
      }
      case 'forEachClick': {
        const p = (step.parameters ?? {}) as unknown as ForEachClickParams;
        const triggers = Array.from(document.querySelectorAll(p.selector));
        const triggerKey = p.trigger_key || '_trigger';
        const collected: Record<string, unknown>[] = [];
        for (const trigger of triggers) {
          (trigger as HTMLElement).click();
          if (p.wait_after_click?.selector) {
            await waitForSelectors([p.wait_after_click.selector], p.wait_after_click.timeout_ms ?? DEFAULT_TIMEOUT_MS);
          }
          const triggerValue = p.trigger_field
            ? (() => {
                const target = resolveFieldEl(trigger, p.trigger_field);
                return target ? extractFrom(target, p.trigger_field!.extract, p.trigger_field!.attr, p.trigger_field!.style_prop) : null;
              })()
            : null;
          const rp = p.read;
          const rows = Array.from(document.querySelectorAll(rp.selector)).filter((el) => passesFilter(el, rp.filter));
          for (const row of rows) {
            const record: Record<string, unknown> = {};
            if (p.trigger_field) record[triggerKey] = triggerValue;
            if (rp.fields) {
              for (const [fieldName, spec] of Object.entries(rp.fields)) {
                const target = resolveFieldEl(row, spec);
                record[fieldName] = target ? extractFrom(target, spec.extract, spec.attr, spec.style_prop) : null;
              }
            } else {
              record.value = extractFrom(row, rp.extract, rp.attr, rp.style_prop);
            }
            collected.push(record);
          }
        }
        this.result[p.as] = collected;
        return;
      }
      default:
        return;
    }
  }
}

// Entry point injected via chrome.scripting.executeScript's func+args (see
// handleBrowserRun in background.js) — steps is already-parsed JSON
// (the Step[] from browserRunClaim), passed straight through from Go's
// json.RawMessage with no re-encoding.
export async function runBrowserSteps(steps: Step[]): Promise<Record<string, unknown>> {
  const extension = new ExtensionRunnerExtension();
  const flow: UserFlow = { title: 'mywant-browser-run', steps };
  const runner = await createRunner(flow, extension);
  await runner.run();
  return extension.result;
}
