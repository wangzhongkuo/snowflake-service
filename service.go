package main

import (
	"context"
	snowflakepb "git.shiyou.kingsoft.com/sdk/snowflake-service/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
}

func (s *Server) NextId(ctx context.Context, request *snowflakepb.NextIdRequest) (*snowflakepb.NextIdResponse, error) {
	id := snowflake.NextId()
	if id <= 0 {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &snowflakepb.NextIdResponse{Id: uint64(id)}, nil
}
