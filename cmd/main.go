package main

import (
	"context"
	"flag"
	"os"
	"strconv"
	"strings"

	"github.com/DevangRadadiya/k8s-s3-bucket-operator/api/v1alpha1"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/cosi"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/backend/resolve"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/controller"
	"github.com/DevangRadadiya/k8s-s3-bucket-operator/internal/minio"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var cosiEnabled bool
	var cosiDriverName string
	var cosiSocketPath string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. ")
	flag.BoolVar(&cosiEnabled, "cosi-enabled", false, "Enable the in-process COSI gRPC driver (Unix socket).")
	flag.StringVar(&cosiDriverName, "cosi-driver-name", "k8s-s3-bucket-operator", "COSI driver name returned by DriverGetInfo (must match BucketClass.driverName / BucketAccessClass.driverName).")
	flag.StringVar(&cosiSocketPath, "cosi-socket-path", "/var/lib/cosi/cosi.sock", "Unix domain socket path for COSI gRPC.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Env overrides (Helm-friendly): lets you enable COSI without changing args.
	if v := strings.TrimSpace(os.Getenv("COSI_ENABLED")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cosiEnabled = b
		}
	}
	if v := strings.TrimSpace(os.Getenv("COSI_DRIVER_NAME")); v != "" {
		cosiDriverName = v
	}
	if v := strings.TrimSpace(os.Getenv("COSI_SOCKET_PATH")); v != "" {
		cosiSocketPath = v
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "s3-bucket-operator-lock",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	minioClient, err := minio.NewClient(minio.ConfigFromEnv())
	if err != nil {
		setupLog.Error(err, "unable to initialize minio client")
		os.Exit(1)
	}

	providerResolver := resolve.NewResolver(mgr.GetClient(), minioClient)

	if err = (&controller.BucketClaimReconciler{
		Client:             mgr.GetClient(),
		Scheme:             mgr.GetScheme(),
		ProviderResolver:   providerResolver,
		EnableCOSI:         cosiEnabled,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BucketClaim")
		os.Exit(1)
	}

	if cosiEnabled {
		// Reconcile COSI access CRs + keep secrets/users in sync.
		if err := (&controller.BucketAccessReconciler{
			Client:             mgr.GetClient(),
			Scheme:             mgr.GetScheme(),
			ProviderResolver:   providerResolver,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "BucketAccess")
			os.Exit(1)
		}

		cosiSrv := cosi.NewServer(minioClient, mgr.GetClient(), cosiDriverName, cosiSocketPath)
		if err := mgr.Add(&cosiRunnable{srv: cosiSrv}); err != nil {
			setupLog.Error(err, "unable to add COSI gRPC server")
			os.Exit(1)
		}
		setupLog.Info("COSI driver enabled", "socket", cosiSocketPath, "driverName", cosiDriverName)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

type cosiRunnable struct {
	srv *cosi.Server
}

func (c *cosiRunnable) Start(ctx context.Context) error {
	return c.srv.Start(ctx)
}
