// Copyright 2018 NetApp, Inc. All Rights Reserved.

package azgo

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/netapp/trident/v21/config"
)

var (
	zapiOpsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: config.OrchestratorName,
			Subsystem: "ontap",
			Name:      "ops_total",
			Help:      "The total number of handled ONTAP ZAPI operations",
		},
		[]string{"svm", "op"},
	)

	zapiOpsDurationInMsBySVMSummary = promauto.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  config.OrchestratorName,
			Subsystem:  "ontap",
			Name:       "operation_duration_in_milliseconds_by_svm",
			Help:       "The duration of operations by SVM",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
		[]string{"svm", "op"},
	)
)
