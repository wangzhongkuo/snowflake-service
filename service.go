package main

import (
	"context"
	snowflakepb "git.shiyou.kingsoft.com/sdk/snowflake-service/proto/snowflake"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
}

func (s Server) Generate(ctx context.Context, request *snowflakepb.GenerateRequest) (*snowflakepb.GenerateResponse, error) {
	id := snowflake.NextId()
	if id <= 0 {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &snowflakepb.GenerateResponse{Id: id}, nil
}
