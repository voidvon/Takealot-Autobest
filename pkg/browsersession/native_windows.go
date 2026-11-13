//go:build windows

package browsersession

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/wailsapp/go-webview2/pkg/edge"
	wv "github.com/wailsapp/go-webview2/pkg/webview2"
	"github.com/wailsapp/go-webview2/webviewloader"
	"golang.org/x/sys/windows"
)

var user32 = windows.NewLazySystemDLL("user32.dll")
var ole32 = windows.NewLazySystemDLL("ole32.dll")
var getMessage = user32.NewProc("GetMessageW")
var translateMessage = user32.NewProc("TranslateMessage")
var dispatchMessage = user32.NewProc("DispatchMessageW")
var postThreadMessage = user32.NewProc("PostThreadMessageW")
var createWindow = user32.NewProc("CreateWindowExW")
var destroyWindow = user32.NewProc("DestroyWindow")
var showWindow = user32.NewProc("ShowWindow")
var moveWindow = user32.NewProc("MoveWindow")
var getClientRect = user32.NewProc("GetClientRect")

const sessionMessage = 0x8000 + 339

type winMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	X, Y           int32
	Private        uint32
}
type winDriver struct {
	mu      sync.Mutex
	thread  uint32
	parent  uintptr
	jobs    chan func()
	done    chan struct{}
	started bool
	views   map[string]*winView // confined to the COM STA
}
type winView struct {
	id, origin  string
	hwnd        uintptr
	controller  *edge.ICoreWebView2Controller
	core        *edge.ICoreWebView2
	environment unsafe.Pointer
	closed      bool
}
type evalReply struct {
	value string
	err   error
}

func NewNativeDriver() Driver {
	return &winDriver{jobs: make(chan func(), 128), done: make(chan struct{}), views: map[string]*winView{}}
}

func (d *winDriver) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return nil
	}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(d.done)
		hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 2) // COINIT_APARTMENTTHREADED
		if int32(hr) < 0 {
			ready <- fmt.Errorf("COM 初始化失败: 0x%x", hr)
			return
		}
		defer ole32.NewProc("CoUninitialize").Call()
		parent := findMainWindow()
		if parent == 0 {
			ready <- errors.New("未找到主程序窗口")
			return
		}
		d.parent = parent
		d.thread = windows.GetCurrentThreadId()
		var msg winMessage
		user32.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
		ready <- nil
		for {
			result, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(result) <= 0 {
				break
			}
			if msg.Message == sessionMessage {
				select {
				case job := <-d.jobs:
					job()
				default:
				}
				continue
			}
			translateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			dispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
		}
		for _, view := range d.views {
			closeWinView(view)
		}
	}()
	err := <-ready
	if err == nil {
		d.started = true
	}
	return err
}

func findMainWindow() uintptr {
	var found uintptr
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var pid uint32
		user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
		if pid != windows.GetCurrentProcessId() {
			return 1
		}
		buffer := make([]uint16, 256)
		user32.NewProc("GetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		if windows.UTF16ToString(buffer) == "Takealot 掌柜" {
			found = hwnd
			return 0
		}
		return 1
	})
	user32.NewProc("EnumWindows").Call(callback, 0)
	return found
}

func (d *winDriver) dispatch(ctx context.Context, job func()) error {
	d.mu.Lock()
	started, thread := d.started, d.thread
	d.mu.Unlock()
	if !started {
		return errors.New("WebView2 尚未启动")
	}
	select {
	case <-d.done:
		return errors.New("会话已关闭")
	case <-ctx.Done():
		return ctx.Err()
	case d.jobs <- job:
	}
	result, _, err := postThreadMessage.Call(uintptr(thread), sessionMessage, 0, 0)
	if result == 0 {
		return err
	}
	return nil
}
func (d *winDriver) sync(job func() error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	done := make(chan error, 1)
	if err := d.dispatch(ctx, func() { done <- job() }); err != nil {
		return err
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-d.done:
		return errors.New("会话已关闭")
	}
}

