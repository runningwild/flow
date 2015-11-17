package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/boltdb/bolt"
	"github.com/runningwild/flow/sample/storage"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

var (
	port   = flag.Int("port", 12312, "Port to serve on.")
	dbPath = flag.String("db", "", "Path to db.")
)

func main() {
	log.SetFlags(log.Lshortfile | log.Ltime)
	flag.Parse()
	if *dbPath == "" {
		log.Fatalf("Must specify a db with --db.")
	}
	db, err := bolt.Open(*dbPath, 0644, nil)
	if err != nil {
		log.Fatal(err)
	}
	s := &server{
		db: db,
	}
	grpcServer := grpc.NewServer()
	storage.RegisterKeyValServer(grpcServer, s)
	laddr := fmt.Sprintf(":%d", *port)
	listener, err := net.Listen("tcp", laddr)
	if err != nil {
		log.Fatalf("unable to listen on %v: %v", laddr, err)
	}
	log.Fatalf("server terminated: %v", grpcServer.Serve(listener))

}

type server struct {
	db *bolt.DB
}

func (s *server) Put(ctx context.Context, req *storage.PutRequest) (*storage.PutReply, error) {
	log.Printf("Put: %s", req.Element.Key)
	err := s.db.Batch(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("world"))
		if err != nil {
			return err
		}
		if req.Element.Val == "" {
			if err := bucket.Delete([]byte(req.Element.Key)); err != nil {
				return err
			}
		} else {
			if err := bucket.Put([]byte(req.Element.Key), []byte(req.Element.Val)); err != nil {
				return err
			}
		}
		return nil
	})
	return &storage.PutReply{}, err
}

func (s *server) Get(ctx context.Context, req *storage.GetRequest) (*storage.GetReply, error) {
	log.Printf("Get: %s", req.Key)
	reply := &storage.GetReply{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("world"))
		if bucket == nil {
			return fmt.Errorf("not found")
		}
		if r := bucket.Get([]byte(req.Key)); r == nil {
			return fmt.Errorf("not found")
		} else {
			reply.Element = &storage.Element{
				Key: req.Key,
				Val: string(r),
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reply, nil
}

func (s *server) Range(ctx context.Context, req *storage.RangeRequest) (*storage.RangeReply, error) {
	reply := &storage.RangeReply{}
	err := s.db.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("world"))
		if bucket == nil {
			return fmt.Errorf("not found")
		}
		c := bucket.Cursor()
		start := req.Start
		if start == "" {
			start = req.Prefix
		}
		k, v := c.Seek([]byte(start))
		for k != nil && len(reply.Elements) < int(req.Count) && strings.HasPrefix(string(k), req.Prefix) {
			if string(k) != req.Start {
				reply.Elements = append(reply.Elements, &storage.Element{Key: string(k), Val: string(v)})
			}
			k, v = c.Next()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return reply, nil
}
