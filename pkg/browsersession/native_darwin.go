//go:build darwin && cgo

package browsersession

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework WebKit
#include <stdlib.h>
#include "native_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"unsafe"
)

type macDriver struct{}
type macReply struct {
	value string
	err   error
}
type macRequest struct {
	ctx   context.Context
	reply chan macReply
}

var macPending sync.Map
var macSequence atomic.Uint64

func NewNativeDriver() Driver { return macDriver{} }
func nativeError(value *C.char) error {
	if value == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(value))
	return errors.New(C.GoString(value))
}
func (macDriver) Start() error { return nativeError(C.browser_start()) }
func (macDriver) Create(id, profile, proxy, origin, initialURL string) error {
	if proxy != "" {
		return errors.New("macOS 持久会话暂仅支持系统代理/TUN，请清空店铺代理后使用")
	}
	a, b, c, d := C.CString(id), C.CString(profileID(profile)), C.CString(origin), C.CString(initialURL)
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(b))
	defer C.free(unsafe.Pointer(c))
	defer C.free(unsafe.Pointer(d))
	return nativeError(C.browser_create(a, b, c, d))
}
func (macDriver) Eval(ctx context.Context, id, script string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	token := macSequence.Add(1)
	channel := make(chan macReply, 1)
	macPending.Store(token, macRequest{ctx: ctx, reply: channel})
	defer macPending.Delete(token)
	a, b := C.CString(id), C.CString(script)
	defer C.free(unsafe.Pointer(a))
	defer C.free(unsafe.Pointer(b))
	C.browser_eval(C.uint64_t(token), a, b)
	select {
	case result := <-channel:
		return result.value, result.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

//export browser_can_execute
func browser_can_execute(token C.uint64_t) C.int {
	value, ok := macPending.Load(uint64(token))
	if ok && value.(macRequest).ctx.Err() == nil {
		return 1
	}
	return 0
}

//export browser_result
func browser_result(token C.uint64_t, value *C.char, problem *C.char) {
	if channel, ok := macPending.LoadAndDelete(uint64(token)); ok {
		result := macReply{value: C.GoString(value)}
		if message := C.GoString(problem); message != "" {
			result.err = errors.New(message)
		}
		channel.(macRequest).reply <- result
	}
}
func (macDriver) Show(id string, b Bounds) error {
	key := C.CString(id)
	defer C.free(unsafe.Pointer(key))
	return nativeError(C.browser_show(key, C.double(b.X), C.double(b.Y), C.double(b.Width), C.double(b.Height), C.double(b.ViewportWidth), C.double(b.ViewportHeight)))
}
func (macDriver) Hide() error { C.browser_hide(); return nil }
func (macDriver) Destroy(id string) error {
	key := C.CString(id)
	defer C.free(unsafe.Pointer(key))
	C.browser_destroy(key)
	return nil
}
func (macDriver) Close() {
	C.browser_close()
	macPending.Range(func(key, value any) bool {
		if actual, ok := macPending.LoadAndDelete(key); ok {
			actual.(macRequest).reply <- macReply{err: errors.New("会话已关闭")}
		}
		return true
	})
}
