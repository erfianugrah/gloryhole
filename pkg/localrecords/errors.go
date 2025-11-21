package localrecords

import "errors"

var (
	// ErrInvalidRecord is returned when a record is invalid
	ErrInvalidRecord = errors.New("invalid record")

	// ErrRecordNotFound is returned when a record is not found
	ErrRecordNotFound = errors.New("record not found")

	// ErrInvalidDomain is returned when a domain name is invalid
	ErrInvalidDomain = errors.New("invalid domain name")

	// ErrInvalidIP is returned when an IP address is invalid
	ErrInvalidIP = errors.New("invalid IP address")

	// ErrCNAMELoop is returned when a CNAME loop is detected
	ErrCNAMELoop = errors.New("CNAME loop detected")

	// ErrMultipleCNAME is returned when multiple CNAME records exist for the same domain
	ErrMultipleCNAME = errors.New("multiple CNAME records not allowed")

	// ErrCNAMEWithOther is returned when CNAME coexists with other record types
	ErrCNAMEWithOther = errors.New("CNAME cannot coexist with other record types")

	// ErrEmptyTarget is returned when a CNAME/MX/SRV record has no target
	ErrEmptyTarget = errors.New("target cannot be empty")

	// ErrNoIPs is returned when an A/AAAA record has no IP addresses
	ErrNoIPs = errors.New("A/AAAA record must have at least one IP address")
)
