package payment

import (
	"math/rand"

	"github.com/google/uuid"
)

func Charge(amount int) (success bool, transactionID string) {
	transactionID = uuid.NewString()
	success = rand.Intn(100) < 80
	return
}
