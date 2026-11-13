//go:build !windows && (!darwin || !cgo)

package browsersession

import (
	"context"
	"errors"
)

type unsupportedDriver struct{}

func NewNativeDriver() Driver { return unsupportedDriver{} }
func (unsupportedDriver) Start() error {
	return errors.New("此平台暂不支持内嵌浏览器会话")
}
func (unsupportedDriver) Create(string, string, string, string, string) error {
	return errors.New("不支持内嵌会话")
}
func (unsupportedDriver) Eval(context.Context, string, string) (string, error) {
	return "", errors.New("不支持内嵌会话")
}
func (unsupportedDriver) Show(string, Bounds) error { return errors.New("不支持内嵌会话") }
func (unsupportedDriver) Hide() error               { return nil }
func (unsupportedDriver) Destroy(string) error      { return nil }
func (unsupportedDriver) Close()                    {}