// Only the small COM ABI gaps missing from edge's public API are called here.
// All COM calls and HWND ownership stay on this driver's dedicated STA.
func callCOM(object unsafe.Pointer, index int, args ...uintptr) error {
	if object == nil {
		return errors.New("COM 对象不存在")
	}
	table := *(*unsafe.Pointer)(object)
	address := *(*uintptr)(unsafe.Add(table, index*int(unsafe.Sizeof(uintptr(0)))))
	hr, _, _ := syscall.SyscallN(address, append([]uintptr{uintptr(object)}, args...)...)
	if int32(hr) < 0 {
		return fmt.Errorf("WebView2 HRESULT 0x%x", hr)
	}
	return nil
}
func comString(object unsafe.Pointer, index int) (string, error) {
	var value *uint16
	err := callCOM(object, index, uintptr(unsafe.Pointer(&value)))
	if err != nil {
		return "", err
	}
	defer windows.CoTaskMemFree(unsafe.Pointer(value))
	return windows.UTF16PtrToString(value), nil
}

type comHandler struct {
	table *[4]uintptr
	fn    func(uintptr, uintptr) uintptr
	refs  atomic.Int32
}

var handlerQI = syscall.NewCallback(func(this, iid, out uintptr) uintptr {
	if out == 0 {
		return 0x80004003
	}
	*(*uintptr)(unsafe.Pointer(out)) = 0
	// Event handlers are IUnknown, not IAgileObject; they stay on the STA.
	guid := (*windows.GUID)(unsafe.Pointer(iid))
	if *guid != (windows.GUID{Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}) {
		return 0x80004002
	}
	*(*uintptr)(unsafe.Pointer(out)) = this
	(*comHandler)(unsafe.Pointer(this)).refs.Add(1)
	return 0
})
var handlerAddRef = syscall.NewCallback(func(this uintptr) uintptr { return uintptr((*comHandler)(unsafe.Pointer(this)).refs.Add(1)) })
var handlerRelease = syscall.NewCallback(func(this uintptr) uintptr { return releaseHandler((*comHandler)(unsafe.Pointer(this))) })
var handlerInvoke = syscall.NewCallback(func(this, a, b uintptr) uintptr { return (*comHandler)(unsafe.Pointer(this)).fn(a, b) })
var handlerTable = [4]uintptr{handlerQI, handlerAddRef, handlerRelease, handlerInvoke}

var handlerRoots sync.Map

func (v *winView) handler(fn func(uintptr, uintptr) uintptr) *comHandler {
	h := &comHandler{table: &handlerTable, fn: fn}
	h.refs.Store(1)
	handlerRoots.Store(h, true)
	return h
}
func releaseHandler(h *comHandler) uintptr {
	n := h.refs.Add(-1)
	if n == 0 {
		handlerRoots.Delete(h)
	}
	return uintptr(n)
}

type environmentCallback struct {
	driver *winDriver
	view   *winView
	done   chan error
}

func (h *environmentCallback) EnvironmentCompleted(code webviewloader.HRESULT, environment *webviewloader.ICoreWebView2Environment) webviewloader.HRESULT {
	v := h.view
	if code < 0 || environment == nil {
		h.done <- fmt.Errorf("WebView2 环境创建失败: 0x%x", uint32(code))
		return 0
	}
	if v.closed {
		h.done <- errors.New("会话已关闭")
		return 0
	}
	v.environment = unsafe.Pointer(environment)
	_ = callCOM(v.environment, 1)
	callback := v.handler(func(result, controller uintptr) uintptr {
		if int32(result) < 0 || controller == 0 {
			h.done <- errors.New("WebView2 控制器创建失败")
			return 0
		}
		v.controller = (*edge.ICoreWebView2Controller)(unsafe.Pointer(controller))
		v.controller.AddRef()
		if v.closed {
			_ = callCOM(unsafe.Pointer(v.controller), 24)
			h.done <- errors.New("会话已关闭")
			return 0
		}
		core, err := v.controller.GetCoreWebView2()
		if err != nil {
			h.done <- err
			return 0
		}
		v.core = core
		err = h.driver.setup(v)
		h.done <- err
		return 0
	})
	err := callCOM(v.environment, 3, v.hwnd, uintptr(unsafe.Pointer(callback)))
	releaseHandler(callback)
	if err != nil {
		h.done <- err
	}
	return 0
}

