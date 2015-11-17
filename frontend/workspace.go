package main

import (
	"fmt"
	"regexp"

	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
	"github.com/gopherjs/gopherjs/js"
)

type Workspace struct {
	doc          *js.Object
	canvas, ctx  *js.Object
	x, y, dx, dy int
	images       chan schema.ImageManifest
	draw         chan struct{}
	mouseDown    chan point
	mouseMove    chan point
	mouseUp      chan point
}

func MakeWorkspace(canvas *js.Object) *Workspace {
	doc := js.Global.Get("document")
	ctx := canvas.Call("getContext", "2d")
	// canvas.Set("font-kerning", "normal")
	// canvas.Set("text-rendering", "optimizeLegibility")
	// js.Global.Call("alert", fmt.Sprintf("%q %q", canvas.Get("font-kerning").String(), canvas.Get("text-rendering").String()))
	w := &Workspace{
		doc:       doc,
		canvas:    canvas,
		ctx:       ctx,
		x:         canvas.Get("offsetLeft").Int(),
		y:         canvas.Get("offsetTop").Int(),
		dx:        canvas.Get("offsetWidth").Int(),
		dy:        canvas.Get("offsetHeight").Int(),
		images:    make(chan schema.ImageManifest),
		draw:      make(chan struct{}),
		mouseDown: make(chan point),
		mouseMove: make(chan point),
		mouseUp:   make(chan point),
	}
	doc.Call("addEventListener", "mousedown", js.MakeFunc(w.onMouseDown), "false")
	doc.Call("addEventListener", "mousemove", js.MakeFunc(w.onMouseMove), "false")
	doc.Call("addEventListener", "mouseup", js.MakeFunc(w.onMouseUp), "false")
	go w.run()
	return w
}

func (w *Workspace) run() {
	var state workspaceState
	for {
		select {
		case <-w.draw:
			// Let's us force a draw if we need to for some reason.

		case im := <-w.images:
			state.pods = append(state.pods, MakePod(&im, w.ctx))

		case pt := <-w.mouseDown:
			for i := range state.pods {
				if state.pods[i].Contains(pt) {
					pods := []*pod{state.pods[i]}
					for j := range state.pods {
						if j != i {
							pods = append(pods, state.pods[j])
						}
					}
					state.pods = pods
					state.pods[0].Click(pt)
					break
				}
			}

		case pt := <-w.mouseMove:
			if len(state.pods) > 0 && state.pods[0].selected {
				state.pods[0].Move(pt)
			}

		case pt := <-w.mouseUp:
			if len(state.pods) > 0 && state.pods[0].selected {
				state.pods[0].Release(pt)
			}

		}
		w.doDraw(&state)
	}
}

type workspaceState struct {
	pods []*pod
}

type pod struct {
	manifest     *schema.ImageManifest
	selected     bool
	x, y, dx, dy int

	anchors []*podAnchor

	origin point
	drag   point
}

type podAnchor struct {
	edgePt point
	textPt point
	text   string
	obj    interface{}
}

var requiredFlagNameRe = regexp.MustCompile(`required-flag/(.*)`)
var requiredFlagValueRe = regexp.MustCompile(`name=(.*);type=(.*)`)

type requiredFlag struct {
	name string
	flag string
	typ  string
}

func annotationToRequiredFlag(ann types.Annotation) *requiredFlag {
	name := requiredFlagNameRe.FindStringSubmatch(ann.Name.String())
	if len(name) == 0 {
		return nil
	}
	value := requiredFlagValueRe.FindStringSubmatch(ann.Value)
	if len(value) == 0 {
		return nil
	}
	return &requiredFlag{
		name: name[1],
		flag: value[1],
		typ:  value[2],
	}
}

