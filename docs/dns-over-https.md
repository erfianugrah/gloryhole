# DNS-over-HTTPS (DoH)

Glory-Hole now supports DNS-over-HTTPS (DoH), allowing you to query DNS records over HTTPS. This provides privacy and security benefits by encrypting DNS queries, making them indistinguishable from regular HTTPS traffic.

## Overview

The DoH implementation is **RFC 8484 compatible** and works with Cloudflare, Google, and other DoH clients.

**Endpoint**: `/dns-query`

**Supported Methods**:
- `GET` - Query with URL parameters or base64-encoded DNS message
- `POST` - Query with binary DNS message in request body
- `HEAD` - Health check / availability test

## Quick Start

### Basic GET Query

```bash
# Query A record for example.com (JSON response)
curl -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=example.com&type=A"

# Query AAAA record for google.com
curl -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=google.com&type=AAAA"
```

### Query Types

You can specify the DNS record type as either a string or number:

```bash
# Using type name
?name=example.com&type=A
?name=example.com&type=AAAA
?name=example.com&type=MX
?name=example.com&type=TXT

# Using type number (RFC 1035)
?name=example.com&type=1    # A record
?name=example.com&type=28   # AAAA record
?name=example.com&type=15   # MX record
```

## Request Methods

### GET - JSON Response

Get DNS query results as JSON (Cloudflare/Google compatible format):

```bash
curl -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=example.com&type=A"
```

**Response**:
```json
{
  "Status": 0,
  "TC": false,
  "RD": true,
  "RA": true,
  "AD": false,
  "CD": false,
  "Question": [
    {
      "name": "example.com",
      "type": 1
    }
  ],
  "Answer": [
    {
      "name": "example.com",
      "type": 1,
      "TTL": 300,
      "data": "93.184.216.34"
    }
  ]
}
```

### GET - Wire Format Response

Get DNS response in binary wire format (RFC 1035):

```bash
curl -H "Accept: application/dns-message" \
  "http://localhost:8080/dns-query?name=example.com&type=A" \
  --output response.bin
```

### GET - Base64 Encoded Query

Send a base64-encoded DNS query (useful for complex queries):

```bash
# Create DNS query, encode it, and send
dns_query=$(echo -n "..." | base64 -w0)
curl -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?dns=$dns_query"
```

### POST - Binary DNS Message

Send a binary DNS message in the POST body:

```bash
# Create DNS query using dig and send via POST
dig +qr example.com A | \
  sed -n '/;; QUESTION SECTION:/,/^$/p' | \
  tail -n +2 | head -n -1 > query.txt

# Convert to wire format and POST
curl -X POST \
  -H "Content-Type: application/dns-message" \
  -H "Accept: application/dns-json" \
  --data-binary @query.bin \
  "http://localhost:8080/dns-query"
```

### HEAD - Health Check

Check if the DoH endpoint is available:

```bash
curl -I "http://localhost:8080/dns-query"
```

## Query Parameters

| Parameter | Type | Description | Example |
|-----------|------|-------------|---------|
| `name` | string | Domain name to query | `example.com` |
| `type` | string/int | DNS record type | `A`, `AAAA`, `1`, `28` |
| `dns` | base64 | Base64-encoded DNS query | `AAABAAABAAA...` |
| `cd` | boolean | Checking Disabled (DNSSEC) | `1`, `true` |
| `do` | boolean | DNSSEC OK (EDNS0) | `1`, `true` |

## Response Formats

### JSON Format (`application/dns-json`)

Compatible with Cloudflare and Google DNS-over-HTTPS APIs.

**Fields**:
- `Status`: DNS response code (0 = NOERROR, 3 = NXDOMAIN, etc.)
- `TC`: Truncated response
- `RD`: Recursion Desired
- `RA`: Recursion Available
- `AD`: Authenticated Data (DNSSEC)
- `CD`: Checking Disabled (DNSSEC)
- `Question`: Array of DNS questions
- `Answer`: Array of DNS answers
- `Authority`: Array of authority records
- `Additional`: Array of additional records

### Wire Format (`application/dns-message`)

Binary DNS message format as defined in RFC 1035. Use this for:
- Maximum compatibility
- Smallest response size
- Integration with existing DNS tools

## DNSSEC Support

Glory-Hole supports DNSSEC query flags:

```bash
# Request DNSSEC validation
curl -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=example.com&type=A&do=1"

# Disable DNSSEC checking
curl -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=example.com&type=A&cd=1"
```

## Caching

DoH responses include `Cache-Control` headers based on the DNS TTL:

```
Cache-Control: max-age=300
```

The TTL is calculated as the minimum TTL of all answers in the response.

## Error Handling

### HTTP Status Codes

| Code | Meaning | Example |
|------|---------|---------|
| 200 | Success | Query completed successfully |
| 400 | Bad Request | Missing `name` parameter, invalid query |
| 405 | Method Not Allowed | Used PUT or DELETE |
| 413 | Payload Too Large | Query exceeds 4KB |
| 504 | Gateway Timeout | Upstream DNS timeout |

### Error Response (JSON)

```json
{
  "error": "missing 'name' or 'dns' parameter",
  "status": 400
}
```

## Security Considerations

### Authentication

DoH endpoints respect the global auth configuration. If auth is enabled, DoH queries require:
- HTTP Basic Auth, or
- Bearer token (API key)

```bash
# With Basic Auth
curl -u "admin:password" \
  -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=example.com&type=A"

# With API Key
curl -H "Authorization: Bearer your-api-key" \
  -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=example.com&type=A"
```

### CORS

Cross-Origin Resource Sharing (CORS) is controlled by the `cors_allowed_origins` configuration:

