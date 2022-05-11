package main

import (
	"context"
	"flag"
	"fmt"
	"git.shiyou.kingsoft.com/go/graceful"
	snowflakepb "git.shiyou.kingsoft.com/sdk/snowflake-service/proto"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"log"
	"net"
	"net/http"
	"time"
)

var (
	host                   string
	grpcPort               uint64
	metricsPort            uint64
	provider               string
	enableSelfPreservation bool
	consulAddress          string
	consulKeyPrefix        string
	hintWorkerId           uint64
	workerId               uint64
)

func main() {
	// ======================= parse program arguments ============================
	flag.StringVar(&host, "host", "0.0.0.0", "Which host the server listening on")
	flag.Uint64Var(&grpcPort, "rpc-port", 8080, "gRPC listen port")
	flag.Uint64Var(&metricsPort, "metrics-port", 8090, "/metrics http endpoint listen port")
	flag.StringVar(&provider, "provider", "consul", "What provider to get the snowflake worker id:[simple, consul], default is consul")
	flag.BoolVar(&enableSelfPreservation, "enable-self-preservation", true, "If the provider lost the worker id then use the latest available or the hint worker id")
	flag.StringVar(&consulAddress, "consul-address", "localhost:8500", "Address to the consul")
	flag.StringVar(&consulKeyPrefix, "consul-key-prefix", "snowflake/worker/id/", "Consul kv prefix")
	flag.Uint64Var(&hintWorkerId, "hint-worker-id", 0, "Acquire worker id start with the hint worker id")
	flag.Uint64Var(&workerId, "worker-id", 0, "Specify a worker id to the simple provider")
	flag.Parse()

	// =========================== init snowflake =================================
	var p Provider
	if provider == "simple" {
		p = getSimpleProvider(int64(workerId))
	} else {
		p = getConsulProvider(consulAddress, consulKeyPrefix, int64(hintWorkerId), enableSelfPreservation)
	}
	initSnowflake(p)

	// =========================== init gRPC server =================================
	// Create a listener on TCP port
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, grpcPort))
	if err != nil {
		log.Panicf("Failed to listen: %v", err)
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
	log.Printf("Serving gRPC on %s:%d", host, grpcPort)
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Panicf("failed to serve: %s", err)
		}
	}()

	// ======================= init metrics endpoint http server ====================
	// The Handler function provides a default handler to expose metrics
	// via an HTTP server. "/metrics" is the usual endpoint for that.
	http.Handle("/metrics", promhttp.Handler())
	// Start your http server for prometheus.
	go func() {
		if err := http.ListenAndServe(fmt.Sprintf(":%d", metricsPort), nil); err != nil {
			log.Panicf("unable to start a http server.")
		}
	}()
	log.Printf("metrics server listening at %d", metricsPort)
	shutdown := graceful.Shutdown(
		context.Background(),
		15*time.Second,
		[]graceful.Operation{
			func(ctx context.Context) {
				p.Stop()
				s.GracefulStop()
			},
		},
	)
	<-shutdown
	log.Println("Good bye ...")
}
