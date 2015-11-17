package main

import (
	"encoding/json"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"io/ioutil"
	"net/http"

	"github.com/appc/spec/schema"
	"github.com/gopherjs/gopherjs/js"
)

func main() {
	doc := js.Global.Get("document")
	doc.Call("addEventListener", "keypress", js.MakeFunc(func(this *js.Object, args []*js.Object) interface{} {
		if args[0].Get("keyCode").Int() == 13 {
			args[0].Call("preventDefault")
		}
		return nil
	}))
	canvas := doc.Call("getElementById", "workspace-canvas")
	canvas.Set("height", js.Global.Get("window").Get("innerHeight").Int()-canvas.Get("offsetTop").Int())
	canvas.Set("width", js.Global.Get("window").Get("innerWidth").Int())

	w := MakeWorkspace(canvas)

	addContainer := doc.Call("getElementById", "add-container")
	containerName := doc.Call("getElementById", "container-name")
	addContainer.Call("addEventListener", "click", js.MakeFunc(func(this *js.Object, args []*js.Object) interface{} {
		go func() {
			resp, err := http.Get("http://localhost:9090/container/" + containerName.Get("value").String())
			if err != nil {
				js.Global.Call("alert", err)
				return
			}
			data, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				js.Global.Call("alert", err)
				return
			}
			var manifest schema.ImageManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				js.Global.Call("alert", fmt.Sprintf("Unsbale to unmarshal: %v", err))
				return
			}
			w.Images() <- manifest
		}()
		return nil
	}), false)
	addContainer.Set("disabled", nil)
}
