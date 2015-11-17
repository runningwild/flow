package main

import (
	"encoding/base64"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/runningwild/flow/sample/processor"
	"github.com/runningwild/flow/sample/storage"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	port  = flag.Int("port", 9990, "Port to serve on.")
	pAddr = flag.String("process-addr", "localhost:23423", "Addr of processor.")
	sAddr = flag.String("store-addr", "localhost:12312", "Addr of store.")
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	flag.Parse()

	pConn, err := grpc.Dial(*pAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Unable to dial processor: %v", err)
	}
	pClient := processor.NewProcessClient(pConn)

	sConn, err := grpc.Dial(*sAddr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Unable to dial store: %v", err)
	}
	sClient := storage.NewKeyValClient(sConn)

	s := &server{
		proc:  pClient,
		store: sClient,
	}
	log.Fatalf("%v", http.ListenAndServe(fmt.Sprintf(":%d", *port), s))
}

type server struct {
	proc  processor.ProcessClient
	store storage.KeyValClient
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/tasks":
		s.serveTasks(w, r)
	case r.URL.Path == "/":
		s.serveForm(w, r)
	case r.URL.Path == "/wait":
		s.serveWait(w, r)
	}
}

func (s *server) serveTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Add("Content-Type", "text/html")
	reply, err := s.store.Range(context.Background(), &storage.RangeRequest{Prefix: "task-", Count: 100})
	if err != nil {
		fmt.Fprintf(w, "Failed: %v", err)
		return
	}
	fmt.Fprintf(w, "Tasks</br>")
	for _, e := range reply.Elements {
		fmt.Fprintf(w, "%s</br>", e.Key)
	}
}

func (s *server) serveForm(w http.ResponseWriter, r *http.Request) {
	r.ParseMultipartForm(10000000)
	file, _, err := r.FormFile("file")
	if err != nil {
		log.Printf("Unable to parse form: %v", err)
		fmt.Fprintf(w, form)
		return
	}
	data, err := ioutil.ReadAll(file)
	reply, err := s.proc.Process(context.Background(), &processor.ProcessRequest{Image: string(data)})
	if err != nil {
		fmt.Fprintf(w, "Failed: %v", err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/wait?id=%s", reply.Id), http.StatusMovedPermanently)
}

const form = `
<html>
<head>
<meta charset="utf-8">
<link rel="stylesheet" href="http://yui.yahooapis.com/pure/0.6.0/pure-min.css">
<link rel="stylesheet" href="http://yui.yahooapis.com/pure/0.6.0/grids-responsive-min.css">
</head>

<form enctype="multipart/form-data" method="post" class="pure-form pure-form-stacked">
    <legend>New Task</legend>
    <div class="pure-g">
        <fieldset class="pure-u-1 pure-u-md-1-5">
			<label for="image">Image</label>
			<input class="pure-button" type="file" name="file" id="file">
        </fieldset>
        <button id="submit" type="submit" class="pure-button pure-button-primary">Start</button>
    </div>
</form>
</html>
`

func (s *server) serveWait(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	reply, err := s.store.Get(context.Background(), &storage.GetRequest{Key: id + "-done"})
	w.Header().Add("Content-Type", "text/html")
	if err != nil {
		reply, err := s.store.Get(context.Background(), &storage.GetRequest{Key: id})
		if err != nil {
			fmt.Fprintf(w, "No such task")
			return
		}
		fmt.Fprintf(w, waitHtml, base64.StdEncoding.EncodeToString([]byte(reply.Element.Val)))
		return
	}
	fmt.Fprintf(w, fmt.Sprintf(`<img src="data:image/png;base64,%s" />`, base64.StdEncoding.EncodeToString([]byte(reply.Element.Val))))
}

const waitHtml = `
<html>
<head>
<meta charset="utf-8">
</head>
Waiting...</br>
<img src="data:image/png;base64,%s" />
<script>
setTimeout(function(){
   window.location.reload(1);
}, 1000);
</script>
</html>
`
