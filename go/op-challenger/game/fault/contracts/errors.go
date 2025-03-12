package contracts

import "fmt"

var (
	InsufficientAllowance = fmt.Errorf("insufficient allowance")
	InsufficientBalance   = fmt.Errorf("insufficient balance")
)
