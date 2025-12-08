package dns

import (
    "sync"

    "golang.org/x/crypto/acme/autocert"
)

var (
    autocertOnce sync.Once
    autocertMgr  *autocert.Manager
)

// getAutocertManager lazily initializes a shared autocert manager for HTTP-01.
func getAutocertManager(cacheDir string, hosts []string, email string) *autocert.Manager {
    autocertOnce.Do(func() {
        autocertMgr = &autocert.Manager{
            Cache:      autocert.DirCache(cacheDir),
            Prompt:     autocert.AcceptTOS,
            Email:      email,
            HostPolicy: autocert.HostWhitelist(hosts...),
        }
    })
    return autocertMgr
}

