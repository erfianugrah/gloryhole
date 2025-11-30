# Glory-Hole Helper Scripts

Utility scripts for Glory-Hole DNS administration.

## Password Hashing Script

Generate bcrypt password hashes for authentication configuration.

### Usage

```bash
go run scripts/hash-password.go "your-password"
```

### Examples

**Basic usage (default cost 12):**
```bash
go run scripts/hash-password.go "mysecretpassword"
```

**Output:**
```yaml
# Copy this into your config.yml:
auth:
  enabled: true
  username: "admin"
  password_hash: "$2a$12$abcdefghijklmnopqrstuvwxyz..."
  api_key: ""  # Optional: For bearer token auth
  header: "Authorization"

# Or just the hash:
$2a$12$abcdefghijklmnopqrstuvwxyz...
```

**Custom cost (higher = more secure but slower):**
```bash
go run scripts/hash-password.go -cost 14 "mysecretpassword"
```

### Cost Parameter

- **Default**: 12 (recommended)
- **Range**: 10-14
- **10**: Fast, less secure
- **12**: Balanced (recommended)
- **14**: Slow, more secure

Higher cost increases security by making brute-force attacks slower, but also increases server CPU usage during authentication.

### Docker Usage

If you're using Docker and don't have Go installed locally:

```bash
# Use the built image
docker run --rm ghcr.io/erfianugrah/gloryhole:latest /glory-hole hash-password "your-password"

# Or with docker-compose
docker-compose run --rm glory-hole /glory-hole hash-password "your-password"
```

### Integration

1. Run the script to generate a hash
2. Copy the `password_hash` value
3. Update your `config.yml`:

```yaml
auth:
  enabled: true
  username: "admin"
  password_hash: "$2a$12$..."  # Paste your hash here
```

4. Restart Glory-Hole to apply changes

### Security Notes

- **Never commit plaintext passwords** to version control
- Use `password_hash` instead of the deprecated `password` field
- Each hash is unique (bcrypt includes random salt)
- Store hashes securely (e.g., use environment variables in production)

### Troubleshooting

**"command not found: go"**
- Install Go from https://go.dev/dl/
- Or use Docker method above

**"Error: cost must be between 4 and 31"**
- Use cost between 10-14 (recommended: 12)

**Authentication not working?**
- Verify `auth.enabled: true` in config
- Check username matches your config
- Ensure you're using the correct password
- Check logs for auth errors
