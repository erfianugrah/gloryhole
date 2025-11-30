package api

import (
	"context"
	"os"
	"strings"

	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

type systemMetrics struct {
	CPUPercent    float64
	MemUsed       uint64
	MemTotal      uint64
	MemPercent    float64
	TemperatureC  float64
	temperatureOK bool
}

func collectSystemMetrics(ctx context.Context) systemMetrics {
	var metrics systemMetrics

	// Get process-specific metrics
	proc, err := process.NewProcessWithContext(ctx, int32(os.Getpid()))
	if err == nil {
		// Get CPU percent for this process
		if cpuPercent, err := proc.PercentWithContext(ctx, 0); err == nil {
			metrics.CPUPercent = cpuPercent
		}

		// Get memory info for this process
		if memInfo, err := proc.MemoryInfoWithContext(ctx); err == nil {
			metrics.MemUsed = memInfo.RSS // Resident Set Size
		}
	}

	// Get total system memory for context
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		metrics.MemTotal = vm.Total
		if metrics.MemTotal > 0 && metrics.MemUsed > 0 {
			metrics.MemPercent = (float64(metrics.MemUsed) / float64(metrics.MemTotal)) * 100
		}
	}

	if temps, err := host.SensorsTemperaturesWithContext(ctx); err == nil {
		// Prefer CPU package sensors; otherwise average everything
		var sum float64
		var count float64
		for _, sensor := range temps {
			if sensor.Temperature == 0 {
				continue
			}
			sum += sensor.Temperature
			count++
			if strings.Contains(strings.ToLower(sensor.SensorKey), "package") {
				metrics.TemperatureC = sensor.Temperature
				metrics.temperatureOK = true
				break
			}
		}
		if !metrics.temperatureOK && count > 0 {
			metrics.TemperatureC = sum / count
			metrics.temperatureOK = true
		}
	}

	return metrics
}

func (m systemMetrics) TemperatureAvailable() bool {
	return m.temperatureOK && m.TemperatureC != 0
}
