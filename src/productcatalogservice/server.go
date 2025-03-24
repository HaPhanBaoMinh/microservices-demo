package main

import (
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	pb "github.com/GoogleCloudPlatform/microservices-demo/src/productcatalogservice/genproto"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
)

var (
	catalogMutex *sync.Mutex
	log          *logrus.Logger
	port         = "3550" // Default port for the server
)

func init() {
	log = logrus.New()
	log.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}
	log.Out = os.Stdout
	catalogMutex = &sync.Mutex{}
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Warnf("failed to load .env file: %v", err)
	}

	if os.Getenv("PORT") != "" {
		port = os.Getenv("PORT")
	}
	log.Infof("starting grpc server at :%s", port)
	run(port)
	select {}
}

func run(port string) {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatal(err)
	}

	svc, err := newProductCatalog()
	if err != nil {
		log.Fatalf("failed to initialize product catalog: %v", err)
	}

	srv := grpc.NewServer()
	pb.RegisterProductCatalogServiceServer(srv, svc)
	healthpb.RegisterHealthServer(srv, svc)

	go srv.Serve(listener)
}
