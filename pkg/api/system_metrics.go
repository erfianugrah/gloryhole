package api

import (
	"context"
	"strings"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
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

	if percentages, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(percentages) > 0 {
		metrics.CPUPercent = percentages[0]
	}

	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		metrics.MemUsed = vm.Used
		metrics.MemTotal = vm.Total
		metrics.MemPercent = vm.UsedPercent
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
