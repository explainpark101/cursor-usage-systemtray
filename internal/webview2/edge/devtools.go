//go:build windows

package edge

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// Additional bindings for DevTools protocol cookie extraction.
// Adapted from go-webview2 (MIT, John Chadwick).

type _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl struct {
	_IUnknownVtbl
	Invoke ComProc
}

type ICoreWebView2CallDevToolsProtocolMethodCompletedHandler struct {
	vtbl *_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl
	impl _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl
}

type _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl interface {
	_IUnknownImpl
	DevToolsProtocolMethodCompleted(errorCode uintptr, returnObjectAsJson *uint16) uintptr
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownQueryInterface(this *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler, refiid, object uintptr) uintptr {
	return this.impl.QueryInterface(refiid, object)
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownAddRef(this *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler) uintptr {
	return this.impl.AddRef()
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownRelease(this *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler) uintptr {
	return this.impl.Release()
}

func _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerInvoke(this *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler, errorCode uintptr, returnObjectAsJson *uint16) uintptr {
	return this.impl.DevToolsProtocolMethodCompleted(errorCode, returnObjectAsJson)
}

var _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerFn = _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerVtbl{
	_IUnknownVtbl{
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownQueryInterface),
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownAddRef),
		NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerIUnknownRelease),
	},
	NewComProc(_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerInvoke),
}

func newICoreWebView2CallDevToolsProtocolMethodCompletedHandler(impl _ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerImpl) *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler {
	return &ICoreWebView2CallDevToolsProtocolMethodCompletedHandler{
		vtbl: &_ICoreWebView2CallDevToolsProtocolMethodCompletedHandlerFn,
		impl: impl,
	}
}

type devToolsMethodCompletedHandlerImpl struct {
	ref  *ICoreWebView2CallDevToolsProtocolMethodCompletedHandler
	callback func(errorCode uintptr, returnObjectAsJSON string)
}

func (i *devToolsMethodCompletedHandlerImpl) QueryInterface(refiid, object uintptr) uintptr {
	// Only IUnknown is required by WebView2 for handlers.
	*(*uintptr)(unsafe.Pointer(object)) = uintptr(unsafe.Pointer(i.ref))
	return 0
}

func (i *devToolsMethodCompletedHandlerImpl) AddRef() uintptr { return 1 }
func (i *devToolsMethodCompletedHandlerImpl) Release() uintptr { return 1 }

func (i *devToolsMethodCompletedHandlerImpl) DevToolsProtocolMethodCompleted(errorCode uintptr, returnObjectAsJSON *uint16) uintptr {
	json := ""
	if returnObjectAsJSON != nil {
		json = windows.UTF16PtrToString(returnObjectAsJSON)
	}
	if i.callback != nil {
		i.callback(errorCode, json)
	}
	return 0
}

// GetCookies collects cookies via the DevTools protocol and returns them as a
// JSON string shaped like {"cookies":[{name,value,domain,...}]}.
func (e *Chromium) GetAllCookiesDevTools(callback func(json string, err error)) {
	handlerImpl := &devToolsMethodCompletedHandlerImpl{}
	handlerImpl.ref = newICoreWebView2CallDevToolsProtocolMethodCompletedHandler(handlerImpl)
	handlerImpl.callback = func(errorCode uintptr, json string) {
		if errorCode != 0 {
			callback("", windows.Errno(errorCode))
			return
		}
		callback(json, nil)
	}

	methodNamePtr, _ := windows.UTF16PtrFromString("Network.getAllCookies")
	paramsPtr, _ := windows.UTF16PtrFromString("{}")

	r, _, err := e.webview.vtbl.CallDevToolsProtocolMethod.Call(
		uintptr(unsafe.Pointer(e.webview)),
		uintptr(unsafe.Pointer(methodNamePtr)),
		uintptr(unsafe.Pointer(paramsPtr)),
		uintptr(unsafe.Pointer(handlerImpl.ref)),
	)
	if r != 0 {
		callback("", err)
	}
}

// WebView returns the underlying ICoreWebView2 for advanced use.
func (e *Chromium) WebView() *ICoreWebView2 { return e.webview }
