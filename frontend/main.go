package main

import (
	"encoding/json"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"io/ioutil"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/appc/spec/schema"
	"github.com/gopherjs/gopherjs/js"
)

var lastToastMu sync.Mutex
var lastToast time.Time

type ToastSeverity int

const (
	ToastNone ToastSeverity = iota
	ToastSuccess
	ToastWarning
	ToastError
)

func SetToast(id string, severity ToastSeverity, msg string) {
	toaster := js.Global.Get("document").Call("getElementById", id)
	if toaster == nil {
		js.Global.Call("alert", "No toaster")
		return
	}
	switch severity {
	case ToastNone:
		toaster.Set("className", "pure-alert")
	case ToastSuccess:
		toaster.Set("className", "pure-alert pure-alert-success")
	case ToastWarning:
		toaster.Set("className", "pure-alert pure-alert-warning")
	case ToastError:
		toaster.Set("className", "pure-alert pure-alert-error")
	default:
		toaster.Set("className", "pure-alert pure-alert-error")
		toaster.Set("innerHTML", fmt.Sprintf("Unknown severity level: %d", severity))
		return
	}
	toaster.Set("innerHTML", msg)
	now := time.Now()
	lastToastMu.Lock()
	lastToast = now
	lastToastMu.Unlock()

	go func() {
		time.Sleep(15 * time.Second)
		lastToastMu.Lock()
		subverted := now != lastToast
		lastToastMu.Unlock()
		if subverted {
			return
		}
		toaster.Set("className", "pure-alert")
		toaster.Set("innerHTML", "&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;|")
	}()
}

func main() {
	doc := js.Global.Get("document")
	canvas := doc.Call("getElementById", "workspace-canvas")
	canvas.Set("height", js.Global.Get("window").Get("innerHeight").Int()-canvas.Get("offsetTop").Int())
	canvas.Set("width", js.Global.Get("window").Get("innerWidth").Int())
	w := MakeWorkspace(canvas)

	doc.Call("addEventListener", "keypress", js.MakeFunc(func(this *js.Object, args []*js.Object) interface{} {
		if args[0].Get("keyCode").Int() == 13 {
			args[0].Call("preventDefault")
		}
		if args[0].Get("keyCode").Int() == 'x' {
			w.Cut()
		}
		return nil
	}))

	addContainer := doc.Call("getElementById", "add-container")
	containerName := doc.Call("getElementById", "container-name")
	addContainer.Call("addEventListener", "click", js.MakeFunc(func(this *js.Object, args []*js.Object) interface{} {
		go func() {
			resp, err := http.Get("/container/" + containerName.Get("value").String())
			if err != nil {
				SetToast("toaster", ToastError, fmt.Sprintf("Unable to contact server: %v", err))
				return
			}
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				SetToast("toaster", ToastError, fmt.Sprintf("Unable to parse response from server: %v", err))
				return
			}
			var manifest schema.ImageManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				SetToast("toaster", ToastError, fmt.Sprintf("Unable to parse response from server: %v", err))
				return
			}
			w.Images() <- manifest
		}()
		return nil
	}), false)
	addContainer.Set("disabled", nil)

	addDisk := doc.Call("getElementById", "add-disk")
	addDisk.Call("addEventListener", "click", js.MakeFunc(func(this *js.Object, args []*js.Object) interface{} {
		name := containerName.Get("value").String()
		if name == "" {
			return nil
		}
		go func() {
			w.Disks() <- name
		}()
		return nil
	}), false)
	addDisk.Set("disabled", nil)

	addIngress := doc.Call("getElementById", "add-ingress")
	addIngress.Call("addEventListener", "click", js.MakeFunc(func(this *js.Object, args []*js.Object) interface{} {
		str := containerName.Get("value").String()
		n, err := strconv.ParseInt(str, 10, 32)
		if err != nil || n <= 0 {
			SetToast("toaster", ToastError, fmt.Sprintf("Unable to parse %q as a positive integer", str))
			return nil
		}
		go func() {
			w.Ingresses() <- int(n)
		}()
		return nil
	}), false)
	addIngress.Set("disabled", nil)

	makeItSo := doc.Call("getElementById", "make-it-so")
	makeItSo.Call("addEventListener", "click", js.MakeFunc(func(this *js.Object, args []*js.Object) interface{} {
		go func() {
			w.MakeItSo()
		}()
		return nil
	}), false)
	makeItSo.Set("disabled", nil)
}
