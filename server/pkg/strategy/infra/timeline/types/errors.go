package types

type ErrorPolicy string

const (
	ErrorPolicyFailFast ErrorPolicy = "fail_fast"
	ErrorPolicyDegrade  ErrorPolicy = "degrade"
)
