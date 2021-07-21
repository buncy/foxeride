package db

import "time"

type Transaction map[*time.Time]Payment

type Payment struct {
	ID     string
	Amount int
}

type Customer struct {
	PaymentID    string `json:"paymentID"`
	Transactions string `json:"transaction"`
}
