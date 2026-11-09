//go:build windows

// Package webview2 is a minimal WebView2 window wrapper with Go function
// bindings, adapted from go-webview2 (MIT License, Copyright 2020 John
// Chadwick, portions Copyright 2017 Serge Zaitsev). Trimmed and modified
// for cursor-tray: added CloseCallback and exposed the Chromium instance.
package webview2

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"sync"
	"unsafe"

	"cursor-tray/internal/webview2/edge"
	win32 "cursor-tray/internal/win32"

	"golang.org/x/sys/windows"
)

type Hint int

const (
	HintNone Hint = iota
	HintFixed
	HintMin
	HintMax
)

type Options struct {
	Title     string
	Width     uint
	Height    uint
	Center    bool
	DataPath  string // WebView2 user data folder; empty = exe-adjacent
	AutoFocus bool
	OnClose   func() // called when the user closes the window
}

type WebView struct {
	hwnd       uintptr
	mainthread uintptr
	chromium   *edge.Chromium
	autofocus  bool
	maxsz      win32.Point
	minsz      win32.Point
	m          sync.Mutex
	bindings   map[string]interface{}
	dispatchq  []func()
	onClose    func()
}

var (
	windowContext     = map[uintptr]*WebView{}
	windowContextSync sync.RWMutex
)

func getWindowContext(wnd uintptr) *WebView {
	windowContextSync.RLock()
	defer windowContextSync.RUnlock()
	return windowContext[wnd]
}

func setWindowContext(wnd uintptr, w *WebView) {
	windowContextSync.Lock()
	defer windowContextSync.Unlock()
	if w == nil {
		delete(windowContext, wnd)
		return
	}
	windowContext[wnd] = w
}

func (w *WebView) Chromium() *edge.Chromium { return w.chromium }

// New creates a WebView2-backed window. Must be called on the main thread.
func New(options Options) (*WebView, error) {
	if err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED); err != nil {
		// S_FALSE (already initialized) is fine; only fail on real errors.
		if errno, ok := err.(windows.Errno); !ok || errno != 1 {
			return nil, err
		}
	}

	w := &WebView{
		bindings:  map[string]interface{}{},
		autofocus: options.AutoFocus,
		onClose:   options.OnClose,
	}

	w.chromium = edge.NewChromium()
	w.chromium.DataPath = options.DataPath
	w.chromium.MessageCallback = w.msgcb

	w.mainthread, _, _ = win32.Kernel32GetCurrentThreadID.Call()

	if !w.createWindow(options) {
		return nil, errors.New("webview2: failed to create window")
	}

	settings, err := w.chromium.GetSettings()
	if err != nil {
		return nil, err
	}
	if err := settings.PutAreDefaultContextMenusEnabled(false); err != nil {
		return nil, err
	}
	if err := settings.PutAreDevToolsEnabled(false); err != nil {
		return nil, err
	}
	if err := settings.PutIsStatusBarEnabled(false); err != nil {
		return nil, err
	}
	w.chromium.SetPermission(edge.CoreWebView2PermissionKindClipboardRead, edge.CoreWebView2PermissionStateAllow)

	return w, nil
}

func (w *WebView) createWindow(opts Options) bool {
	var hinstance windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &hinstance)

	// Match upstream go-webview2: RegisterClassExW return/last-error must not
	// abort creation. Go syscalls often surface stale GetLastError values even
	// when the class registers successfully (or already exists).
	className, _ := windows.UTF16PtrFromString("cursorTrayWebView")
	wc := win32.WndClassExW{
		CbSize:        uint32(unsafe.Sizeof(win32.WndClassExW{})),
		HInstance:     hinstance,
		LpszClassName: className,
		LpfnWndProc:   windows.NewCallback(wndproc),
	}
	_, _, _ = win32.User32RegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	title, _ := windows.UTF16PtrFromString(opts.Title)
	width, height := opts.Width, opts.Height
	if width == 0 {
		width = 640
	}
	if height == 0 {
		height = 480
	}

	var posX, posY uintptr = uintptr(win32.CW_USEDEFAULT), uintptr(win32.CW_USEDEFAULT)
	if opts.Center {
		screenWidth, _, _ := win32.User32GetSystemMetrics.Call(win32.SM_CXSCREEN)
		screenHeight, _, _ := win32.User32GetSystemMetrics.Call(win32.SM_CYSCREEN)
		posX = (screenWidth - uintptr(width)) / 2
		posY = (screenHeight - uintptr(height)) / 2
	}

	w.hwnd, _, _ = win32.User32CreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		0xCF0000, // WS_OVERLAPPEDWINDOW
		posX, posY, uintptr(width), uintptr(height),
		0, 0, uintptr(hinstance), 0,
	)
	if w.hwnd == 0 {
		return false
	}
	setWindowContext(w.hwnd, w)

	_, _, _ = win32.User32ShowWindow.Call(w.hwnd, win32.SWShow)
	_, _, _ = win32.User32UpdateWindow.Call(w.hwnd)
	_, _, _ = win32.User32BringWindowToTop.Call(w.hwnd)
	_, _, _ = win32.User32SetForegroundWindow.Call(w.hwnd)
	_, _, _ = win32.User32SetFocus.Call(w.hwnd)

	if !w.chromium.Embed(w.hwnd) {
		return false
	}
	w.chromium.Resize()
	return true
}

