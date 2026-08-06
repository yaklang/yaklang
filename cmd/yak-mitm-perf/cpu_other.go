//go:build !linux

package main

import runtimemetrics "runtime/metrics"

const cpuMeasurementSource = "runtime/metrics(total-idle estimate)"

func readProcessCPUSeconds() float64 {
	samples := []runtimemetrics.Sample{
		{Name: "/cpu/classes/total:cpu-seconds"},
		{Name: "/cpu/classes/idle:cpu-seconds"},
	}
	runtimemetrics.Read(samples)
	if samples[0].Value.Kind() != runtimemetrics.KindFloat64 || samples[1].Value.Kind() != runtimemetrics.KindFloat64 {
		return 0
	}
	total := samples[0].Value.Float64()
	idle := samples[1].Value.Float64()
	if total <= idle {
		return 0
	}
	return total - idle
}
