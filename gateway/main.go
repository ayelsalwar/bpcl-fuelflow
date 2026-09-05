package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	pbAuth "bpcl-fuelflow/proto/authpb"
	pbStation "bpcl-fuelflow/proto/stationpb"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// auth middleware
func AuthMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header missing or invalid"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method")
			}
			return []byte(os.Getenv("JWT_SECRET")), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token claims"})
			c.Abort()
			return
		}

		userRole := claims["role"].(string)
		// Enforce Role-Based Access Control
		roleAllowed := len(allowedRoles) == 0
		for _, role := range allowedRoles {
			if userRole == role {
				roleAllowed = true
				break
			}
		}

		if !roleAllowed {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: insufficient permissions"})
			c.Abort()
			return
		}

		// Pass data to the route handler
		c.Set("user_id", claims["user_id"])
		c.Set("role", userRole)
		c.Next()
	}
}

func main() {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("No .env file found")
	}

	// SERVICES
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

	// --- PUBLIC ROUTES ---
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

	router.GET("/inventory/station/:station_id", func(c *gin.Context) {
		req := &pbStation.GetInventoryRequest{StationId: c.Param("station_id")}
		res, err := stationClient.GetInventory(c.Request.Context(), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	})

	// --- PROTECTED ROUTES ---
	secureGroup := router.Group("/inventory")
	secureGroup.Use(AuthMiddleware("manager", "admin"))
	{
		secureGroup.POST("/deduct", func(c *gin.Context) {
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
	}

	gatewayPort := os.Getenv("APP_PORT")
	log.Printf("API Gateway running on port %s...", gatewayPort)
	if err := router.Run(":" + gatewayPort); err != nil {
		log.Fatalf("Failed to run gateway: %v", err)
	}
}