```yaml
server:
  cors_allowed_origins:
    - "https://dns-client.example.com"
```

If not configured, cross-origin requests will be blocked.

### Rate Limiting

DoH queries are subject to the same rate limiting as regular API requests. Configure rate limits in your `config.yml`:

```yaml
rate_limit:
  enabled: true
  requests_per_second: 100
  burst: 200
```

## Integration Examples

### Browser JavaScript

```javascript
// Fetch DNS A record for example.com
fetch('http://localhost:8080/dns-query?name=example.com&type=A', {
  headers: {
    'Accept': 'application/dns-json'
  }
})
.then(response => response.json())
.then(data => {
  console.log('DNS Response:', data);
  console.log('IP Addresses:', data.Answer.map(a => a.data));
});
```

### Python

```python
import requests

response = requests.get(
    'http://localhost:8080/dns-query',
    params={'name': 'example.com', 'type': 'A'},
    headers={'Accept': 'application/dns-json'}
)

data = response.json()
print(f"Status: {data['Status']}")
for answer in data['Answer']:
    print(f"{answer['name']} -> {answer['data']} (TTL: {answer['TTL']})")
```

### Go

```go
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type DNSResponse struct {
	Status int `json:"Status"`
	Answer []struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		TTL  uint32 `json:"TTL"`
		Data string `json:"data"`
	} `json:"Answer"`
}

func main() {
	resp, _ := http.Get("http://localhost:8080/dns-query?name=example.com&type=A")
	defer resp.Body.Close()

	var dns DNSResponse
	json.NewDecoder(resp.Body).Decode(&dns)

	for _, answer := range dns.Answer {
		fmt.Printf("%s -> %s\n", answer.Name, answer.Data)
	}
}
```

### curl with jq

```bash
# Pretty-print DNS response
curl -s -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=example.com&type=A" | \
  jq '.Answer[] | "\(.name) -> \(.data) (TTL: \(.TTL))"'

# Get only IP addresses
curl -s -H "Accept: application/dns-json" \
  "http://localhost:8080/dns-query?name=example.com&type=A" | \
  jq -r '.Answer[].data'
```

## Client Configuration

### Configure Your Browser

Most modern browsers support DNS-over-HTTPS. Configure them to use Glory-Hole:

**Firefox**:
1. Go to `about:config`
2. Set `network.trr.mode` to `2` (DoH with fallback) or `3` (DoH only)
3. Set `network.trr.uri` to `http://your-server:8080/dns-query`

**Chrome/Edge** (via command line):
```bash
chrome --enable-features="DnsOverHttps" \
  --dns-over-https-server="http://your-server:8080/dns-query"
```

### System-Wide DoH

Use tools like `dnscrypt-proxy` or `cloudflared` to configure system-wide DoH:

**cloudflared** (as DoH proxy):
```bash
cloudflared proxy-dns \
  --address 0.0.0.0 \
  --port 5300 \
  --upstream http://your-server:8080/dns-query
```

## Comparison with Traditional DNS

| Feature | Traditional DNS (Port 53) | DNS-over-HTTPS |
|---------|--------------------------|----------------|
| **Encryption** | No | Yes (HTTPS/TLS) |
| **Port** | 53 (UDP/TCP) | 443 or custom (HTTPS) |
| **Firewall Friendly** | Often blocked | Works everywhere HTTPS does |
| **ISP Visibility** | Queries visible to ISP | Encrypted, looks like HTTPS |
| **Performance** | Lower latency | Slightly higher (TLS overhead) |
| **Caching** | DNS-level caching | HTTP caching + DNS caching |

## Performance

DoH adds minimal overhead:
- **TLS handshake**: ~50-100ms (first request only, then reused)
- **HTTP/2**: Multiplexing reduces per-query overhead
- **Caching**: HTTP cache-control headers enable client-side caching

Tested performance (local network):
- Traditional DNS: ~5-10ms average
- DNS-over-HTTPS: ~10-15ms average (after TLS handshake)

## Troubleshooting

### Query not working?

1. Check if the API server is running:
   ```bash
   curl -I http://localhost:8080/health
   ```

2. Verify DoH endpoint is accessible:
   ```bash
   curl -I http://localhost:8080/dns-query
   ```

3. Check auth requirements (if enabled):
   ```bash
   curl -u admin:password -H "Accept: application/dns-json" \
     "http://localhost:8080/dns-query?name=example.com&type=A"
   ```

### Getting 400 errors?

- Ensure `name` parameter is provided
- Check that `type` is valid (A, AAAA, MX, etc.)
- For POST requests, use `Content-Type: application/dns-message`

### Slow responses?

- Check upstream DNS server latency
- Enable caching (`cache.enabled: true`)
- Use HTTP/2 for connection multiplexing

## Future Enhancements (v0.9)

Planned features for upcoming releases:
- **Full DNSSEC validation**: Validate DNSSEC signatures
- **Recursive resolver mode**: Become a validating recursive resolver
- **DoH Proxy mode**: Proxy to other DoH servers
- **Query logging**: Detailed DoH query logging
- **DNS-over-TLS (DoT)**: Add DoT support alongside DoH

## References

- [RFC 8484 - DNS Queries over HTTPS (DoH)](https://datatracker.ietf.org/doc/html/rfc8484)
- [RFC 1035 - Domain Names - Implementation and Specification](https://datatracker.ietf.org/doc/html/rfc1035)
- [Cloudflare DoH API](https://developers.cloudflare.com/1.1.1.1/encryption/dns-over-https/)
- [Google Public DNS DoH](https://developers.google.com/speed/public-dns/docs/doh)
