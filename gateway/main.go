package main

import (
	"log"
	"net/http"
	"os"

	pb "bpcl-fuelflow/proto/authpb"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}

	// conecct auth service
	authPort := os.Getenv("AUTH_SERVICE_PORT")
	conn, err := grpc.Dial("localhost:"+authPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect to Auth service: %v", err)
	}
	defer conn.Close()

	authClient := pb.NewAuthServiceClient(conn)

	// start gin server
	router := gin.Default()

	router.POST("/register", func(c *gin.Context) {
		var req pb.RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		res, err := authClient.Register(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, res)
	})

	router.POST("/login", func(c *gin.Context) {
		var req pb.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}

		res, err := authClient.Login(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// start main app server
	appPort := os.Getenv("APP_PORT")
	log.Printf("API Gateway running on port %s...", appPort)
	if err := router.Run(":" + appPort); err != nil {
		log.Fatalf("Failed to run gateway: %v", err)
	}
}
