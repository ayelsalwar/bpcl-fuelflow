package main

import (
	"context"
	"testing"

	pb "bpcl-fuelflow/proto/stationpb"

	"github.com/go-redis/redismock/v9"
)

func TestDeductFuel(t *testing.T) {
	// 1. Set up the Mock Redis Client
	db, mock := redismock.NewClientMock()
	server := &stationServer{
		redisClient: db,
	}

	// 2. Define the Table of Test Cases
	tests := []struct {
		name           string
		req            *pb.DeductFuelRequest
		mockSetup      func()
		expectSuccess  bool
		expectedErrMsg string
	}{
		{
			name: "Successful Deduction",
			req: &pb.DeductFuelRequest{
				StationId: "TEST-STATION",
				FuelType:  "petrol",
				Amount:    20.0,
			},
			mockSetup: func() {
				mock.ExpectGet("station:TEST-STATION:petrol").SetVal("100")
				mock.ExpectSet("station:TEST-STATION:petrol", float32(80.0), 0).SetVal("OK")
			},
			expectSuccess:  true,
			expectedErrMsg: "",
		},
		{
			name: "Insufficient Stock",
			req: &pb.DeductFuelRequest{
				StationId: "TEST-STATION",
				FuelType:  "diesel",
				Amount:    500.0,
			},
			mockSetup: func() {
				mock.ExpectGet("station:TEST-STATION:diesel").SetVal("50")
			},
			expectSuccess:  false,
			expectedErrMsg: "", // The gRPC call succeeds, but the business logic returns success=false
		},
		{
			name: "Station Not Initialized",
			req: &pb.DeductFuelRequest{
				StationId: "GHOST-STATION",
				FuelType:  "cng",
				Amount:    10.0,
			},
			mockSetup: func() {
				mock.ExpectGet("station:GHOST-STATION:cng").RedisNil()
			},
			expectSuccess:  false,
			expectedErrMsg: "rpc error: code = NotFound desc = fuel stock not found or initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			res, err := server.DeductFuel(context.Background(), tt.req)

			if tt.expectedErrMsg != "" {
				if err == nil {
					t.Errorf("expected error %q, got nil", tt.expectedErrMsg)
				} else if err.Error() != tt.expectedErrMsg {
					t.Errorf("expected error %q, got %q", tt.expectedErrMsg, err.Error())
				}
				return // Test passed
			}

			// Validate Success Conditions
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if res.Success != tt.expectSuccess {
				t.Errorf("expected success=%v, got %v", tt.expectSuccess, res.Success)
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("there were unfulfilled Redis expectations: %s", err)
			}
		})
	}
}