func wndproc(hwnd, msg, wp, lp uintptr) uintptr {
	if w := getWindowContext(hwnd); w != nil {
		switch msg {
		case win32.WMMove, win32.WMMoving:
			_ = w.chromium.NotifyParentWindowPositionChanged()
		case win32.WMNCLButtonDown:
			_, _, _ = win32.User32SetFocus.Call(w.hwnd)
			r, _, _ := win32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		case win32.WMSize:
			w.chromium.Resize()
		case win32.WMActivate:
			if wp != win32.WAInactive && w.autofocus {
				w.chromium.Focus()
			}
		case win32.WMClose:
			if w.onClose != nil {
				w.onClose()
			}
			_, _, _ = win32.User32DestroyWindow.Call(hwnd)
		case win32.WMDestroy:
			setWindowContext(hwnd, nil)
			w.Terminate()
		case win32.WMGetMinMaxInfo:
			lpmmi := (*win32.MinMaxInfo)(unsafe.Pointer(lp))
			if w.maxsz.X > 0 && w.maxsz.Y > 0 {
				lpmmi.PtMaxSize = w.maxsz
				lpmmi.PtMaxTrackSize = w.maxsz
			}
			if w.minsz.X > 0 && w.minsz.Y > 0 {
				lpmmi.PtMinTrackSize = w.minsz
			}
		default:
			r, _, _ := win32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
			return r
		}
		return 0
	}
	r, _, _ := win32.User32DefWindowProcW.Call(hwnd, msg, wp, lp)
	return r
}

// Run blocks until Terminate is called, pumping window messages.
func (w *WebView) Run() {
	var msg win32.Msg
	for {
		r, _, _ := win32.User32GetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if r == 0 || msg.Message == win32.WMQuit {
			return
		}
		if msg.Message == win32.WMApp {
			w.m.Lock()
			q := append([]func(){}, w.dispatchq...)
			w.dispatchq = []func(){}
			w.m.Unlock()
			for _, v := range q {
				v()
			}
			continue
		}
		if r, _, _ := win32.User32GetAncestor.Call(uintptr(msg.Hwnd), win32.GARoot); r != 0 {
			if r, _, _ = win32.User32IsDialogMessage.Call(r, uintptr(unsafe.Pointer(&msg))); r != 0 {
				continue
			}
		}
		_, _, _ = win32.User32TranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = win32.User32DispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// RunOnce pumps pending messages without blocking (for embedded loops).
func (w *WebView) Terminate() {
	_, _, _ = win32.User32PostQuitMessage.Call(0)
}

func (w *WebView) Destroy() {
	_, _, _ = win32.User32PostMessageW.Call(w.hwnd, win32.WMClose, 0, 0)
}

func (w *WebView) Window() uintptr { return w.hwnd }

func (w *WebView) Navigate(url string)   { w.chromium.Navigate(url) }
func (w *WebView) SetHTML(html string)   { w.chromium.NavigateToString(html) }
func (w *WebView) Init(js string)        { w.chromium.Init(js) }
func (w *WebView) Eval(js string)        { w.chromium.Eval(js) }

func (w *WebView) SetTitle(title string) {
	t, _ := windows.UTF16FromString(title)
	_, _, _ = win32.User32SetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(&t[0])))
}

func (w *WebView) SetSize(width, height int, hints Hint) {
	index := win32.GWLStyle
	style := win32.GetWindowLong(w.hwnd, index)
	if hints == HintFixed {
		style &^= (win32.WSThickFrame | win32.WSMaximizeBox)
	} else {
		style |= (win32.WSThickFrame | win32.WSMaximizeBox)
	}
	win32.SetWindowLong(w.hwnd, index, style)

	if hints == HintMax {
		w.maxsz.X = int32(width)
		w.maxsz.Y = int32(height)
	} else if hints == HintMin {
		w.minsz.X = int32(width)
		w.minsz.Y = int32(height)
	} else {
		r := win32.Rect{Right: int32(width), Bottom: int32(height)}
		_, _, _ = win32.User32AdjustWindowRect.Call(uintptr(unsafe.Pointer(&r)), win32.WSOverlappedWindow, 0)
		_, _, _ = win32.User32SetWindowPos.Call(
			w.hwnd, 0, uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top),
			win32.SWPNoZOrder|win32.SWPNoActivate|win32.SWPNoMove|win32.SWPFrameChanged)
		w.chromium.Resize()
	}
}

