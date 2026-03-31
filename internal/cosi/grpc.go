package cosi

import (
	"errors"

	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/provisioning"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func grpcErr(err error) error {
	if err == nil {
		return nil
	}

	// Preserve explicitly constructed gRPC errors.
	if _, ok := status.FromError(err); ok {
		return err
	}

	var pErr *provisioning.ProvisioningError
	if errors.As(err, &pErr) {
		if provisioning.IsTransientMinioError(pErr) {
			return status.Errorf(codes.Unavailable, "%v", pErr)
		}
		return status.Errorf(codes.Internal, "%v", pErr)
	}

	if provisioning.IsTransientMinioError(err) {
		return status.Errorf(codes.Unavailable, "%v", err)
	}

	return status.Errorf(codes.Internal, "%v", err)
}