func (d *winDriver) Create(id, profile, proxy, origin, initialURL string) error {
	done := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	err := d.dispatch(ctx, func() {
		if existing := d.views[id]; existing != nil && !existing.closed {
			if existing.core == nil {
				done <- errors.New("会话初始化尚未完成")
			} else {
				done <- nil
			}
			return
		}
		class, _ := windows.UTF16PtrFromString("STATIC")
		hwnd, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0x40000000|0x04000000, 0, 0, 800, 500, d.parent, 0, 0, 0)
		if hwnd == 0 {
			done <- fmt.Errorf("创建内嵌验证面板失败: %w", err)
			return
		}
		v := &winView{id: id, origin: origin, hwnd: hwnd}
		d.views[id] = v
		callback := &environmentCallback{driver: d, view: v, done: done}
		args := ""
		if proxy != "" {
			args = "--proxy-server=" + proxy
		}
		err = webviewloader.CreateCoreWebView2EnvironmentWithOptions(callback, webviewloader.WithUserDataFolder(profile), webviewloader.WithAdditionalBrowserArguments(args))
		if err != nil {
			closeWinView(v)
			delete(d.views, id)
			done <- errors.New("无法启动 WebView2，请检查 Microsoft Edge WebView2 Runtime")
		}
	})
	if err != nil {
		return err
	}
	select {
	case err = <-done:
		if err != nil {
			return err
		}
		return d.sync(func() error { return d.views[id].core.Navigate(initialURL) })
	case <-ctx.Done():
		return ctx.Err()
	case <-d.done:
		return errors.New("会话已关闭")
	}
}

func (d *winDriver) setup(v *winView) error {
	if err := v.controller.PutIsVisible(false); err != nil {
		return err
	}
	settings, err := v.core.GetSettings()
	if err != nil {
		return err
	}
	defer settings.Release()
	for _, setting := range []func() error{func() error { return settings.PutAreHostObjectsAllowed(false) }, func() error { return settings.PutIsWebMessageEnabled(false) }, func() error { return settings.PutAreDefaultContextMenusEnabled(false) }} {
		if err = setting(); err != nil {
			return err
		}
	}
	core := (*wv.ICoreWebView2)(unsafe.Pointer(v.core))
	register := func(method wv.ComProc, fn func(uintptr, uintptr) uintptr) error {
		handler := v.handler(fn)
		var token int64
		hr, _, _ := method.Call(uintptr(unsafe.Pointer(v.core)), uintptr(unsafe.Pointer(handler)), uintptr(unsafe.Pointer(&token)))
		releaseHandler(handler)
		if int32(hr) < 0 {
			return fmt.Errorf("注册 WebView2 事件失败: 0x%x", hr)
		}
		return nil
	}
	if err = register(core.Vtbl.AddNavigationStarting, func(_, args uintptr) uintptr {
		target, e := comString(unsafe.Pointer(args), 3)
		if e != nil || !allowedNavigation(v.origin, target, false) {
			_ = callCOM(unsafe.Pointer(args), 8, 1)
		}
		return 0
	}); err != nil {
		return err
	}
	if err = register(core.Vtbl.AddFrameNavigationStarting, func(_, args uintptr) uintptr {
		target, e := comString(unsafe.Pointer(args), 3)
		if e != nil || !allowedNavigation(v.origin, target, true) {
			_ = callCOM(unsafe.Pointer(args), 8, 1)
		}
		return 0
	}); err != nil {
		return err
	}
	if err = register(core.Vtbl.AddNewWindowRequested, func(_, args uintptr) uintptr { _ = callCOM(unsafe.Pointer(args), 6, 1); return 0 }); err != nil {
		return err
	}

	return nil
}

func allowedNavigation(origin, target string, frame bool) bool {
	if frame && target == "about:blank" {
		return true
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return false
	}
	return "https://"+u.Host == origin || (frame && u.Host == "challenges.cloudflare.com")
}

