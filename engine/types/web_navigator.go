package types

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// WebNavElement describes a single interactive element for navigation or form fill.
type WebNavElement struct {
	Role     string `json:"role"`
	Name     string `json:"name"`
	Selector string `json:"selector,omitempty"`
	FieldKey string `json:"field_key,omitempty"` // ASCII param key for textbox inputs
}

// OpenWebNavigator opens targetURL in the CDP browser (at cdpURL) and injects
// a keyboard/gamepad navigation overlay for the given elements.
// It returns immediately; the overlay runs asynchronously in the browser.
func OpenWebNavigator(ctx context.Context, cdpURL, targetURL string, elements []WebNavElement) error {
	serverPath := resolvePlaywrightServerPath()
	if serverPath == "" {
		return fmt.Errorf("[WEB-NAVIGATOR] playwright-app server not found; build mcp/playwright-app first")
	}

	mgr := GetNativeMCPManager(ctx)

	if err := ensurePlaywrightServer(ctx, serverPath, false); err != nil {
		return err
	}

	elemJSON, err := json.Marshal(elements)
	if err != nil {
		return fmt.Errorf("failed to marshal elements: %w", err)
	}

	toolArgs := map[string]any{
		"cdp_url":      cdpURL,
		"target_url":   targetURL,
		"nav_elements": string(elemJSON),
	}

	toolCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	result, err := mgr.ExecuteTool(toolCtx,
		"playwright-mcp-app",
		"node", []string{serverPath},
		"open_nav_tab",
		toolArgs)
	if isPipeClosed(err) {
		if restartErr := ensurePlaywrightServer(ctx, serverPath, true); restartErr != nil {
			return fmt.Errorf("failed to restart playwright-app server: %w", restartErr)
		}
		toolCtx2, cancel2 := context.WithTimeout(ctx, 30*time.Second)
		defer cancel2()
		result, err = mgr.ExecuteTool(toolCtx2,
			"playwright-mcp-app",
			"node", []string{serverPath},
			"open_nav_tab",
			toolArgs)
	}
	if err != nil {
		return fmt.Errorf("open_nav_tab failed: %w", err)
	}
	if result != nil && result.IsError {
		return fmt.Errorf("open_nav_tab error: %s", extractMCPErrorText(result))
	}
	return nil
}

// FillAndSubmitForm opens targetURL in the CDP browser, fills each textbox element
// from fieldValues (keyed by field_key), then clicks buttons or presses Enter to submit.
func FillAndSubmitForm(ctx context.Context, cdpURL, targetURL string, elements []WebNavElement, fieldValues map[string]string) error {
	serverPath := resolvePlaywrightServerPath()
	if serverPath == "" {
		return fmt.Errorf("[FILL-FORM] playwright-app server not found; build mcp/playwright-app first")
	}
	mgr := GetNativeMCPManager(ctx)
	if err := ensurePlaywrightServer(ctx, serverPath, false); err != nil {
		return err
	}

	elemJSON, err := json.Marshal(elements)
	if err != nil {
		return fmt.Errorf("failed to marshal elements: %w", err)
	}
	fvJSON, err := json.Marshal(fieldValues)
	if err != nil {
		return fmt.Errorf("failed to marshal field_values: %w", err)
	}

	toolArgs := map[string]any{
		"cdp_url":      cdpURL,
		"target_url":   targetURL,
		"elements":     string(elemJSON),
		"field_values": string(fvJSON),
	}

	toolCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := mgr.ExecuteTool(toolCtx,
		"playwright-mcp-app",
		"node", []string{serverPath},
		"fill_form",
		toolArgs)
	if isPipeClosed(err) {
		if restartErr := ensurePlaywrightServer(ctx, serverPath, true); restartErr != nil {
			return fmt.Errorf("failed to restart playwright-app server: %w", restartErr)
		}
		toolCtx2, cancel2 := context.WithTimeout(ctx, 60*time.Second)
		defer cancel2()
		result, err = mgr.ExecuteTool(toolCtx2,
			"playwright-mcp-app",
			"node", []string{serverPath},
			"fill_form",
			toolArgs)
	}
	if err != nil {
		return fmt.Errorf("fill_form failed: %w", err)
	}
	if result != nil && result.IsError {
		return fmt.Errorf("fill_form error: %s", extractMCPErrorText(result))
	}
	return nil
}

// BuildNavJS builds the self-contained navigation overlay JS for the given elements.
func BuildNavJS(elements []WebNavElement) string {
	elemJSON, _ := json.Marshal(elements)
	return fmt.Sprintf(`(function(){
  if(window.__mywantNavLoaded)return;
  window.__mywantNavLoaded=true;
  const ELEMENTS=%s;
  const s=document.createElement('style');
  s.textContent='.mwn-focused{outline:3px solid #f59e0b!important;box-shadow:0 0 0 4px rgba(245,158,11,.25)!important;}.mwn-panel{position:fixed;bottom:12px;right:12px;background:rgba(15,23,42,.95);color:#f1f5f9;font:12px/1.4 system-ui,sans-serif;padding:8px 12px;border-radius:8px;z-index:2147483647;border:1px solid #334155;}';
  document.head.appendChild(s);
  let idx=0;
  const find=sel=>document.querySelector(sel);
  const panel=document.createElement('div');panel.className='mwn-panel';document.body.appendChild(panel);
  const setFocus=i=>{
    ELEMENTS.forEach(e=>{const el=find(e.selector||e.name);if(el)el.classList.remove('mwn-focused');});
    const el=ELEMENTS[i]?find(ELEMENTS[i].selector||ELEMENTS[i].name):null;
    if(el){el.classList.add('mwn-focused');el.scrollIntoView({block:'nearest'});}
    panel.textContent='🤖 '+(ELEMENTS[i]?ELEMENTS[i].name:'?')+' ('+((i+1))+'/'+ELEMENTS.length+') ↑↓:移動';
  };
  document.addEventListener('keydown',e=>{
    if(e.key==='ArrowDown'||e.key==='ArrowRight'){e.preventDefault();idx=(idx+1)%%ELEMENTS.length;setFocus(idx);}
    else if(e.key==='ArrowUp'||e.key==='ArrowLeft'){e.preventDefault();idx=(idx-1+ELEMENTS.length)%%ELEMENTS.length;setFocus(idx);}
  },true);
  let prev=[];
  const gp=()=>{
    for(const g of navigator.getGamepads?.()??[]){if(!g)continue;
    const cur=g.buttons.map(b=>b.pressed);
    if((cur[13]&&!prev[13])||(cur[15]&&!prev[15])){idx=(idx+1)%%ELEMENTS.length;setFocus(idx);}
    if((cur[12]&&!prev[12])||(cur[14]&&!prev[14])){idx=(idx-1+ELEMENTS.length)%%ELEMENTS.length;setFocus(idx);}
    prev=cur;}requestAnimationFrame(gp);};
  requestAnimationFrame(gp);
  setFocus(0);
})();`, string(elemJSON))
}
