package main

import (
	"log"
	"net/http"
	"os"

	pbAuth "bpcl-fuelflow/proto/authpb"
	pbStation "bpcl-fuelflow/proto/stationpb"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found")
	}

	// auth service
	authConn, err := grpc.NewClient("localhost:"+os.Getenv("AUTH_SERVICE_PORT"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect to Auth service: %v", err)
	}
	defer authConn.Close()
	authClient := pbAuth.NewAuthServiceClient(authConn)

	// station service
	stationConn, err := grpc.NewClient("localhost:"+os.Getenv("STATION_SERVICE_PORT"), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect to Station service: %v", err)
	}
	defer stationConn.Close()
	stationClient := pbStation.NewStationServiceClient(stationConn)

	router := gin.Default()

	// Auth Routes
	router.POST("/register", func(c *gin.Context) {
		var req pbAuth.RegisterRequest
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
		var req pbAuth.LoginRequest
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

	// Station Routes
	router.GET("/inventory/:station_id", func(c *gin.Context) {
		req := &pbStation.GetInventoryRequest{
			StationId: c.Param("station_id"),
		}
		res, err := stationClient.GetInventory(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// Deduct Fuel Route
	router.POST("/inventory/deduct", func(c *gin.Context) {
		var req pbStation.DeductFuelRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
			return
		}
		res, err := stationClient.DeductFuel(c.Request.Context(), &req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	})

	gatewayPort := os.Getenv("APP_PORT")
	log.Printf("API Gateway running on port %s...", gatewayPort)
	if err := router.Run(":" + gatewayPort); err != nil {
		log.Fatalf("Failed to run gateway: %v", err)
	}
}
