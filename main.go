package main

import (
	"context"
	"flag"
	snowflakepb "git.shiyou.kingsoft.com/sdk/snowflake-service/proto/snowflake"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"net/http"
)

var (
	rpcPort         string
	httpPort        string
	provider        string
	consulAddress   string
	consulKeyPrefix string
	workerId        int64
)

func main() {
	// ======================= parse program arguments ============================
	flag.StringVar(&rpcPort, "rpc_port", "8080", "gGPC listen port")
	flag.StringVar(&httpPort, "http_port", "8090", "http listen port")
	flag.StringVar(&provider, "provider", "consul", "what provider to get the snowflake worker id:[simple, consul], default is consul")
	flag.StringVar(&consulAddress, "consul_address", "localhost:8500", "address to the consul")
	flag.StringVar(&consulKeyPrefix, "consul_key_prefix", "snowflake/worker/id/", "consul kv prefix")
	flag.Int64Var(&workerId, "worker_id", 0, "specify a worker id to the simple provider")
	flag.Parse()

	// =========================== init snowflake =================================
	var p Provider
	if provider == "simple" {
		p = getSimpleProvider(workerId)
	} else {
		p = getConsulProvider(consulAddress, consulKeyPrefix)
	}
	initSnowflake(p)

	// =========================== init gRPC server =================================
	// Create a listener on TCP port
	lis, err := net.Listen("tcp", ":"+rpcPort)
	if err != nil {
		log.Fatalln("Failed to listen:", err)
	}
	// Create a gRPC server object
	recovery_opts := []grpc_recovery.Option{
		grpc_recovery.WithRecoveryHandler(func(p interface{}) (err error) {
			return status.Errorf(codes.Internal, "panic triggered.")
		}),
	}
	s := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpc_prometheus.UnaryServerInterceptor,
			grpc_recovery.UnaryServerInterceptor(recovery_opts...),
		),
	)
	snowflakepb.RegisterSnowflakeServer(s, &Server{})
	grpc_prometheus.Register(s)
	// Serve gRPC server
	log.Println("Serving gRPC on 0.0.0.0:" + rpcPort)
	go func() {
		log.Fatalln(s.Serve(lis))
	}()

	// =========================== init http server =================================
	// Create a client connection to the gRPC server we just started
	// This is where the gRPC-Gateway proxies the requests
	conn, err := grpc.DialContext(
		context.Background(),
		"0.0.0.0:"+rpcPort,
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalln("Failed to dial server:", err)
	}
	gatewayMux := runtime.NewServeMux()

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/", gatewayMux)

	err = snowflakepb.RegisterSnowflakeHandler(context.Background(), gatewayMux, conn)
	if err != nil {
		log.Fatalln("Failed to register gateway:", err)
	}
	gwServer := &http.Server{
		Addr:    ":" + httpPort,
		Handler: mux,
	}
	log.Println("Serving gRPC-Gateway on http://0.0.0.0:" + httpPort)
	log.Fatalln(gwServer.ListenAndServe())
}
