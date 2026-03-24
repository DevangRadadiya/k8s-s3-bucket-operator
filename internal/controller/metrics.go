package controller

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	reconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bucket_operator_reconcile_total",
			Help: "Total number of BucketClaim reconcile attempts.",
		},
		[]string{"result"},
	)

	reconcileErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bucket_operator_reconcile_errors_total",
			Help: "Total number of BucketClaim reconcile errors by stage.",
		},
		[]string{"stage"},
	)

	reconcileDurationSeconds = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "bucket_operator_reconcile_duration_seconds",
			Help:    "BucketClaim reconcile duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"bucketclass"},
	)

	bucketsBoundTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "bucket_operator_buckets_bound_total",
			Help: "Total number of BucketClaims successfully bound.",
		},
		[]string{"bucketclass"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		reconcileTotal,
		reconcileErrorsTotal,
		reconcileDurationSeconds,
		bucketsBoundTotal,
	)
}

