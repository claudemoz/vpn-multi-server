package agent

import (
	"context"
	"crypto/subtle"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// AuthorizationHeader is the metadata key carrying the bearer token.
const AuthorizationHeader = "authorization"

// TokenInterceptor rejects any call that does not carry
// "authorization: Bearer <token>" matching the configured shared secret.
func TokenInterceptor(token string) grpc.UnaryServerInterceptor {
	expected := []byte(token)

	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}

		values := md.Get(AuthorizationHeader)
		if len(values) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization")
		}

		presented, found := strings.CutPrefix(values[0], "Bearer ")
		if !found {
			return nil, status.Error(codes.Unauthenticated, "malformed authorization")
		}

		if subtle.ConstantTimeCompare([]byte(presented), expected) != 1 {
			return nil, status.Error(codes.Unauthenticated, "invalid token")
		}

		return handler(ctx, req)
	}
}
