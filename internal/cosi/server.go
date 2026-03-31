package cosi

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"

	v1alpha1 "github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/minio"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/provisioning"
	"github.com/go-logr/logr"
	cosipb "sigs.k8s.io/container-object-storage-interface-spec"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Sidecar credential wiring (see container-object-storage-interface-provisioner-sidecar/pkg/consts/consts.go).
const (
	credMapKeyS3            = "s3"
	credSecretAccessKeyID   = "accessKeyID"
	credSecretAccessSecret  = "accessSecretKey"
)

// Server implements the COSI gRPC services.
//
// Note: the published Go module is still `cosi.v1alpha1` even though the Kubernetes API evolves independently.
type Server struct {
	cosipb.UnimplementedIdentityServer
	cosipb.UnimplementedProvisionerServer

	Log        logr.Logger
	Minio      *minio.Client
	KubeClient client.Client
	DriverName string

	sockPath string
	grpcSrv  *grpc.Server
	lis      net.Listener
}

func NewServer(minioClient *minio.Client, kube client.Client, driverName, socketPath string) *Server {
	return &Server{
		Log:        ctrl.Log.WithName("cosi"),
		Minio:      minioClient,
		KubeClient: kube,
		DriverName: driverName,
		sockPath:   socketPath,
	}
}

func (s *Server) Start(ctx context.Context) error {
	if strings.TrimSpace(s.sockPath) == "" {
		return fmt.Errorf("COSI socket path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(s.sockPath), 0o755); err != nil {
		return fmt.Errorf("create cosi socket dir: %w", err)
	}
	_ = os.Remove(s.sockPath)

	lis, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("listen unix %q: %w", s.sockPath, err)
	}
	if err := os.Chmod(s.sockPath, 0o660); err != nil {
		return fmt.Errorf("chmod cosi socket: %w", err)
	}

	grpcSrv := grpc.NewServer()
	cosipb.RegisterIdentityServer(grpcSrv, s)
	cosipb.RegisterProvisionerServer(grpcSrv, s)

	s.lis = lis
	s.grpcSrv = grpcSrv

	errCh := make(chan error, 1)
	go func() {
		errCh <- grpcSrv.Serve(lis)
	}()

	s.Log.Info("starting COSI gRPC server", "socket", s.sockPath)

	select {
	case <-ctx.Done():
		grpcSrv.GracefulStop()
		<-errCh
		return nil
	case err := <-errCh:
		return err
	}
}

func (s *Server) DriverGetInfo(ctx context.Context, req *cosipb.DriverGetInfoRequest) (*cosipb.DriverGetInfoResponse, error) {
	_ = ctx
	_ = req
	if strings.TrimSpace(s.DriverName) == "" {
		return nil, status.Errorf(codes.FailedPrecondition, "driver name is not configured")
	}
	return &cosipb.DriverGetInfoResponse{Name: s.DriverName}, nil
}

func (s *Server) DriverCreateBucket(ctx context.Context, req *cosipb.DriverCreateBucketRequest) (*cosipb.DriverCreateBucketResponse, error) {
	if req == nil || strings.TrimSpace(req.Name) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bucket name is required")
	}

	claim, err := BuildSyntheticClaim("cosi", req.Name, req.GetParameters())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := provisioning.ApplyClaimParameterExtensions(claim, req.GetParameters()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	class, err := syntheticClassFromParameters(s.DriverName, claim.Spec.BucketClassName, req.GetParameters())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	bucketName, err := provisioning.ProvisionBucket(ctx, s.Minio, claim, class)
	if err != nil {
		return nil, grpcErr(err)
	}

	region := ""
	if class.Parameters != nil {
		region = strings.TrimSpace(class.Parameters["region"])
	}
	if region == "" {
		region = "us-east-1"
	}

	return &cosipb.DriverCreateBucketResponse{
		BucketId: bucketName,
		BucketInfo: &cosipb.Protocol{
			Type: &cosipb.Protocol_S3{
				S3: &cosipb.S3{
					Region:           region,
					SignatureVersion: cosipb.S3SignatureVersion_S3V4,
				},
			},
		},
	}, nil
}

func (s *Server) DriverDeleteBucket(ctx context.Context, req *cosipb.DriverDeleteBucketRequest) (*cosipb.DriverDeleteBucketResponse, error) {
	if req == nil || strings.TrimSpace(req.BucketId) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bucket_id is required")
	}

	bucketObjectName := strings.TrimSpace(req.BucketId)
	// `bucket_id` is the driver's identifier returned from DriverCreateBucket; for us it is the
	// actual backend bucket name (lowercased).
	bucketName := strings.ToLower(bucketObjectName)

	// Prefer the Kubernetes `Bucket` object's deletion policy (authoritative for COSI).
	if s.KubeClient != nil {
		bk := &unstructured.Unstructured{}
		bk.SetGroupVersionKind(BucketGVK())
		bk.SetName(bucketObjectName)

		if err := s.KubeClient.Get(ctx, client.ObjectKey{Name: bucketObjectName}, bk); err == nil {
			deletionPolicy, _, _ := unstructured.NestedString(bk.Object, "spec", "deletionPolicy")
			if strings.TrimSpace(deletionPolicy) != string(v1alpha1.DeletionPolicyDelete) {
				return &cosipb.DriverDeleteBucketResponse{}, nil
			}
		} else {
			// If we can't read the Bucket, fall back to a best-effort guess from naming scheme.
			bucketClassName := BucketClassNameFromBucketObjectName(bucketObjectName)
			if bucketClassName != "" {
				class, err := syntheticClassFromParameters(s.DriverName, bucketClassName, nil)
				if err != nil {
					return nil, grpcErr(err)
				}
				if class.DeletionPolicy != v1alpha1.DeletionPolicyDelete {
					return &cosipb.DriverDeleteBucketResponse{}, nil
				}
			} else {
				s.Log.Info("COSI DriverDeleteBucket: unable to determine deletion policy; skipping backend delete", "bucketObject", bucketObjectName)
				return &cosipb.DriverDeleteBucketResponse{}, nil
			}
		}
	} else {
		// No kube client configured: be conservative.
		s.Log.Info("COSI DriverDeleteBucket: kube client not configured; skipping backend delete", "bucket", bucketName)
		return &cosipb.DriverDeleteBucketResponse{}, nil
	}

	if err := s.Minio.DeleteBucket(ctx, bucketName); err != nil {
		return nil, grpcErr(err)
	}
	return &cosipb.DriverDeleteBucketResponse{}, nil
}

