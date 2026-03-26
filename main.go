package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/yourorg/envoy-vertex-extproc/processor"
)

func main() {
	// Config from env
	listenAddr := getEnv("LISTEN_ADDR", ":9001")
	secretID := getEnv("SECRET_ID", "lab/envoy-sds-test")
	secretKey := getEnv("SECRET_JSON_KEY", "api-key")
	ttl := getDurationEnv("SECRET_TTL", 5*time.Minute)

	// AWS SDK — picks up instance profile, IRSA, env vars automatically
	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("failed to load AWS config: %v", err)
	}
	smClient := secretsmanager.NewFromConfig(awsCfg)

	// Secret cache
	cache := processor.NewSecretCache(smClient, secretID, secretKey, ttl)

	// Pre-warm the cache
	if _, err := cache.Get(ctx); err != nil {
		log.Fatalf("failed to fetch initial secret: %v", err)
	}
	log.Printf("secret cache warmed for %s", secretID)

	// Safety settings
	safetySettings := processor.DefaultSafetySettings()
	log.Printf("safety settings: %+v", safetySettings)

	// gRPC server
	lis, err := net.Listen("tcp", listenAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", listenAddr, err)
	}

	srv := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(srv, processor.NewVertexProcessor(cache, safetySettings))

	// Health check for EKS readiness/liveness probes
	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	log.Printf("ext_proc server listening on %s", listenAddr)
	if err := srv.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		d, err := time.ParseDuration(v)
		if err == nil {
			return d
		}
	}
	return fallback
}
