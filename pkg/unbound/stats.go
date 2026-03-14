package unbound

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Stats holds parsed Unbound statistics.
type Stats struct {
	TotalQueries    int64            `json:"total_queries"`
	CacheHits       int64            `json:"cache_hits"`
	CacheMiss       int64            `json:"cache_miss"`
	CacheHitRate    float64          `json:"cache_hit_rate"`
	AvgRecursionMs  float64          `json:"avg_recursion_ms"`
	MsgCacheCount   int64            `json:"msg_cache_count"`
	RRSetCacheCount int64            `json:"rrset_cache_count"`
	MemTotalBytes   int64            `json:"mem_total_bytes"`
	UptimeSeconds   int64            `json:"uptime_seconds"`
	QueryTypes      map[string]int64 `json:"query_types"`
	ResponseCodes   map[string]int64 `json:"response_codes"`
}

// statsCache caches stats results to avoid hammering unbound-control.
type statsCache struct {
	mu        sync.Mutex
	stats     *Stats
	fetchedAt time.Time
	ttl       time.Duration
}

// GetStats retrieves Unbound statistics, cached for 5 seconds.
func (s *Supervisor) GetStats() (*Stats, error) {
	s.mu.Lock()
	controlBin := s.controlBin
	socket := s.cfg.ControlSocket
	s.mu.Unlock()

	if controlBin == "" {
		return nil, fmt.Errorf("unbound-control not available")
	}

	s.mu.Lock()
	configPath := s.cfg.ConfigPath
	s.mu.Unlock()

	out, err := exec.Command(controlBin, "-c", configPath, "-s", socket, "stats_noreset").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("unbound-control stats: %w: %s", err, out)
	}

	return parseStats(string(out)), nil
}

func parseStats(output string) *Stats {
	s := &Stats{
		QueryTypes:    make(map[string]int64),
		ResponseCodes: make(map[string]int64),
	}

	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "total.num.queries":
			s.TotalQueries, _ = strconv.ParseInt(val, 10, 64)
		case "total.num.cachehits":
			s.CacheHits, _ = strconv.ParseInt(val, 10, 64)
		case "total.num.cachemiss":
			s.CacheMiss, _ = strconv.ParseInt(val, 10, 64)
		case "total.recursion.time.avg":
			avg, _ := strconv.ParseFloat(val, 64)
			s.AvgRecursionMs = avg * 1000 // seconds → ms
		case "msg.cache.count":
			s.MsgCacheCount, _ = strconv.ParseInt(val, 10, 64)
		case "rrset.cache.count":
			s.RRSetCacheCount, _ = strconv.ParseInt(val, 10, 64)
		case "mem.total.sbrk":
			s.MemTotalBytes, _ = strconv.ParseInt(val, 10, 64)
		case "time.up":
			up, _ := strconv.ParseFloat(val, 64)
			s.UptimeSeconds = int64(up)
		default:
			if strings.HasPrefix(key, "num.query.type.") {
				qtype := strings.TrimPrefix(key, "num.query.type.")
				s.QueryTypes[qtype], _ = strconv.ParseInt(val, 10, 64)
			} else if strings.HasPrefix(key, "num.answer.rcode.") {
				rcode := strings.TrimPrefix(key, "num.answer.rcode.")
				s.ResponseCodes[rcode], _ = strconv.ParseInt(val, 10, 64)
			}
		}
	}

	if s.TotalQueries > 0 {
		s.CacheHitRate = float64(s.CacheHits) / float64(s.TotalQueries) * 100
	}

	return s
}
