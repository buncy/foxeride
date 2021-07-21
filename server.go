package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	db "stripe-payments-sample-code/db"

	"github.com/boltdb/bolt"
	"github.com/joho/godotenv"
	"github.com/stripe/stripe-go/v72"
	"github.com/stripe/stripe-go/v72/customer"
	"github.com/stripe/stripe-go/v72/ephemeralkey"
	"github.com/stripe/stripe-go/v72/invoice"
	"github.com/stripe/stripe-go/v72/invoiceitem"
	"github.com/stripe/stripe-go/v72/paymentintent"
	"github.com/stripe/stripe-go/v72/paymentmethod"
	"github.com/stripe/stripe-go/v72/setupintent"
)

var (
	paymentIntentID string
	data            *bolt.DB
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	data, err = db.GetDB()
	if err != nil {
		fmt.Println("this db had problems", err.Error())
	}

	stripe.Key = os.Getenv("STRIPE_SECRET_KEY")

	// For sample support and debugging, not required for production:
	stripe.SetAppInfo(&stripe.AppInfo{
		Name:    "stripe-samples/accept-a-payment/custom-payment-flow",
		Version: "0.0.1",
		URL:     "https://github.com/stripe-samples",
	})

	http.Handle("/", http.FileServer(http.Dir(".")))
	http.HandleFunc("/config", handleConfig)
	http.HandleFunc("/create-setup-intent", handleCreateSetupIntent)
	http.HandleFunc("/create-payment-intent", handleCreatePaymentIntent)
	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/customer", HandleCustomer)
	http.HandleFunc("/create-invoice", HandleCreateInvoice)
	http.HandleFunc("/charge", HandleCharge)
	http.HandleFunc("/cards", handleCards)

	log.Println("server running at 0.0.0.0:4242")
	http.ListenAndServe("0.0.0.0:4242", nil)
}

// ErrorResponseMessage represents the structure of the error
// object sent in failed responses.
type ErrorResponseMessage struct {
	Message string `json:"message"`
}

// ErrorResponse represents the structure of the error object sent
// in failed responses.
type ErrorResponse struct {
	Error *ErrorResponseMessage `json:"error"`
}

func handleConfig(w http.ResponseWriter, r *http.Request) {
	fmt.Println("sending config ==================>")
	if r.Method != "GET" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, struct {
		PublishableKey string `json:"publishableKey"`
	}{
		PublishableKey: os.Getenv("STRIPE_PUBLISHABLE_KEY"),
	})
}

type paymentIntentCreateReq struct {
	Currency          string `json:"currency"`
	PaymentMethodType string `json:"paymentMethodType"`
	CustomerName      string `json:"customerName"`
	CustomerEmail     string `json:"customerEmail"`
	UserID            string `json:"userID"`
}

type foxeCustomer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type paymentMethods struct {
	UserID        string `json:"userID"`
	PaymentMethod string `json:"type"`
}

type CardDetails struct {
	CardHolder      string `json:"cardHolder"`
	Brand           string `json:"brand"`
	Month           int    `json:"month"`
	Year            int    `json:"year"`
	PaymentMethodID string `json:"paymentMethodID"`
	BalanceType     string `json:"balanceType"`
	LastFourDigits  string `json:"lastFourDigits"`
}

type ChargeDetails struct {
	PaymentMethodID string `json:"paymentMethodID"`
	UserID          string `json:"userID"`
	OffSession      bool   `json:"offSession"`
}

