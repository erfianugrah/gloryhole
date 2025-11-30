package api

import (
	"context"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
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

	// Get process-specific CPU (per-core percentage, needs normalization)
	proc, err := process.NewProcessWithContext(ctx, int32(os.Getpid()))
	if err == nil {
		// Get CPU percent for this process (500ms sample interval)
		// Returns per-core percentage, so normalize by dividing by number of CPUs
		if cpuPercent, err := proc.PercentWithContext(ctx, 500*time.Millisecond); err == nil {
			numCPU := runtime.NumCPU()
			if numCPU > 0 {
				// Normalize to 0-100% range by dividing by number of CPUs
				metrics.CPUPercent = cpuPercent / float64(numCPU)
			} else {
				metrics.CPUPercent = cpuPercent
			}
		} else {
			// Fallback to system-wide CPU if process metrics fail
			if percents, err := cpu.PercentWithContext(ctx, 500*time.Millisecond, false); err == nil && len(percents) > 0 {
				metrics.CPUPercent = percents[0]
			}
		}

		// Get memory info for this process
		if memInfo, err := proc.MemoryInfoWithContext(ctx); err == nil {
			metrics.MemUsed = memInfo.RSS // Resident Set Size
		}
	}

	// Get total system memory
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		metrics.MemTotal = vm.Total
		if metrics.MemTotal > 0 && metrics.MemUsed > 0 {
			metrics.MemPercent = (float64(metrics.MemUsed) / float64(metrics.MemTotal)) * 100
		}
	}

	// Temperature sensors (often unavailable in Docker/VMs)
	if temps, err := host.SensorsTemperaturesWithContext(ctx); err == nil && len(temps) > 0 {
		var sum float64
		var count float64
		for _, sensor := range temps {
			if sensor.Temperature == 0 {
				continue
			}
			sum += sensor.Temperature
			count++
			if strings.Contains(strings.ToLower(sensor.SensorKey), "package") ||
			   strings.Contains(strings.ToLower(sensor.SensorKey), "cpu") {
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
