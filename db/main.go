package db

import (
	"fmt"
	"time"

	"github.com/boltdb/bolt"
)

// var (
// 	db *bolt.DB
// )

func AddCustomer(db *bolt.DB, userID, customerID string) error {

	err := db.Update(func(tx *bolt.Tx) error {

		err := tx.Bucket([]byte("DB")).Bucket([]byte("CUSTOMERS")).Put([]byte(userID), []byte(customerID))
		if err != nil {
			return fmt.Errorf("could not insert entry: %v", err)
		}

		return nil
	})
	fmt.Println("Added Entry")
	if err != nil {
		return err
	}
	return nil
}

func GetCustomer(db *bolt.DB, userID string) (string, error) {
	fmt.Println("reading customer ====>", userID)
	var customerID string
	err := db.View(func(tx *bolt.Tx) error {
		customerID = string(tx.Bucket([]byte("DB")).Bucket([]byte("CUSTOMERS")).Get([]byte(userID)))
		return nil
	})
	return customerID, err
}

func GetDB() (*bolt.DB, error) {
	fmt.Println("opening DB")
	db, err := bolt.Open("test.db", 0600, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("could not open db, %v", err)
	}
	return db, nil
}