func handleCreatePaymentIntent(w http.ResponseWriter, r *http.Request) {
	req := paymentIntentCreateReq{}
	json.NewDecoder(r.Body).Decode(&req)
	fmt.Println("\n================request\n", req, "\n========end\n")
	var c *stripe.Customer
	customerID := ""
	var ephemeralKey *stripe.EphemeralKey
	var ephemeralKeyParams *stripe.EphemeralKeyParams

	user, err := db.GetCustomer(data, req.UserID)
	if err != nil {
		fmt.Println("this db had problems", err.Error())
	}
	customerID = user

	fmt.Println("this is the customer ======>", user)
	//if customer is empty then create a new customerID and attach the userID to it

	if customerID == "" {
		fmt.Println("customerID is empty so creating one======>")
		customerParams := &stripe.CustomerParams{
			Name: stripe.String(req.CustomerName),
		}
		c, _ = customer.New(customerParams)
		ephemeralKeyParams = &stripe.EphemeralKeyParams{
			Customer:      stripe.String(c.ID),
			StripeVersion: stripe.String("2020-08-27"),
		}
		customerID = c.ID
		fmt.Println("customerID is created======>", customerID)
		db.AddCustomer(data, req.UserID, customerID)
	}
	fmt.Println("created customerID is =-====>", customerID)
	ephemeralKeyParams = &stripe.EphemeralKeyParams{
		Customer:      stripe.String(customerID),
		StripeVersion: stripe.String("2020-08-27"),
	}
	ephemeralKey, _ = ephemeralkey.New(ephemeralKeyParams)

	params := &stripe.PaymentIntentParams{
		Amount:             stripe.Int64(1999),
		Currency:           stripe.String(req.Currency),
		PaymentMethodTypes: stripe.StringSlice([]string{req.PaymentMethodType}),
		Customer:           stripe.String(customerID),
	}

	// If this is for an ACSS payment, we add payment_method_options to create
	// the Mandate.
	if req.PaymentMethodType == "acss_debit" {
		params.PaymentMethodOptions = &stripe.PaymentIntentPaymentMethodOptionsParams{
			ACSSDebit: &stripe.PaymentIntentPaymentMethodOptionsACSSDebitParams{
				MandateOptions: &stripe.PaymentIntentPaymentMethodOptionsACSSDebitMandateOptionsParams{
					PaymentSchedule: stripe.String("sporadic"),
					TransactionType: stripe.String("personal"),
				},
			},
		}
	}

	pi, err := paymentintent.New(params)
	if err != nil {
		// Try to safely cast a generic error to a stripe.Error so that we can get at
		// some additional Stripe-specific information about what went wrong.
		if stripeErr, ok := err.(*stripe.Error); ok {
			fmt.Printf("Other Stripe error occurred: %v\n", stripeErr.Error())
			writeJSONErrorMessage(w, stripeErr.Error(), 400)
		} else {
			fmt.Printf("Other error occurred: %v\n", err.Error())
			writeJSONErrorMessage(w, "Unknown server error", 500)
		}

		return
	}
	paymentIntentID = pi.ID
	writeJSON(w, struct {
		ClientSecret string `json:"clientSecret"`
		CustomerID   string `json:"customerID"`
		EphemeralKey string `json:"ephemeralKey"`
	}{
		ClientSecret: pi.ClientSecret,
		CustomerID:   customerID,
		EphemeralKey: ephemeralKey.Secret,
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		log.Printf("json.NewEncoder.Encode: %v", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := io.Copy(w, &buf); err != nil {
		log.Printf("io.Copy: %v", err)
		return
	}
}

func writeJSONError(w http.ResponseWriter, v interface{}, code int) {
	w.WriteHeader(code)
	writeJSON(w, v)
	return
}

func writeJSONErrorMessage(w http.ResponseWriter, message string, code int) {
	resp := &ErrorResponse{
		Error: &ErrorResponseMessage{
			Message: message,
		},
	}
	writeJSONError(w, resp, code)
}

func handleCreateSetupIntent(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	params := &stripe.CustomerParams{}
	c, _ := customer.New(params)
	setupParams := &stripe.SetupIntentParams{
		Customer: stripe.String(c.ID),
		Usage:    stripe.String("on_session"),
	}
	si, _ := setupintent.New(setupParams)
	clientSecret := si.ClientSecret

	writeJSON(w, struct {
		SetupClientSecret string `json:"setupClientSecret"`
	}{
		SetupClientSecret: clientSecret,
	})
}

func handleWebhook(w http.ResponseWriter, req *http.Request) {
	const MaxBodyBytes = int64(65536)
	req.Body = http.MaxBytesReader(w, req.Body, MaxBodyBytes)
	payload, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading request body: %v\n", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}

	event := stripe.Event{}

	if err := json.Unmarshal(payload, &event); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse webhook body json: %v\n", err.Error())
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Unmarshal the event data into an appropriate struct depending on its Type
	switch event.Type {
	case "payment_intent.succeeded":
		var paymentIntent stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &paymentIntent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing webhook JSON: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if paymentIntent.ID == paymentIntentID {
			fmt.Println("=====||======the payment was successful========||=========")
		}
		fmt.Printf("PaymentIntent was successful! this is the \n paymentIntent ID :==>> %v \n customerID is :==>> %v", paymentIntent.ID, paymentIntent.Customer)
		fmt.Printf("\n charged amount is :==>> %v \n emailID is :==>> %v", paymentIntent.Amount, paymentIntent.ReceiptEmail)
	case "payment_intent.created":
		var paymentIntent stripe.PaymentIntent
		err := json.Unmarshal(event.Data.Raw, &paymentIntent)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing webhook JSON: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Println("PaymentIntent created!")
	case "payment_method.attached":
		var paymentMethod stripe.PaymentMethod
		err := json.Unmarshal(event.Data.Raw, &paymentMethod)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing webhook JSON: %v\n", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Println("PaymentMethod was attached to a Customer!")
	// ... handle other event types
	default:
		fmt.Fprintf(os.Stderr, "Unhandled event type: %s\n", event.Type)
	}

	w.WriteHeader(http.StatusOK)
}

func HandleCustomer(w http.ResponseWriter, r *http.Request) {

	req := foxeCustomer{}
	json.NewDecoder(r.Body).Decode(&req)
	fmt.Println("\n================request\n", req, "\n========end\n")

	params := &stripe.CustomerParams{
		Name: stripe.String(req.Name),
	}
	cus, err := customer.New(params)
	fmt.Println(cus.ID)

	if err != nil {
		// Try to safely cast a generic error to a stripe.Error so that we can get at
		// some additional Stripe-specific information about what went wrong.
		if stripeErr, ok := err.(*stripe.Error); ok {
			fmt.Printf("Other Stripe error occurred: %v\n", stripeErr.Error())
			writeJSONErrorMessage(w, stripeErr.Error(), 400)
		} else {
			fmt.Printf("Other error occurred: %v\n", err.Error())
			writeJSONErrorMessage(w, "Unknown server error", 500)
		}

		return
	}
	writeJSON(w, struct {
		CustomerID string `json:"customerID"`
	}{
		CustomerID: cus.ID,
	})
}

func HandleCreateInvoice(w http.ResponseWriter, r *http.Request) {

	// params := &stripe.CustomerParams{
	// 	Name:        stripe.String("jenny rosen"),
	// 	Email:       stripe.String("jenny.rosen@example.com"),
	// 	Description: stripe.String("My First Test Customer (created for API docs)"),
	// }
	// cus, _ := customer.New(params)
	// spew.Println(cus)
	// customerID := cus.ID
	invoiceParams := &stripe.InvoiceItemParams{
		Customer: stripe.String("cus_Jl6aiT1bRvWZAh"),
		Amount:   stripe.Int64(1999),
		Currency: stripe.String("usd"),
	}
	ii, _ := invoiceitem.New(invoiceParams)
	fmt.Println(ii.Customer.ID)
	invParams := &stripe.InvoiceParams{
		Customer:         stripe.String("cus_Jl6aiT1bRvWZAh"),
		AutoAdvance:      stripe.Bool(true),
		CollectionMethod: stripe.String("charge_automatically"),
	}
	in, _ := invoice.New(invParams)
	fmt.Println(in.CustomFields)
	inv, _ := invoice.FinalizeInvoice(in.ID, nil)
	fmt.Println(inv.Charge)
}

// func HandleCharge(w http.ResponseWriter, r *http.Request) {
// 	params := &stripe.PaymentIntentParams{
// 		Amount:        stripe.Int64(1099),
// 		Currency:      stripe.String(string(stripe.CurrencyUSD)),
// 		Customer:      stripe.String("cus_Jl6aiT1bRvWZAh"),
// 		PaymentMethod: stripe.String("pm_1J7aNYLl6Dh4Ry32t69VPmZE"),
// 		Confirm:       stripe.Bool(true),
// 		OffSession:    stripe.Bool(true),
// 	}

// 	_, err := paymentintent.New(params)

// 	if err != nil {
// 		if stripeErr, ok := err.(*stripe.Error); ok {
// 			// Error code will be authentication_required if authentication is needed
// 			fmt.Printf("Error code: %v", stripeErr.Code)

// 			paymentIntentID := stripeErr.PaymentIntent.ID
// 			paymentIntent, _ := paymentintent.Get(paymentIntentID, nil)

// 			fmt.Printf("PI: %v", paymentIntent.ID)
// 		}
// 	}
// }

func handleCards(w http.ResponseWriter, r *http.Request) {

	req := paymentMethods{}
	json.NewDecoder(r.Body).Decode(&req)
	fmt.Println("cards request ===============>\n", req, "\n end of request for cards ==========>") //TODO: remove this later
	customerID, err := db.GetCustomer(data, req.UserID)
	if err != nil {
		fmt.Println("this is getCustomer===>", err.Error())
	}
	fmt.Println("this is the customer ====>", customerID)
	params := &stripe.PaymentMethodListParams{
		Customer: stripe.String(customerID),
		Type:     stripe.String(req.PaymentMethod),
	}
	i := paymentmethod.List(params)
	cards := []*CardDetails{}
	for i.Next() {
		pm := i.PaymentMethod()
		card := &CardDetails{
			PaymentMethodID: pm.ID,
			Brand:           string(pm.Card.Brand),
			Month:           int(pm.Card.ExpMonth),
			Year:            int(pm.Card.ExpYear),
			BalanceType:     string(pm.Card.Funding),
			CardHolder:      pm.BillingDetails.Name,
			LastFourDigits:  pm.Card.Last4,
		}

		cards = append(cards, card)

		//spew.Dump(pm.BillingDetails)
	}
	//spew.Dump(cards)

	writeJSON(w, struct {
		Cards []*CardDetails `json:"cards"`
	}{

		Cards: cards,
	})
}

func HandleCharge(w http.ResponseWriter, r *http.Request) {
	req := ChargeDetails{}
	json.NewDecoder(r.Body).Decode(&req)
	customerID, err := db.GetCustomer(data, req.UserID)
	fmt.Println("this is the customerID in charge ====>", customerID)
	if err != nil {
		fmt.Println("this is getCustomer===>", err.Error())
	}
	params := &stripe.PaymentIntentParams{
		Amount:                stripe.Int64(1099),
		Currency:              stripe.String(string(stripe.CurrencyUSD)),
		Customer:              stripe.String(customerID),
		PaymentMethod:         stripe.String(req.PaymentMethodID),
		Confirm:               stripe.Bool(true),
		OffSession:            stripe.Bool(req.OffSession),
		ErrorOnRequiresAction: stripe.Bool(true),
	}

	pi, perr := paymentintent.New(params)
	if perr != nil {
		if stripeErr, ok := perr.(*stripe.Error); ok {
			// Error code will be authentication_required if authentication is needed
			fmt.Printf("Error code: %v", stripeErr.Code)

			paymentIntentID := stripeErr.PaymentIntent.ID
			paymentIntent, _ := paymentintent.Get(paymentIntentID, nil)

			fmt.Printf("PI: %v", paymentIntent.ID)
		}
	}

	writeJSON(w, struct {
		ClientSecret string `json:"clientSecret"`
	}{
		ClientSecret: pi.ClientSecret,
	})
}