// protocol uses WebView2's in-process DevTools API, without opening a debugging
// port. Requests execute in a separate world, so challenge/page JavaScript
// cannot replace fetch or inspect the store credential in a shared global.
func (d *winDriver) protocol(ctx context.Context, id, method string, params any) (string, error) {
	data, err := json.Marshal(params)
	if err != nil {
		return "", err
	}
	reply := make(chan evalReply, 1)
	err = d.dispatch(ctx, func() {
		if err := ctx.Err(); err != nil {
			reply <- evalReply{err: err}
			return
		}
		v := d.views[id]
		if v == nil || v.closed || v.core == nil {
			reply <- evalReply{err: errors.New("会话未就绪")}
			return
		}
		callback := v.handler(func(code, result uintptr) uintptr {
			answer := evalReply{}
			if int32(code) < 0 {
				answer.err = errors.New("WebView2 脚本协议调用失败")
			} else {
				answer.value = windows.UTF16PtrToString((*uint16)(unsafe.Pointer(result)))
			}
			reply <- answer
			return 0
		})
		methodText, _ := windows.UTF16PtrFromString(method)
		parameters, _ := windows.UTF16PtrFromString(string(data))
		core := (*wv.ICoreWebView2)(unsafe.Pointer(v.core))
		hr, _, _ := core.Vtbl.CallDevToolsProtocolMethod.Call(uintptr(unsafe.Pointer(v.core)), uintptr(unsafe.Pointer(methodText)), uintptr(unsafe.Pointer(parameters)), uintptr(unsafe.Pointer(callback)))
		var callErr error
		if int32(hr) < 0 {
			callErr = errors.New("WebView2 协议调用失败")
		}
		releaseHandler(callback)
		if callErr != nil {
			reply <- evalReply{err: errors.New("WebView2 脚本协议不可用")}
		}
	})
	if err != nil {
		return "", err
	}
	select {
	case response := <-reply:
		return response.value, response.err
	case <-ctx.Done():
		return "", ctx.Err()
	case <-d.done:
		return "", errors.New("会话已关闭")
	}
}
func (d *winDriver) Eval(ctx context.Context, id, script string) (string, error) {
	raw, err := d.protocol(ctx, id, "Page.getFrameTree", map[string]any{})
	if err != nil {
		return "", err
	}
	var frame struct {
		FrameTree struct {
			Frame struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	if json.Unmarshal([]byte(raw), &frame) != nil || frame.FrameTree.Frame.ID == "" {
		return "", errors.New("页面尚未就绪")
	}
	// The JavaScript body also checks exact origin before sending credentials.
	raw, err = d.protocol(ctx, id, "Page.createIsolatedWorld", map[string]any{"frameId": frame.FrameTree.Frame.ID, "worldName": "TakealotSession", "grantUniveralAccess": false})
	if err != nil {
		return "", err
	}
	var world struct {
		ID int `json:"executionContextId"`
	}
	if json.Unmarshal([]byte(raw), &world) != nil || world.ID == 0 {
		return "", errors.New("无法创建隔离请求环境")
	}
	raw, err = d.protocol(ctx, id, "Runtime.callFunctionOn", map[string]any{"executionContextId": world.ID, "functionDeclaration": "async function(){" + script + "}", "awaitPromise": true, "returnByValue": true})
	if err != nil {
		return "", err
	}
	var result struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
		Exception json.RawMessage `json:"exceptionDetails"`
	}
	if json.Unmarshal([]byte(raw), &result) != nil || len(result.Exception) > 0 || result.Result.Value == "" {
		return "", errors.New("隔离浏览器请求失败")
	}
	return result.Result.Value, nil
}
func (d *winDriver) Show(id string, b Bounds) error {
	return d.sync(func() error {
		var client edge.Rect
		if ok, _, err := getClientRect.Call(d.parent, uintptr(unsafe.Pointer(&client))); ok == 0 {
			return err
		}
		v := d.views[id]
		if v == nil || v.controller == nil {
			return errors.New("会话尚未就绪")
		}
		sx, sy := float64(client.Right)/b.ViewportWidth, float64(client.Bottom)/b.ViewportHeight
		width, height := int32(b.Width*sx), int32(b.Height*sy)
		moveWindow.Call(v.hwnd, uintptr(int32(b.X*sx)), uintptr(int32(b.Y*sy)), uintptr(width), uintptr(height), 1)
		if err := v.controller.PutBounds(edge.Rect{Right: width, Bottom: height}); err != nil {
			return err
		}
		showWindow.Call(v.hwnd, 5)
		return v.controller.PutIsVisible(true)
	})
}
func (d *winDriver) Hide() error {
	return d.sync(func() error {
		for _, v := range d.views {
			if v.controller != nil {
				_ = v.controller.PutIsVisible(false)
			}
			showWindow.Call(v.hwnd, 0)
		}
		return nil
	})
}
func closeWinView(v *winView) {
	if v.closed {
		return
	}
	v.closed = true

	if v.controller != nil {
		_ = callCOM(unsafe.Pointer(v.controller), 24)
		v.controller.Release()
	}
	if v.core != nil {
		v.core.Release()
	}
	if v.environment != nil {
		_ = callCOM(v.environment, 2)
	}
	destroyWindow.Call(v.hwnd)
}
func (d *winDriver) Destroy(id string) error {
	return d.sync(func() error {
		if v := d.views[id]; v != nil {
			closeWinView(v)
			delete(d.views, id)
		}
		return nil
	})
}
func (d *winDriver) Close() {
	_ = d.sync(func() error {
		for _, v := range d.views {
			closeWinView(v)
		}
		postThreadMessage.Call(uintptr(d.thread), 0x0012, 0, 0)
		return nil
	})
}
