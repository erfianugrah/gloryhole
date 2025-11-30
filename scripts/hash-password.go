package main

import (
	"flag"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	cost := flag.Int("cost", 12, "Bcrypt cost parameter (10-14 recommended, higher = more secure but slower)")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Usage: go run scripts/hash-password.go [OPTIONS] <password>")
		fmt.Println("\nOptions:")
		fmt.Println("  -cost int    Bcrypt cost parameter (default: 12)")
		fmt.Println("\nExample:")
		fmt.Println("  go run scripts/hash-password.go \"mysecretpassword\"")
		fmt.Println("  go run scripts/hash-password.go -cost 14 \"mysecretpassword\"")
		os.Exit(1)
	}

	password := args[0]

	// Validate cost
	if *cost < bcrypt.MinCost || *cost > bcrypt.MaxCost {
		fmt.Fprintf(os.Stderr, "Error: cost must be between %d and %d\n", bcrypt.MinCost, bcrypt.MaxCost)
		os.Exit(1)
	}

	// Generate hash
	hash, err := bcrypt.GenerateFromPassword([]byte(password), *cost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error generating hash: %v\n", err)
		os.Exit(1)
	}

	// Output in config format
	fmt.Printf("# Copy this into your config.yml:\n")
	fmt.Printf("auth:\n")
	fmt.Printf("  enabled: true\n")
	fmt.Printf("  username: \"admin\"\n")
	fmt.Printf("  password_hash: \"%s\"\n", string(hash))
	fmt.Printf("  api_key: \"\"  # Optional: For bearer token auth\n")
	fmt.Printf("  header: \"Authorization\"\n")
	fmt.Printf("\n# Or just the hash:\n")
	fmt.Printf("%s\n", string(hash))
}
