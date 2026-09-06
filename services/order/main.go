package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pbOrder "bpcl-fuelflow/proto/orderpb"
	pbStation "bpcl-fuelflow/proto/stationpb"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Order represents the PostgreSQL schema
type Order struct {
	ID        string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	UserID    string
	StationID string
	FuelType  string
	Amount    float32
	Status    string
	CreatedAt time.Time
}

type orderServer struct {
	pbOrder.UnimplementedOrderServiceServer
	db            *gorm.DB
	stationClient pbStation.StationServiceClient
}

func (s *orderServer) PlaceOrder(ctx context.Context, req *pbOrder.PlaceOrderRequest) (*pbOrder.PlaceOrderResponse, error) {
	// 1. Delegate the inventory deduction to the Station Service
	deductReq := &pbStation.DeductFuelRequest{
		StationId: req.StationId,
		FuelType:  req.FuelType,
		Amount:    req.Amount,
	}
	deductRes, err := s.stationClient.DeductFuel(ctx, deductReq)
	if err != nil || !deductRes.Success {
		errMsg := "failed to deduct fuel"
		if deductRes != nil {
			errMsg = deductRes.Message
		}
		return nil, status.Errorf(codes.FailedPrecondition, errMsg)
	}

	// 2. If fuel was successfully deducted, record the transaction in PostgreSQL
	order := Order{
		UserID:    req.UserId,
		StationID: req.StationId,
		FuelType:  req.FuelType,
		Amount:    req.Amount,
		Status:    "COMPLETED",
	}

	if result := s.db.Create(&order); result.Error != nil {
		return nil, status.Errorf(codes.Internal, "fuel deducted but failed to save order record")
	}

	return &pbOrder.PlaceOrderResponse{
		OrderId: order.ID,
		Status:  order.Status,
		Message: "Order placed and fuel deducted successfully",
	}, nil
}

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Println("No .env file found")
	}

	// Connect to Neon PostgreSQL
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"))

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	db.AutoMigrate(&Order{})
	log.Println("Database connection established and migrations complete.")

	// Connect to Station Service
	stationConn, err := grpc.NewClient("localhost:"+os.Getenv("STATION_SERVICE_PORT"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Station service: %v", err)
	}
	defer stationConn.Close()
	stationClient := pbStation.NewStationServiceClient(stationConn)

	// Start the Order gRPC Server
	port := os.Getenv("ORDER_SERVICE_PORT")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pbOrder.RegisterOrderServiceServer(grpcServer, &orderServer{
		db:            db,
		stationClient: stationClient,
	})

	log.Printf("Order Service running on port %s...", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC server: %v", err)
	}
}