func MakePod(manifest *schema.ImageManifest, ctx *js.Object) *pod {
	if manifest.App == nil {
		return nil
	}
	p := &pod{
		manifest: manifest,
		x:        10,
		y:        10,
		dy:       125,
	}

	ctx.Set("fillStyle", "rgb(0, 0, 0)")
	ctx.Set("textAlign", "center")
	ctx.Set("font", "20px Monaco")

	imageNameWidth := ctx.Call("measureText", p.manifest.Name.String()).Get("width").Int()

	var topAnchors, botAnchors []*podAnchor

	for _, port := range p.manifest.App.Ports {
		topAnchors = append(topAnchors, &podAnchor{
			edgePt: point{0, 0},
			textPt: point{0, 0 + 20},
			text:   fmt.Sprintf("%s:%d", port.Name.String(), port.Port),
			obj:    &port,
		})
	}

	for _, mount := range p.manifest.App.MountPoints {
		botAnchors = append(botAnchors, &podAnchor{
			edgePt: point{0, p.dy},
			textPt: point{0, p.dy - 20},
			text:   mount.Name.String(),
			obj:    &mount,
		})
	}
	for _, ann := range p.manifest.Annotations {
		r := annotationToRequiredFlag(ann)
		if r != nil {
			botAnchors = append(botAnchors, &podAnchor{
				edgePt: point{0, p.dy},
				textPt: point{0, p.dy - 20},
				text:   r.name,
				obj:    r,
			})
		}
	}

	const buffer = 75
	topWidth := buffer
	for _, anch := range topAnchors {
		topWidth += ctx.Call("measureText", anch.text).Get("width").Int()
		topWidth += buffer
	}

	botWidth := buffer
	for _, anch := range botAnchors {
		botWidth += ctx.Call("measureText", anch.text).Get("width").Int()
		botWidth += buffer
	}

	p.dx = imageNameWidth
	if topWidth > p.dx {
		p.dx = topWidth
	}
	if botWidth > p.dx {
		p.dx = botWidth
	}

	for i, anch := range topAnchors {
		x := ((i + 1) * p.dx) / (1 + len(topAnchors))
		anch.edgePt.x = x
		anch.textPt.x = x
		p.anchors = append(p.anchors, anch)
	}

	for i, anch := range botAnchors {
		x := ((i + 1) * p.dx) / (1 + len(botAnchors))
		anch.edgePt.x = x
		anch.textPt.x = x
		p.anchors = append(p.anchors, anch)
	}

	return p
}

func (p *pod) Click(pt point) {
	p.drag = pt
	p.origin = point{p.x, p.y}
	p.selected = true
}

func (p *pod) Move(pt point) {
	p.x = p.origin.x + pt.x - p.drag.x
	p.y = p.origin.y + pt.y - p.drag.y
}

func (p *pod) Release(pt point) {
	p.x = p.origin.x + pt.x - p.drag.x
	p.y = p.origin.y + pt.y - p.drag.y
	p.selected = false
}

func (p *pod) Contains(pt point) bool {
	return pt.x >= p.x && pt.x < p.x+p.dx && pt.y >= p.y && pt.y < p.y+p.dy
}

type point struct {
	x, y int
}

func (p *pod) Draw(ctx *js.Object) {
	if p.selected {
		ctx.Set("fillStyle", "rgb(0, 255, 0)")
	} else {
		ctx.Set("fillStyle", "rgb(0, 0, 0)")
	}
	ctx.Call("fillRect", p.x, p.y, p.dx, p.dy)
	ctx.Set("fillStyle", "rgb(255, 255, 255)")
	ctx.Call("fillRect", p.x+1, p.y+1, p.dx-2, p.dy-2)

	ctx.Set("fillStyle", "rgb(0, 0, 0)")
	ctx.Set("textAlign", "center")
	ctx.Set("font", "20px Monaco")
	ctx.Call("fillText", p.manifest.Name, p.x+p.dx/2, p.y+p.dy/2)

	for _, anchor := range p.anchors {
		ctx.Call("fillText", anchor.text, anchor.textPt.x+p.x, anchor.textPt.y+p.y)
		ctx.Call("beginPath")
		ctx.Call("arc", anchor.edgePt.x+p.x, anchor.edgePt.y+p.y, 5, 0, 7)
		ctx.Call("fill")
	}
}

func (w *Workspace) Images() chan<- schema.ImageManifest {
	return w.images
}

func (w *Workspace) getEventPosition(e *js.Object) (x, y, cx, cy int, in bool) {
	w.x = w.canvas.Get("offsetLeft").Int()
	w.y = w.canvas.Get("offsetTop").Int()
	x = e.Get("pageX").Int() - w.x
	y = e.Get("pageY").Int() - w.y
	cx = x
	cy = y
	if cx < 0 {
		cx = 0
	}
	if cx > w.dx {
		cx = w.dx
	}
	if cy < 0 {
		cy = 0
	}
	if cy > w.dy {
		cy = w.dy
	}
	in = x >= 0 && y >= 0 && x < w.dx && y < w.dy
	return
}

func (w *Workspace) onMouseDown(this *js.Object, args []*js.Object) interface{} {
	x, y, _, _, _ := w.getEventPosition(args[0])
	go func() {
		w.mouseDown <- point{x, y}
	}()
	return nil
}

func (w *Workspace) onMouseMove(this *js.Object, args []*js.Object) interface{} {
	x, y, _, _, _ := w.getEventPosition(args[0])
	go func() {
		w.mouseMove <- point{x, y}
	}()
	return nil
}

func (w *Workspace) onMouseUp(this *js.Object, args []*js.Object) interface{} {
	x, y, _, _, _ := w.getEventPosition(args[0])
	go func() {
		w.mouseUp <- point{x, y}
	}()
	return nil
}

func (w *Workspace) doDraw(state *workspaceState) {
	w.ctx.Call("clearRect", 0, 0, w.dx, w.dy)
	for i := len(state.pods) - 1; i >= 0; i-- {
		state.pods[i].Draw(w.ctx)
	}
}
