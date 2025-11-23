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

	// ErrNoTxtData is returned when a TXT record has no text data
	ErrNoTxtData = errors.New("TXT record must have at least one text string")

	// ErrTxtTooLong is returned when a TXT string exceeds 255 characters
	ErrTxtTooLong = errors.New("TXT string exceeds 255 characters")

	// ErrInvalidSOA is returned when a SOA record is invalid
	ErrInvalidSOA = errors.New("SOA record must have primary nameserver (ns) and responsible person (mbox)")

	// ErrInvalidCAA is returned when a CAA record is invalid
	ErrInvalidCAA = errors.New("CAA record must have tag and value")
)
