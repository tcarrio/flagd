package service

import (
	"context"
	"fmt"
	"net"

	"github.com/open-feature/flagd/pkg/eval"
	"github.com/open-feature/flagd/pkg/model"
	log "github.com/sirupsen/logrus"
	gen "go.buf.build/open-feature/flagd-server/open-feature/flagd/schema/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
)

type GRPCServiceConfiguration struct {
	Port             int32
	ServerKeyPath    string
	ServerCertPath   string
	ServerSocketPath string
}

type GRPCService struct {
	GRPCServiceConfiguration *GRPCServiceConfiguration
	Eval                     eval.IEvaluator
	gen.UnimplementedServiceServer
	Logger *log.Entry
}

// Serve allows for the use of GRPC only without HTTP, where as HTTP service enables both
// GRPC and HTTP
func (s *GRPCService) Serve(ctx context.Context, eval eval.IEvaluator) error {
	var lis net.Listener
	var err error
	g, gCtx := errgroup.WithContext(ctx)
	s.Eval = eval

	// TLS
	var serverOpts []grpc.ServerOption
	if s.GRPCServiceConfiguration.ServerCertPath != "" && s.GRPCServiceConfiguration.ServerKeyPath != "" {
		config, err := loadTLSConfig(s.GRPCServiceConfiguration.ServerCertPath, s.GRPCServiceConfiguration.ServerKeyPath)
		if err != nil {
			return err
		}
		serverOpts = append(serverOpts, grpc.Creds(credentials.NewTLS(config)))
	}

	grpcServer := grpc.NewServer(serverOpts...)
	gen.RegisterServiceServer(grpcServer, s)

	if s.GRPCServiceConfiguration.ServerSocketPath != "" {
		lis, err = net.Listen("unix", s.GRPCServiceConfiguration.ServerSocketPath)
	} else {
		lis, err = net.Listen("tcp", fmt.Sprintf(":%d", s.GRPCServiceConfiguration.Port))
	}
	if err != nil {
		return err
	}

	g.Go(func() error {
		return grpcServer.Serve(lis)
	})
	<-gCtx.Done()
	grpcServer.GracefulStop()
	return nil
}

// TODO: might be able to simplify some of this with generics.
func (s *GRPCService) ResolveBoolean(
	ctx context.Context,
	req *gen.ResolveBooleanRequest,
) (*gen.ResolveBooleanResponse, error) {
	res := gen.ResolveBooleanResponse{}
	result, variant, reason, err := s.Eval.ResolveBooleanValue(req.GetFlagKey(), req.GetContext())
	if err != nil {
		return &res, s.HandleEvaluationError(err, reason)
	}
	res.Reason = reason
	res.Value = result
	res.Variant = variant
	return &res, nil
}

func (s *GRPCService) ResolveString(
	ctx context.Context,
	req *gen.ResolveStringRequest,
) (*gen.ResolveStringResponse, error) {
	res := gen.ResolveStringResponse{}
	result, variant, reason, err := s.Eval.ResolveStringValue(req.GetFlagKey(), req.GetContext())
	if err != nil {
		return &res, s.HandleEvaluationError(err, reason)
	}
	res.Reason = reason
	res.Value = result
	res.Variant = variant
	return &res, nil
}

func (s *GRPCService) ResolveInt(
	ctx context.Context,
	req *gen.ResolveIntRequest,
) (*gen.ResolveIntResponse, error) {
	res := gen.ResolveIntResponse{}
	result, variant, reason, err := s.Eval.ResolveIntValue(req.GetFlagKey(), req.GetContext())
	if err != nil {
		return &res, s.HandleEvaluationError(err, reason)
	}
	res.Reason = reason
	res.Value = result
	res.Variant = variant
	return &res, nil
}

func (s *GRPCService) ResolveFloat(
	ctx context.Context,
	req *gen.ResolveFloatRequest,
) (*gen.ResolveFloatResponse, error) {
	res := gen.ResolveFloatResponse{}
	result, variant, reason, err := s.Eval.ResolveFloatValue(req.GetFlagKey(), req.GetContext())
	if err != nil {
		return &res, s.HandleEvaluationError(err, reason)
	}
	res.Reason = reason
	res.Value = result
	res.Variant = variant
	return &res, nil
}

func (s *GRPCService) ResolveObject(
	ctx context.Context,
	req *gen.ResolveObjectRequest,
) (*gen.ResolveObjectResponse, error) {
	res := gen.ResolveObjectResponse{}
	result, variant, reason, err := s.Eval.ResolveObjectValue(req.GetFlagKey(), req.GetContext())
	if err != nil {
		return &res, s.HandleEvaluationError(err, reason)
	}
	val, err := structpb.NewStruct(result)
	if err != nil {
		return &res, s.HandleEvaluationError(err, reason)
	}
	res.Reason = reason
	res.Value = val
	res.Variant = variant
	return &res, nil
}

func (s *GRPCService) HandleEvaluationError(err error, reason string) error {
	statusCode := codes.Internal
	message := err.Error()
	switch message {
	case model.FlagNotFoundErrorCode:
		statusCode = codes.NotFound
	case model.TypeMismatchErrorCode:
		statusCode = codes.InvalidArgument
	}
	st := status.New(statusCode, message)
	stWD, err := st.WithDetails(&gen.ErrorResponse{
		ErrorCode: message,
		Reason:    "ERROR",
	})
	if err != nil {
		s.Logger.Error(err)
		return st.Err()
	}
	return stWD.Err()
}