func (s *Server) DriverGrantBucketAccess(ctx context.Context, req *cosipb.DriverGrantBucketAccessRequest) (*cosipb.DriverGrantBucketAccessResponse, error) {
	if req == nil || strings.TrimSpace(req.BucketId) == "" || strings.TrimSpace(req.Name) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bucket_id and name are required")
	}

	switch req.AuthenticationType {
	case cosipb.AuthenticationType_Key:
	case cosipb.AuthenticationType_IAM:
		return nil, status.Errorf(codes.Unimplemented, "IAM authentication is not supported")
	default:
		return nil, status.Errorf(codes.InvalidArgument, "unsupported authentication type")
	}

	bucketObjectName := strings.TrimSpace(req.BucketId)
	claim, err := BuildSyntheticClaimForAccess("cosi", bucketObjectName, req.Name, req.GetParameters())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := provisioning.ApplyClaimParameterExtensions(claim, req.GetParameters()); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}

	existingAccessKey := ""
	existingSecretKey := ""
	if req.Parameters != nil {
		existingAccessKey = strings.TrimSpace(req.Parameters["existingAccessKey"])
		existingSecretKey = strings.TrimSpace(req.Parameters["existingSecretKey"])
	}

	creds, err := provisioning.GrantBucketAccess(ctx, s.Minio, claim, strings.ToLower(bucketObjectName), existingAccessKey, existingSecretKey)
	if err != nil {
		return nil, grpcErr(err)
	}

	accountID := fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
	return &cosipb.DriverGrantBucketAccessResponse{
		AccountId: accountID,
		Credentials: map[string]*cosipb.CredentialDetails{
			credMapKeyS3: {
				Secrets: map[string]string{
					credSecretAccessKeyID:  creds.AccessKeyID,
					credSecretAccessSecret: creds.AccessSecretKey,
				},
			},
		},
	}, nil
}

func (s *Server) DriverRevokeBucketAccess(ctx context.Context, req *cosipb.DriverRevokeBucketAccessRequest) (*cosipb.DriverRevokeBucketAccessResponse, error) {
	if req == nil || strings.TrimSpace(req.BucketId) == "" || strings.TrimSpace(req.AccountId) == "" {
		return nil, status.Errorf(codes.InvalidArgument, "bucket_id and account_id are required")
	}

	bucketObjectName := strings.ToLower(strings.TrimSpace(req.BucketId))

	// We encode account_id as "<namespace>-<claimName>" in DriverGrantBucketAccess.
	accountID := strings.TrimSpace(req.AccountId)
	parts := strings.SplitN(accountID, "-", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, status.Errorf(codes.InvalidArgument, "invalid account_id")
	}

	claim := &v1alpha1.BucketClaim{}
	claim.Namespace = parts[0]
	claim.Name = parts[1]

	expected := fmt.Sprintf("%s-%s", claim.Namespace, claim.Name)
	if expected != accountID {
		return nil, status.Errorf(codes.InvalidArgument, "invalid account_id")
	}

	if err := provisioning.RevokeBucketAccess(ctx, s.Minio, claim, bucketObjectName, ""); err != nil {
		return nil, grpcErr(err)
	}
	return &cosipb.DriverRevokeBucketAccessResponse{}, nil
}

func syntheticClassFromParameters(driverName, bucketClassName string, parameters map[string]string) (*v1alpha1.BucketClass, error) {
	bucketClassName = strings.TrimSpace(bucketClassName)
	if bucketClassName == "" {
		return nil, fmt.Errorf("bucketClassName is empty")
	}

	class := &v1alpha1.BucketClass{}
	class.Name = bucketClassName
	class.DriverName = strings.TrimSpace(driverName)

	// Defaults: match our samples / operator expectations.
	class.DeletionPolicy = v1alpha1.DeletionPolicyRetain
	class.ObjectLockingEnabled = false

	if parameters != nil {
		if v := strings.TrimSpace(parameters["deletionPolicy"]); v != "" {
			class.DeletionPolicy = v1alpha1.DeletionPolicy(v)
		}
		if v := strings.TrimSpace(parameters["objectLockingEnabled"]); v == "true" || v == "1" {
			class.ObjectLockingEnabled = true
		}
		if v := strings.TrimSpace(parameters["retentionMode"]); v != "" {
			class.RetentionMode = v1alpha1.RetentionMode(v)
		}
	}

	// Start from caller-provided parameters and then overlay known keys explicitly.
	class.Parameters = map[string]string{}
	if parameters != nil {
		for k, v := range parameters {
			class.Parameters[k] = v
		}
	}
	class.Parameters["region"] = strings.TrimSpace(class.Parameters["region"])
	if class.Parameters["region"] == "" {
		class.Parameters["region"] = "us-east-1"
	}

	return class, nil
}
