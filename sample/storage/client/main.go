package main

import (
	"flag"
	"log"

	"github.com/runningwild/flow/sample/storage"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	addr   = flag.String("addr", "localhost:12312", "Addr of server.")
	key    = flag.String("key", "", "key")
	val    = flag.String("val", "", "val")
	prefix = flag.String("prefix", "", "prefix")
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	flag.Parse()
	conn, err := grpc.Dial(*addr, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("Unable to dial server: %v", err)
	}
	client := storage.NewKeyValClient(conn)
	switch {
	case *key != "" && *val != "":
		_, err := client.Put(context.Background(), &storage.PutRequest{Element: &storage.Element{Key: *key, Val: *val}})
		if err != nil {
			log.Fatalf("Unable to put: %v", err)
		}
	case *key != "":
		reply, err := client.Get(context.Background(), &storage.GetRequest{Key: *key})
		if err != nil {
			log.Fatalf("Unable to get: %v", err)
		}
		log.Printf("Got: %v", reply)

	case *prefix != "":
		var cur string
		for {
			reply, err := client.Range(context.Background(), &storage.RangeRequest{Start: cur, Prefix: *prefix, Count: 2})
			if err != nil {
				log.Fatalf("Unable to range: %v", err)
			}
			if len(reply.Elements) == 0 {
				break
			}
			for _, e := range reply.Elements {
				log.Printf("%v", e)
			}
			cur = reply.Elements[len(reply.Elements)-1].Key
		}

	default:
		log.Fatalf("FAIL")
	}
}
