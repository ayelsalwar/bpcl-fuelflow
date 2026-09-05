package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	pb "bpcl-fuelflow/proto/stationpb"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stationServer implements the gRPC interface
type stationServer struct {
	pb.UnimplementedStationServiceServer
	redisClient *redis.Client
	mu          sync.Mutex // The lock for atomic transactions
}

// GetInventory fetches stock from Redis. If it's a new station, it defaults to 10,000L.
func (s *stationServer) GetInventory(ctx context.Context, req *pb.GetInventoryRequest) (*pb.GetInventoryResponse, error) {
	stockMap := make(map[string]float32)
	fuels := []string{"petrol", "diesel", "cng"}

	for _, fuel := range fuels {
		key := fmt.Sprintf("station:%s:%s", req.StationId, fuel)
		val, err := s.redisClient.Get(ctx, key).Float32()

		if err != nil {
			// Seed default inventory if not found
			val = 10000.0
			s.redisClient.Set(ctx, key, val, 0)
		}
		stockMap[fuel] = val
	}

	return &pb.GetInventoryResponse{
		StationId: req.StationId,
		FuelStock: stockMap,
	}, nil
}

// DeductFuel strictly checks stock and processes the deduction atomically
func (s *stationServer) DeductFuel(ctx context.Context, req *pb.DeductFuelRequest) (*pb.DeductFuelResponse, error) {
	s.mu.Lock()         // Lock the process
	defer s.mu.Unlock() // Unlock automatically when function finishes

	key := fmt.Sprintf("station:%s:%s", req.StationId, req.FuelType)

	currentStock, err := s.redisClient.Get(ctx, key).Float32()
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "fuel stock not found or initialized")
	}

	if currentStock < req.Amount {
		return &pb.DeductFuelResponse{
			Success: false,
			Message: fmt.Sprintf("Insufficient stock. Requested: %.2f, Available: %.2f", req.Amount, currentStock),
		}, nil
	}

	newStock := currentStock - req.Amount
	if err := s.redisClient.Set(ctx, key, newStock, 0).Err(); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update stock")
	}

	return &pb.DeductFuelResponse{
		Success: true,
		Message: fmt.Sprintf("Deducted %.2f. Remaining: %.2f", req.Amount, newStock),
	}, nil
}

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Println("No .env file found")
	}

	// Connect to Local Redis
	rdb := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", os.Getenv("REDIS_HOST"), os.Getenv("REDIS_PORT")),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       0,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis. Is redis-server.exe running? Error: %v", err)
	}
	log.Println("Connected to local Redis successfully.")

	port := os.Getenv("STATION_SERVICE_PORT")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterStationServiceServer(grpcServer, &stationServer{redisClient: rdb})

	log.Printf("Station Service running on port %s...", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
