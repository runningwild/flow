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
	disks        chan string
	ingresses    chan int
	draw         chan struct{}
	mouseDown    chan point
	mouseMove    chan point
	mouseUp      chan point
}

func MakeWorkspace(canvas *js.Object) *Workspace {
	doc := js.Global.Get("document")
	ctx := canvas.Call("getContext", "2d")
	w := &Workspace{
		doc:       doc,
		canvas:    canvas,
		ctx:       ctx,
		x:         canvas.Get("offsetLeft").Int(),
		y:         canvas.Get("offsetTop").Int(),
		dx:        canvas.Get("offsetWidth").Int(),
		dy:        canvas.Get("offsetHeight").Int(),
		images:    make(chan schema.ImageManifest),
		disks:     make(chan string),
		ingresses: make(chan int),
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

		case disk := <-w.disks:
			state.pods = append(state.pods, MakeDisk(disk, w.ctx))

		case port := <-w.ingresses:
			state.pods = append(state.pods, MakeIngress(port, w.ctx))

		case pt := <-w.mouseDown:
			for i := range state.pods {
				anch := state.pods[i].AnchorAt(pt)
				if anch != nil {
					state.connect = &edge{
						src:  anch,
						temp: pt,
					}
					break
				}
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
			if state.connect != nil {
				state.connect.temp = pt
			}

		case pt := <-w.mouseUp:
			if len(state.pods) > 0 && state.pods[0].selected {
				state.pods[0].Release(pt)
			}
			if state.connect != nil {
				for i := range state.pods {
					anch := state.pods[i].AnchorAt(pt)
					if anch != nil {
						state.connect.dst = anch
						state.connect.complete = true
						if state.connect.Valid() {
							state.edges = append(state.edges, state.connect)
						}
						break
					}
				}
				state.connect = nil
			}
		}
		w.doDraw(&state)
	}
}

type workspaceState struct {
	pods  []*pod
	edges []*edge

	connect *edge
}

type edge struct {
	src, dst *podAnchor
	temp     point
	complete bool
}

func (e *edge) Valid() bool {
	if !e.complete {
		return false
	}
	if e.src == nil || e.dst == nil {
		return false
	}

	if rf, ok := e.src.obj.(*requiredFlag); ok {
		if rf.typ == "host-port" {
			_, ok := e.dst.obj.(*types.Port)
			return ok
		}
	}

	if _, ok := e.src.obj.(portObj); ok {
		_, ok := e.dst.obj.(*types.Port)
		return ok
	}

	if _, ok := e.src.obj.(*types.MountPoint); ok {
		_, ok := e.dst.obj.(diskObj)
		return ok
	}

	return false
}

type pod struct {
	// Exactly one of the following should be non-zero
	manifest *schema.ImageManifest
	disk     string
	port     int

	selected     bool
	x, y, dx, dy int

	anchors []*podAnchor

	origin point
	drag   point
}

type podAnchor struct {
	pod    *pod
	edgePt point
	textPt point
	text   string
	obj    interface{}
	// App.Ports
	// App.MountPoints
	// requiredFlag
	// diskObj
	// portObj
}
type diskObj string
type portObj int

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
	if value[2] != "host-port" {
		SetToast(ToastError, fmt.Sprintf("Unknown required-flag type %q", value[2]))
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
		dy:       75,
	}

	ctx.Set("fillStyle", "rgb(0, 0, 0)")
	ctx.Set("textAlign", "center")
	ctx.Set("textBaseline", "middle")
	ctx.Set("font", "20px Monaco")

	imageNameWidth := ctx.Call("measureText", p.manifest.Name.String()).Get("width").Int()

	var topAnchors, botAnchors []*podAnchor

	for _, port := range p.manifest.App.Ports {
		topAnchors = append(topAnchors, &podAnchor{
			pod:    p,
			edgePt: point{0, 0},
			textPt: point{0, 0 + 12},
			text:   fmt.Sprintf("%s:%d", port.Name.String(), port.Port),
			obj:    &port,
		})
	}

	for _, mount := range p.manifest.App.MountPoints {
		botAnchors = append(botAnchors, &podAnchor{
			pod:    p,
			edgePt: point{0, p.dy},
			textPt: point{0, p.dy - 12},
			text:   mount.Name.String(),
			obj:    &mount,
		})
	}
	for _, ann := range p.manifest.Annotations {
		r := annotationToRequiredFlag(ann)
		if r != nil {
			botAnchors = append(botAnchors, &podAnchor{
				pod:    p,
				edgePt: point{0, p.dy},
				textPt: point{0, p.dy - 12},
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

func MakeDisk(name string, ctx *js.Object) *pod {
	p := &pod{
		disk: name,
		x:    10,
		y:    10,
		dx:   100,
		dy:   100,
	}
	p.anchors = append(p.anchors, &podAnchor{
		pod:    p,
		text:   "",
		edgePt: point{50, 0},
		textPt: point{50, 12},
		obj:    diskObj(name),
	})

	return p
}

func MakeIngress(port int, ctx *js.Object) *pod {
	p := &pod{
		port: port,
		x:    10,
		y:    10,
		dx:   100,
		dy:   100,
	}
	p.anchors = append(p.anchors, &podAnchor{
		pod:    p,
		text:   "",
		edgePt: point{50, 100},
		textPt: point{50, 100 - 12},
		obj:    portObj(port),
	})

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

func (p *pod) AnchorAt(pt point) *podAnchor {
	for _, anch := range p.anchors {
		dx := anch.edgePt.x + p.x - pt.x
		dy := anch.edgePt.y + p.y - pt.y
		if dx*dx+dy*dy < 100 {
			return anch
		}
	}
	return nil
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
	ctx.Set("font", "15px Monaco")
	switch {
	case p.manifest != nil:
		ctx.Call("fillText", p.manifest.Name, p.x+p.dx/2, p.y+p.dy/2)
	case p.disk != "":
		ctx.Call("fillText", p.disk, p.x+p.dx/2, p.y+p.dy/2)
	case p.port > 0:
		ctx.Call("fillText", fmt.Sprintf("port %d", p.port), p.x+p.dx/2, p.y+p.dy/2)
	}

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

func (w *Workspace) Disks() chan<- string {
	return w.disks
}

func (w *Workspace) Ingresses() chan<- int {
	return w.ingresses
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
	edges := state.edges
	if state.connect != nil {
		edges = append(edges, state.connect)
	}
	for _, e := range edges {
		if e.complete {
			w.ctx.Set("strokeStyle", "rgb(0, 0, 0)")
		} else {
			w.ctx.Set("strokeStyle", "rgb(0, 255, 0)")
		}
		w.ctx.Call("beginPath")
		w.ctx.Call("moveTo", e.src.pod.x+e.src.edgePt.x, e.src.pod.y+e.src.edgePt.y)
		if e.dst != nil {
			w.ctx.Call("lineTo", e.dst.pod.x+e.dst.edgePt.x, e.dst.pod.y+e.dst.edgePt.y)
		} else {
			w.ctx.Call("lineTo", e.temp.x, e.temp.y)
		}
		w.ctx.Call("stroke")
	}
}