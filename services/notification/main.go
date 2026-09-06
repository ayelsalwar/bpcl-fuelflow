package main

import (
	pb "bpcl-fuelflow/proto/notificationpb"
	"context"
	"log"
	"net"
	"os"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
)

type notificationServer struct {
	pb.UnimplementedNotificationServiceServer
	jobs chan *pb.SendNotificationRequest
}

// worker function to process notification jobs
func worker(jobs <-chan *pb.SendNotificationRequest) {
	for req := range jobs {
		log.Printf("NOTIFICATION SENT! Order ID: %s | User ID: %s | Message: %s", req.OrderId, req.UserId, req.Message)
	}
}

// Send Notifciation
func (s *notificationServer) SendNotification(ctx context.Context, req *pb.SendNotificationRequest) (*pb.SendNotificationResponse, error) {
	select {
	case s.jobs <- req:
	default:
		log.Println("Notification Channel Full, dropping notification")
	}

	return &pb.SendNotificationResponse{Success: true}, nil
}

func main() {
	if err := godotenv.Load("../../.env"); err != nil {
		log.Println("No .env file found")
	}

	jobs := make(chan *pb.SendNotificationRequest, 100)

	go worker(jobs)

	port := os.Getenv("NOTIFICATION_SERVICE_PORT")

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterNotificationServiceServer(grpcServer, &notificationServer{jobs: jobs})

	log.Printf("Notification Service running on port %s...", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}
