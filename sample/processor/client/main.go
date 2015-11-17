package main

import (
	"flag"
	"io/ioutil"
	"log"
	"time"

	"github.com/runningwild/flow/sample/processor"
	"github.com/runningwild/flow/sample/storage"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	pAddr = flag.String("process-addr", "localhost:23423", "Addr of processor.")
	sAddr = flag.String("store-addr", "localhost:12312", "Addr of store.")
	path  = flag.String("path", "", "Path to image.")
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	flag.Parse()
	data, err := ioutil.ReadFile(*path)
	if err != nil {
		log.Fatalf("Unable to read %q: %v", *path, err)
	}

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

	pReply, err := pClient.Process(context.Background(), &processor.ProcessRequest{Image: string(data)})
	if err != nil {
		log.Fatalf("Unablet to process image: %v", err)
	}
	log.Printf("Id: %s", pReply.Id)

	for {
		time.Sleep(100 * time.Millisecond)
		sReply, err := sClient.Get(context.Background(), &storage.GetRequest{Key: pReply.Id + "-done"})
		if err != nil {
			continue
		}
		log.Printf("Success!")
		ioutil.WriteFile("out.png", []byte(sReply.Element.Val), 0777)
		break
	}
}