func (w *WebView) Dispatch(f func()) {
	w.m.Lock()
	w.dispatchq = append(w.dispatchq, f)
	w.m.Unlock()
	_, _, _ = win32.User32PostThreadMessageW.Call(w.mainthread, win32.WMApp, 0, 0)
}

// ---------------------------------------------------------------------------
// Go <-> JS bindings (RPC over window.external.invoke)
// ---------------------------------------------------------------------------

type rpcMessage struct {
	ID     int               `json:"id"`
	Method string            `json:"method"`
	Params []json.RawMessage `json:"params"`
}

func jsString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (w *WebView) msgcb(msg string) {
	d := rpcMessage{}
	if err := json.Unmarshal([]byte(msg), &d); err != nil {
		return
	}
	id := strconv.Itoa(d.ID)
	if res, err := w.callbinding(d); err != nil {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].reject(" + jsString(err.Error()) + "); window._rpc[" + id + "] = undefined")
		})
	} else if b, err := json.Marshal(res); err != nil {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].reject(" + jsString(err.Error()) + "); window._rpc[" + id + "] = undefined")
		})
	} else {
		w.Dispatch(func() {
			w.Eval("window._rpc[" + id + "].resolve(" + string(b) + "); window._rpc[" + id + "] = undefined")
		})
	}
}

func (w *WebView) callbinding(d rpcMessage) (interface{}, error) {
	w.m.Lock()
	f, ok := w.bindings[d.Method]
	w.m.Unlock()
	if !ok {
		return nil, nil
	}

	v := reflect.ValueOf(f)
	if v.Kind() != reflect.Func {
		return nil, errors.New("binding is not a function")
	}
	if n := v.Type().NumOut(); n > 2 {
		return nil, errors.New("function may only return a value or a value+error")
	}

	isVariadic := v.Type().IsVariadic()
	numIn := v.Type().NumIn()
	if (isVariadic && len(d.Params) < numIn-1) || (!isVariadic && len(d.Params) != numIn) {
		return nil, errors.New("function arguments mismatch")
	}

	args := []reflect.Value{}
	for i := range d.Params {
		var arg reflect.Value
		if isVariadic && i >= numIn-1 {
			arg = reflect.New(v.Type().In(numIn - 1).Elem())
		} else {
			arg = reflect.New(v.Type().In(i))
		}
		if err := json.Unmarshal(d.Params[i], arg.Interface()); err != nil {
			return nil, err
		}
		args = append(args, arg.Elem())
	}

	errorType := reflect.TypeOf((*error)(nil)).Elem()
	res := v.Call(args)
	switch len(res) {
	case 0:
		return nil, nil
	case 1:
		if res[0].Type().Implements(errorType) {
			if res[0].Interface() != nil {
				return nil, res[0].Interface().(error)
			}
			return nil, nil
		}
		return res[0].Interface(), nil
	case 2:
		if !res[1].Type().Implements(errorType) {
			return nil, errors.New("second return value must be an error")
		}
		if res[1].Interface() == nil {
			return res[0].Interface(), nil
		}
		return nil, res[1].Interface().(error)
	default:
		return nil, errors.New("unexpected number of return values")
	}
}

func (w *WebView) Bind(name string, f interface{}) error {
	v := reflect.ValueOf(f)
	if v.Kind() != reflect.Func {
		return errors.New("only functions can be bound")
	}
	if n := v.Type().NumOut(); n > 2 {
		return errors.New("function may only return a value or a value+error")
	}
	w.m.Lock()
	w.bindings[name] = f
	w.m.Unlock()

	w.Init("(function() { var name = " + jsString(name) + ";" + `
		var RPC = window._rpc = (window._rpc || {nextSeq: 1});
		window[name] = function() {
		  var seq = RPC.nextSeq++;
		  var promise = new Promise(function(resolve, reject) {
			RPC[seq] = {
			  resolve: resolve,
			  reject: reject,
			};
		  });
		  window.external.invoke(JSON.stringify({
			id: seq,
			method: name,
			params: Array.prototype.slice.call(arguments),
		  }));
		  return promise;
		}
	})()`)

	return nil
}
