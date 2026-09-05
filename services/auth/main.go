package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	pb "bpcl-fuelflow/proto/authpb"

	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// User represents the database schema
type User struct {
	ID           string `gorm:"primaryKey;type:uuid;default:gen_random_uuid()"`
	Name         string
	Email        string `gorm:"uniqueIndex"`
	PasswordHash string
	Role         string
	CreatedAt    time.Time
}

// authServer implements the generated protobuf interface
type authServer struct {
	pb.UnimplementedAuthServiceServer
	db *gorm.DB
}

// Register
func (s *authServer) Register(ctx context.Context, req *pb.RegisterRequest) (*pb.AuthResponse, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to hash password")
	}

	user := User{
		Name:         req.Name,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		Role:         req.Role,
	}

	if result := s.db.Create(&user); result.Error != nil {
		return nil, status.Errorf(codes.AlreadyExists, "email already registered")
	}

	return &pb.AuthResponse{
		Status:  201,
		Message: "User registered successfully",
	}, nil
}

// Login
func (s *authServer) Login(ctx context.Context, req *pb.LoginRequest) (*pb.AuthResponse, error) {
	var user User
	if result := s.db.Where("email = ?", req.Email).First(&user); result.Error != nil {
		return nil, status.Errorf(codes.NotFound, "user not found")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, status.Errorf(codes.Unauthenticated, "invalid password")
	}

	// Generate JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": user.ID,
		"role":    user.Role,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString([]byte(os.Getenv("JWT_SECRET")))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to generate token")
	}

	return &pb.AuthResponse{
		Status:  200,
		Message: "Login successful",
		Token:   tokenString,
	}, nil
}

func main() {
	// Load ENV From root
	if err := godotenv.Load("../../.env"); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}

	// connect db
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"), os.Getenv("DB_NAME"))

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// create user table
	db.AutoMigrate(&User{})
	log.Println("Database connection established and migrations complete.")

	// start auth service
	port := os.Getenv("AUTH_SERVICE_PORT")
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterAuthServiceServer(grpcServer, &authServer{db: db})

	log.Printf("Auth Service running on port %s...", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC server: %v", err)
	}
}
