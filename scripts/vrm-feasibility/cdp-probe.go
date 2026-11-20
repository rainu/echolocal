// An isolated capability probe in a disposable Chromium target.
// Run from the repository root: go run ./scripts/vrm-feasibility/cdp-probe.go
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	endpoint := flag.String("endpoint", "http://127.0.0.1:19334", "loopback CDP endpoint forwarded from the test device")
	flag.Parse()
	// Deliberately reject non-loopback URLs: CDP has no authentication.
	if *endpoint != "http://127.0.0.1:19334" {
		return fmt.Errorf("only the documented loopback endpoint is supported")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	r, err := client.Get(*endpoint + "/json/version")
	if err != nil {
		return err
	}
	defer r.Body.Close()
	var version struct {
		Browser string
		V8      string `json:"V8-Version"`
		WS      string `json:"webSocketDebuggerUrl"`
	}
	if err = json.NewDecoder(r.Body).Decode(&version); err != nil {
		return err
	}
	ws, _, err := websocket.DefaultDialer.Dial(version.WS, nil)
	if err != nil {
		return err
	}
	defer ws.Close()
	id := 0
	call := func(method, session string, params any) (json.RawMessage, error) {
		id++
		n := id
		ws.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if e := ws.WriteJSON(map[string]any{"id": n, "method": method, "sessionId": session, "params": params}); e != nil {
			return nil, e
		}
		ws.SetReadDeadline(time.Now().Add(15 * time.Second))
		for {
			var m struct {
				ID     int
				Result json.RawMessage
				Error  json.RawMessage
			}
			if e := ws.ReadJSON(&m); e != nil {
				return nil, e
			}
			if m.ID == n {
				if len(m.Error) > 0 {
					return nil, fmt.Errorf("%s: %s", method, m.Error)
				}
				return m.Result, nil
			}
		}
	}
	gpuRaw, err := call("SystemInfo.getInfo", "", map[string]any{})
	if err != nil {
		return err
	}
	var gpuInfo struct {
		GPU struct {
			Devices []struct {
				Vendor string `json:"vendorString"`
				Device string `json:"deviceString"`
			}
			FeatureStatus map[string]any `json:"featureStatus"`
		}
	}
	if err = json.Unmarshal(gpuRaw, &gpuInfo); err != nil {
		return err
	}
	raw, err := call("Target.createTarget", "", map[string]any{"url": "about:blank", "newWindow": true})
	if err != nil {
		return err
	}
	var target struct {
		ID string `json:"targetId"`
	}
	if err = json.Unmarshal(raw, &target); err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_, e := call("Target.closeTarget", "", map[string]any{"targetId": target.ID})
			if e != nil {
				fmt.Fprintln(os.Stderr, "cleanup failed; close the disposable target manually")
			}
		}
	}()
	raw, err = call("Target.attachToTarget", "", map[string]any{"targetId": target.ID, "flatten": true})
	if err != nil {
		return err
	}
	var session struct {
		ID string `json:"sessionId"`
	}
	if err = json.Unmarshal(raw, &session); err != nil {
		return err
	}
	raw, err = call("Runtime.evaluate", session.ID, map[string]any{"expression": probe, "returnByValue": true, "awaitPromise": true})
	if err != nil {
		return err
	}
	var result struct {
		Result    struct{ Value json.RawMessage }
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return err
	}
	if len(result.Exception) > 0 {
		return fmt.Errorf("probe exception: %s", result.Exception)
	}
	_, err = call("Target.closeTarget", "", map[string]any{"targetId": target.ID})
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}
	closed = true
	out := map[string]any{"browser": version.Browser, "v8": version.V8, "gpu": gpuInfo.GPU, "capabilities": json.RawMessage(result.Result.Value), "targetClosed": true}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

const probe = `(async () => {
 const result = {webgl: {}, audioContext: typeof AudioContext !== 'undefined', canvas2D: !!document.createElement('canvas').getContext('2d')};
 for (const kind of ['webgl2', 'webgl', 'experimental-webgl']) {
  const canvas = document.createElement('canvas');
  const errors = [];
  canvas.addEventListener('webglcontextcreationerror', e => errors.push(e.statusMessage));
  const gl = canvas.getContext(kind);
  result.webgl[kind] = gl ? {
   version: gl.getParameter(gl.VERSION), shading: gl.getParameter(gl.SHADING_LANGUAGE_VERSION),
   vendor: gl.getParameter(gl.VENDOR), renderer: gl.getParameter(gl.RENDERER),
   maxTextureSize: gl.getParameter(gl.MAX_TEXTURE_SIZE),
   unmaskedRenderer: (() => { const d=gl.getExtension('WEBGL_debug_renderer_info');return d?gl.getParameter(d.UNMASKED_RENDERER_WEBGL):null; })()
  } : {available:false,errors};
  if (gl) gl.getExtension('WEBGL_lose_context')?.loseContext();
 }
 let frames=0; const start=performance.now();let previous=start;let maxGap=0;
 await new Promise(resolve => { function tick(now) {frames++;maxGap=Math.max(maxGap,now-previous);previous=now;if(now-start>=3000)resolve();else requestAnimationFrame(tick);}requestAnimationFrame(tick); });
 result.emptyPageRAF={frames,elapsedMs:performance.now()-start,maxGapMs:maxGap};
 return result;
})()`
